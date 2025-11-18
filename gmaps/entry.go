package gmaps

import (
	"encoding/json"
	"fmt"
	"iter"
	"math"
	"net/url"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
)

type Image struct {
	Title string `json:"title"`
	Image string `json:"image"`
}

type LinkSource struct {
	Link   string `json:"link"`
	Source string `json:"source"`
}

type Owner struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Link string `json:"link"`
}

type Address struct {
	Borough    string `json:"borough"`
	Street     string `json:"street"`
	City       string `json:"city"`
	PostalCode string `json:"postal_code"`
	State      string `json:"state"`
	Country    string `json:"country"`
}

type Option struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type About struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Options []Option `json:"options"`
}

type Review struct {
	Name           string
	ProfilePicture string
	Rating         int
	Description    string
	Images         []string
	When           string
}

type Entry struct {
	ID         string              `json:"input_id"`
	Link       string              `json:"link"`
	Cid        string              `json:"cid"`
	Title      string              `json:"title"`
	Categories []string            `json:"categories"`
	Category   string              `json:"category"`
	Address    string              `json:"address"`
	OpenHours  map[string][]string `json:"open_hours"`
	// PopularTImes is a map with keys the days of the week
	// and value is a map with key the hour and value the traffic in that time
	PopularTimes        map[string]map[int]int `json:"popular_times"`
	WebSite             string                 `json:"web_site"`
	Phone               string                 `json:"phone"`
	PlusCode            string                 `json:"plus_code"`
	ReviewCount         int                    `json:"review_count"`
	ReviewRating        float64                `json:"review_rating"`
	ReviewsPerRating    map[int]int            `json:"reviews_per_rating"`
	Latitude            float64                `json:"latitude"`
	Longtitude          float64                `json:"longtitude"`
	Status              string                 `json:"status"`
	Description         string                 `json:"description"`
	ReviewsLink         string                 `json:"reviews_link"`
	Thumbnail           string                 `json:"thumbnail"`
	Timezone            string                 `json:"timezone"`
	PriceRange          string                 `json:"price_range"`
	DataID              string                 `json:"data_id"`
	Images              []Image                `json:"images"`
	Reservations        []LinkSource           `json:"reservations"`
	OrderOnline         []LinkSource           `json:"order_online"`
	Menu                LinkSource             `json:"menu"`
	Owner               Owner                  `json:"owner"`
	CompleteAddress     Address                `json:"complete_address"`
	About               []About                `json:"about"`
	UserReviews         []Review               `json:"user_reviews"`
	UserReviewsExtended []Review               `json:"user_reviews_extended"`
	Emails              []string               `json:"emails"`
	PlaceID             string                 `json:"place_id"`
	PlaceIDURL          string                 `json:"place_id_url"`
}

func (e *Entry) haversineDistance(lat, lon float64) float64 {
	const R = 6371e3 // earth radius in meters

	clat := lat * math.Pi / 180
	clon := lon * math.Pi / 180

	elat := e.Latitude * math.Pi / 180
	elon := e.Longtitude * math.Pi / 180

	dlat := elat - clat
	dlon := elon - clon

	a := math.Sin(dlat/2)*math.Sin(dlat/2) +
		math.Cos(clat)*math.Cos(elat)*
			math.Sin(dlon/2)*math.Sin(dlon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c
}

func (e *Entry) isWithinRadius(lat, lon, radius float64) bool {
	distance := e.haversineDistance(lat, lon)

	return distance <= radius
}

func (e *Entry) IsWebsiteValidForEmail() bool {
	if e.WebSite == "" {
		return false
	}

	needles := []string{
		"facebook",
		"instragram",
		"twitter",
	}

	for i := range needles {
		if strings.Contains(e.WebSite, needles[i]) {
			return false
		}
	}

	return true
}

func (e *Entry) Validate() error {
	if e.Title == "" {
		return fmt.Errorf("title is empty")
	}

	if e.Category == "" {
		return fmt.Errorf("category is empty")
	}

	return nil
}

func (e *Entry) CsvHeaders() []string {
	return []string{
		"input_id",
		"link",
		"title",
		"category",
		"address",
		"open_hours",
		"popular_times",
		"website",
		"phone",
		"plus_code",
		"review_count",
		"review_rating",
		"reviews_per_rating",
		"latitude",
		"longitude",
		"cid",
		"status",
		"descriptions",
		"reviews_link",
		"thumbnail",
		"timezone",
		"price_range",
		"data_id",
		"images",
		"reservations",
		"order_online",
		"menu",
		"owner",
		"complete_address",
		"about",
		"user_reviews",
		"user_reviews_extended",
		"emails",
		"place_id",
		"place_id_url",
	}
}

func (e *Entry) CsvRow() []string {
	return []string{
		e.ID,
		e.Link,
		e.Title,
		e.Category,
		e.Address,
		stringify(e.OpenHours),
		stringify(e.PopularTimes),
		e.WebSite,
		e.Phone,
		e.PlusCode,
		stringify(e.ReviewCount),
		stringify(e.ReviewRating),
		stringify(e.ReviewsPerRating),
		stringify(e.Latitude),
		stringify(e.Longtitude),
		e.Cid,
		e.Status,
		e.Description,
		e.ReviewsLink,
		e.Thumbnail,
		e.Timezone,
		e.PriceRange,
		e.DataID,
		stringify(e.Images),
		stringify(e.Reservations),
		stringify(e.OrderOnline),
		stringify(e.Menu),
		stringify(e.Owner),
		stringify(e.CompleteAddress),
		stringify(e.About),
		stringify(e.UserReviews),
		stringify(e.UserReviewsExtended),
		stringSliceToString(e.Emails),
		e.PlaceID,
		e.PlaceIDURL,
	}
}

func (e *Entry) AddExtraReviews(pages [][]byte) {
	if len(pages) == 0 {
		return
	}

	for _, page := range pages {
		reviews := extractReviews(page)
		if len(reviews) > 0 {
			e.UserReviewsExtended = append(e.UserReviewsExtended, reviews...)
		}
	}
}

func extractReviews(data []byte) []Review {
	if len(data) >= 4 && string(data[0:4]) == `)]}'` {
		data = data[4:] // Skip security prefix
	}

	var jd []any
	if err := json.Unmarshal(data, &jd); err != nil {
		fmt.Printf("Error unmarshalling JSON: %v\n", err)
		return nil
	}

	reviewsI := getNthElementAndCast[[]any](jd, 2)

	return parseReviews(reviewsI)
}

//nolint:gomnd // it's ok, I need the indexes
func EntryFromJSON(raw []byte, reviewCountOnly ...bool) (entry Entry, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered from panic: %v stack: %s", r, debug.Stack())

			return
		}
	}()

	onlyReviewCount := false

	if len(reviewCountOnly) == 1 && reviewCountOnly[0] {
		onlyReviewCount = true
	}

	var jd []any
	if err := json.Unmarshal(raw, &jd); err != nil {
		return entry, err
	}

	if len(jd) < 7 {
		return entry, fmt.Errorf("invalid json")
	}

	darray, ok := jd[6].([]any)
	if !ok {
		return entry, fmt.Errorf("invalid json")
	}

	entry.ReviewCount = int(getNthElementAndCast[float64](darray, 4, 8))

	if onlyReviewCount {
		return entry, nil
	}

	entry.Link = getNthElementAndCast[string](darray, 27)
	entry.Title = getNthElementAndCast[string](darray, 11)

	categoriesI := getNthElementAndCast[[]any](darray, 13)

	entry.Categories = make([]string, len(categoriesI))
	for i := range categoriesI {
		entry.Categories[i], _ = categoriesI[i].(string)
	}

	if len(entry.Categories) > 0 {
		entry.Category = entry.Categories[0]
	}

	entry.Address = strings.TrimSpace(
		strings.TrimPrefix(getNthElementAndCast[string](darray, 18), entry.Title+","),
	)
	entry.OpenHours = getHours(darray)
	entry.PopularTimes = getPopularTimes(darray)
	entry.WebSite = cleanWebsiteURL(getNthElementAndCast[string](darray, 7, 0))
	entry.Phone = getNthElementAndCast[string](darray, 178, 0, 0)
	entry.PlusCode = getNthElementAndCast[string](darray, 183, 2, 2, 0)
	entry.ReviewRating = getNthElementAndCast[float64](darray, 4, 7)
	entry.Latitude = getNthElementAndCast[float64](darray, 9, 2)
	entry.Longtitude = getNthElementAndCast[float64](darray, 9, 3)
	entry.Cid = getNthElementAndCast[string](jd, 25, 3, 0, 13, 0, 0, 1)
	entry.Status = getNthElementAndCast[string](darray, 34, 4, 4)
	entry.Description = getNthElementAndCast[string](darray, 32, 1, 1)
	entry.ReviewsLink = getNthElementAndCast[string](darray, 4, 3, 0)
	entry.Thumbnail = getNthElementAndCast[string](darray, 72, 0, 1, 6, 0)
	entry.Timezone = getNthElementAndCast[string](darray, 30)
	entry.PriceRange = normalizeTimeString(getNthElementAndCast[string](darray, 4, 2))
	entry.DataID = getNthElementAndCast[string](darray, 10)

	items := getLinkSource(getLinkSourceParams{
		arr:    getNthElementAndCast[[]any](darray, 171, 0),
		link:   []int{3, 0, 6, 0},
		source: []int{2},
	})

	entry.Images = make([]Image, len(items))

	for i := range items {
		entry.Images[i] = Image{
			Title: items[i].Source,
			Image: items[i].Link,
		}
	}

	entry.Reservations = getLinkSource(getLinkSourceParams{
		arr:    getNthElementAndCast[[]any](darray, 46),
		link:   []int{0},
		source: []int{1},
	})

	orderOnlineI := getNthElementAndCast[[]any](darray, 75, 0, 1, 2)

	if len(orderOnlineI) == 0 {
		orderOnlineI = getNthElementAndCast[[]any](darray, 75, 0, 0, 2)
	}

	entry.OrderOnline = getLinkSource(getLinkSourceParams{
		arr:    orderOnlineI,
		link:   []int{1, 2, 0},
		source: []int{0, 0},
	})

	entry.Menu = LinkSource{
		Link:   getNthElementAndCast[string](darray, 38, 0),
		Source: getNthElementAndCast[string](darray, 38, 1),
	}

	entry.Owner = Owner{
		ID:   getNthElementAndCast[string](darray, 57, 2),
		Name: getNthElementAndCast[string](darray, 57, 1),
	}

	if entry.Owner.ID != "" {
		entry.Owner.Link = fmt.Sprintf("https://www.google.com/maps/contrib/%s", entry.Owner.ID)
	}

	entry.CompleteAddress = Address{
		Borough:    getNthElementAndCast[string](darray, 183, 1, 0),
		Street:     getNthElementAndCast[string](darray, 183, 1, 1),
		City:       getNthElementAndCast[string](darray, 183, 1, 3),
		PostalCode: getNthElementAndCast[string](darray, 183, 1, 4),
		State:      getNthElementAndCast[string](darray, 183, 1, 5),
		Country:    getNthElementAndCast[string](darray, 183, 1, 6),
	}

	aboutI := getNthElementAndCast[[]any](darray, 100, 1)

	for i := range aboutI {
		el := getNthElementAndCast[[]any](aboutI, i)
		about := About{
			ID:   getNthElementAndCast[string](el, 0),
			Name: getNthElementAndCast[string](el, 1),
		}

		optsI := getNthElementAndCast[[]any](el, 2)

		for j := range optsI {
			opt := Option{
				Enabled: (getNthElementAndCast[float64](optsI, j, 2, 1, 0, 0)) == 1,
				Name:    getNthElementAndCast[string](optsI, j, 1),
			}

			if opt.Name != "" {
				about.Options = append(about.Options, opt)
			}
		}

		entry.About = append(entry.About, about)
	}

	entry.ReviewsPerRating = map[int]int{
		1: int(getNthElementAndCast[float64](darray, 175, 3, 0)),
		2: int(getNthElementAndCast[float64](darray, 175, 3, 1)),
		3: int(getNthElementAndCast[float64](darray, 175, 3, 2)),
		4: int(getNthElementAndCast[float64](darray, 175, 3, 3)),
		5: int(getNthElementAndCast[float64](darray, 175, 3, 4)),
	}

	reviewsI := getNthElementAndCast[[]any](darray, 175, 9, 0, 0)
	entry.UserReviews = make([]Review, 0, len(reviewsI))

	return entry, nil
}

func parseReviews(reviewsI []any) []Review {
	ans := make([]Review, 0, len(reviewsI))

	for i := range reviewsI {
		el := getNthElementAndCast[[]any](reviewsI, i, 0)

		time := getNthElementAndCast[[]any](el, 2, 2, 0, 1, 21, 6, 8)

		profilePic, err := decodeURL(getNthElementAndCast[string](el, 1, 4, 5, 1))
		if err != nil {
			profilePic = ""
		}

		review := Review{
			Name:           getNthElementAndCast[string](el, 1, 4, 5, 0),
			ProfilePicture: profilePic,
			When: func() string {
				if len(time) < 3 {
					return ""
				}

				return fmt.Sprintf("%v-%v-%v", time[0], time[1], time[2])
			}(),
			Rating:      int(getNthElementAndCast[float64](el, 2, 0, 0)),
			Description: getNthElementAndCast[string](el, 2, 15, 0, 0),
		}

		if review.Name == "" {
			continue
		}

		optsI := getNthElementAndCast[[]any](el, 2, 2, 0, 1, 21, 7)

		for j := range optsI {
			val := getNthElementAndCast[string](optsI, j)
			if val != "" {
				review.Images = append(review.Images, val[2:])
			}
		}

		ans = append(ans, review)
	}

	return ans
}

type getLinkSourceParams struct {
	arr    []any
	source []int
	link   []int
}

func getLinkSource(params getLinkSourceParams) []LinkSource {
	var result []LinkSource

	for i := range params.arr {
		item := getNthElementAndCast[[]any](params.arr, i)

		el := LinkSource{
			Source: getNthElementAndCast[string](item, params.source...),
			Link:   getNthElementAndCast[string](item, params.link...),
		}
		if el.Link != "" && el.Source != "" {
			result = append(result, el)
		}
	}

	return result
}

//nolint:gomnd // it's ok, I need the indexes
func getHours(darray []any) map[string][]string {
	// Try multiple known index locations where Google has stored hours
	// Google frequently changes these indices, so we try several
	knownPaths := [][]int{
		{203, 0}, // New (Nov 2025) per field report
		{34, 1},  // legacy
		{30, 1},
		{35, 1},
		{33, 1},
		{34, 0},
		{31, 1},
		// Some newer shapes embed hours under different nodes
		{100, 3},
		{84, 3},
	}

	// 1) Try known direct paths with textual weekday -> times mapping
	var items []any
	for _, path := range knownPaths {
		if len(path) == 2 {
			items = getNthElementAndCast[[]any](darray, path[0], path[1])
			if looksLikeHoursData(items) {
				return hoursFromDayStringArray(items)
			}
		}
	}

	// 2) Deep recursive search for arrays like [["Monday", ...], ...]
	if items = findHoursByPattern(darray); len(items) > 0 {
		return hoursFromDayStringArray(items)
	}

	// 3) Deep recursive search for numeric pattern: [[day,[ [start,end], ... ]], ...]
	if num := findHoursNumericPattern(darray); len(num) > 0 {
		return num
	}

	// Not found
	return map[string][]string{}
}

// looksLikeHoursData validates if an array appears to be hours data
// by checking if it contains at least 3 valid weekday entries
func looksLikeHoursData(items []any) bool {
	if len(items) < 3 {
		return false
	}

	weekdays := map[string]bool{
		"monday": true, "tuesday": true, "wednesday": true, "thursday": true,
		"friday": true, "saturday": true, "sunday": true,
		// common abbreviations
		"mon": true, "tue": true, "tues": true, "wed": true, "thu": true, "thur": true, "thurs": true, "fri": true, "sat": true, "sun": true,
	}

	validCount := 0
	for _, item := range items {
		if itemArr, ok := item.([]any); ok && len(itemArr) >= 2 {
			if dayName, ok := itemArr[0].(string); ok && weekdays[strings.ToLower(dayName)] {
				validCount++
			}
		}
	}

	return validCount >= 3
}

// findHoursByPattern searches for hours data by pattern matching
// Hours data typically has the structure: [["Monday", ["time"]], ["Tuesday", ["time"]], ...]
func findHoursByPattern(arr []any) []any {
	weekdays := map[string]bool{
		"monday": true, "tuesday": true, "wednesday": true, "thursday": true,
		"friday": true, "saturday": true, "sunday": true,
		// common abbreviations
		"mon": true, "tue": true, "tues": true, "wed": true, "thu": true, "thur": true, "thurs": true, "fri": true, "sat": true, "sun": true,
	}

	var searchRecursive func([]any, int) []any
	searchRecursive = func(current []any, depth int) []any {
		if depth > 10 || len(current) == 0 {
			return nil
		}

		// Check if current array itself is hours data
		// Format: [["Monday", ...], ["Tuesday", ...], ...]
		if len(current) >= 2 {
			validCount := 0
			for _, item := range current {
				if itemArr, ok := item.([]any); ok && len(itemArr) >= 2 {
					if dayName, ok := itemArr[0].(string); ok && weekdays[strings.ToLower(dayName)] {
						validCount++
					}
				}
			}
			// If we found at least 3 valid weekdays, this is likely hours data
			if validCount >= 3 {
				return current
			}
		}

		// Search through each element
		for _, item := range current {
			if subArr, ok := item.([]any); ok {
				// Check if this sub-array is hours data
				if found := searchRecursive(subArr, depth+1); found != nil {
					return found
				}
			}
		}

		return nil
	}

	return searchRecursive(arr, 0)
}

// hoursFromDayStringArray converts an array like [["Monday", ["9 AM–5 PM"]], ...] into a map
func hoursFromDayStringArray(items []any) map[string][]string {
	hours := make(map[string][]string, len(items))
	normalize := func(day string) string {
		switch strings.ToLower(day) {
		case "mon", "monday":
			return "Monday"
		case "tue", "tues", "tuesday":
			return "Tuesday"
		case "wed", "wednesday":
			return "Wednesday"
		case "thu", "thur", "thurs", "thursday":
			return "Thursday"
		case "fri", "friday":
			return "Friday"
		case "sat", "saturday":
			return "Saturday"
		case "sun", "sunday":
			return "Sunday"
		default:
			return day
		}
	}

	for _, item := range items {
		itemArr, ok := item.([]any)
		if !ok || len(itemArr) < 2 {
			continue
		}

		day, ok := itemArr[0].(string)
		if !ok {
			continue
		}
		day = normalize(day)

		// array of strings times (old structure index 1)
		if timesI, ok := itemArr[1].([]any); ok && len(timesI) > 0 {
			times := make([]string, 0, len(timesI))
			for i := range timesI {
				if timeStr, ok := timesI[i].(string); ok && timeStr != "" {
					times = append(times, normalizeTimeString(timeStr))
				}
			}
			if len(times) > 0 {
				hours[day] = times
			}
		} else if timeStr, ok := itemArr[1].(string); ok && timeStr != "" {
			hours[day] = []string{normalizeTimeString(timeStr)}
		} else if timeSlotsI, ok := itemArr[3].([]any); ok && len(timeSlotsI) > 0 { // new structure index 3
			// New format: each slot is [formatted_string, [[hour, min], [hour, min]]]
			times := make([]string, 0, len(timeSlotsI))
			for _, slot := range timeSlotsI {
				if slotArr, ok := slot.([]any); ok && len(slotArr) > 0 {
					if timeStr, ok := slotArr[0].(string); ok && timeStr != "" {
						times = append(times, normalizeTimeString(timeStr))
					}
				}
			}
			if len(times) > 0 {
				hours[day] = times
			}
		}
	}

	return hours
}

// findHoursNumericPattern searches for arrays like [[day,[ [start,end], ... ]], ...]
// where day is 1..7 (Mon..Sun) and start/end are integers representing minutes or seconds since midnight.
func findHoursNumericPattern(arr []any) map[string][]string {
	dayMap := map[int]string{1: "Monday", 2: "Tuesday", 3: "Wednesday", 4: "Thursday", 5: "Friday", 6: "Saturday", 7: "Sunday"}

	var res map[string][]string

	var dfs func([]any, int)
	dfs = func(cur []any, depth int) {
		if res != nil || depth > 12 || len(cur) == 0 {
			return
		}

		// Does this look like [[day, intervals], ...] with day as number and intervals as [[a,b], ...]?
		if len(cur) >= 3 { // at least 3 weekdays to be confident
			valid := 0
			tmp := make(map[string][][2]int, 7)
			for _, it := range cur {
				pair, ok := it.([]any)
				if !ok || len(pair) < 2 {
					continue
				}
				dayF, ok := pair[0].(float64)
				if !ok {
					continue
				}
				dayName, ok := dayMap[int(dayF)]
				if !ok {
					continue
				}
				intervalsArr, ok := pair[1].([]any)
				if !ok {
					continue
				}
				intervals := make([][2]int, 0, len(intervalsArr))
				for _, inter := range intervalsArr {
					iv, ok := inter.([]any)
					if !ok || len(iv) < 2 {
						continue
					}
					a, aok := iv[0].(float64)
					b, bok := iv[1].(float64)
					if !aok || !bok {
						continue
					}
					intervals = append(intervals, [2]int{int(a), int(b)})
				}
				if len(intervals) > 0 {
					tmp[dayName] = intervals
					valid++
				}
			}

			if valid >= 3 {
				// Convert to strings
				out := make(map[string][]string, len(tmp))
				for day, ivs := range tmp {
					for _, iv := range ivs {
						out[day] = append(out[day], formatInterval(iv[0], iv[1]))
					}
				}
				res = out
				return
			}
		}

		for _, it := range cur {
			if sub, ok := it.([]any); ok {
				dfs(sub, depth+1)
				if res != nil {
					return
				}
			}
		}
	}

	dfs(arr, 0)
	if res == nil {
		return map[string][]string{}
	}
	return res
}

// formatInterval converts [start,end] into a human-readable string.
// It tries to detect whether the numbers are minutes (<= 1440) or seconds (<= 86400).
func formatInterval(a, b int) string {
	toHM := func(x int) (int, int) {
		h := x / 60
		m := x % 60
		return h, m
	}

	// Detect units
	minutes := true
	if a > 1440 || b > 1440 { // likely seconds
		minutes = false
	}

	var h1, m1, h2, m2 int
	if minutes {
		h1, m1 = toHM(a)
		h2, m2 = toHM(b)
	} else {
		// seconds to minutes
		h1, m1 = toHM(a / 60)
		h2, m2 = toHM(b / 60)
	}

	return fmt.Sprintf("%02d:%02d-%02d:%02d", h1, m1, h2, m2)
}

// normalizeTimeString replaces exotic unicode spaces/dashes with ASCII so that CSV viewers on Windows
// (e.g., Excel with non-UTF-8 defaults) don't show mojibake like "â€¯" or "â€“".
func normalizeTimeString(s string) string {
	// Replace various narrow/thin/nb spaces with regular space
	replacer := strings.NewReplacer(
		"\u202F", " ", // NARROW NO-BREAK SPACE
		"\u00A0", " ", // NO-BREAK SPACE
		"\u2009", " ", // THIN SPACE
		"\u200A", " ",
		"\u2005", " ",
		"\u2006", " ",
		"\u2007", " ",
		"\u2008", " ",
		"\u2002", " ",
		"\u2003", " ",
		"\u2004", " ",
		"\uFEFF", "",   // ZERO WIDTH NO-BREAK SPACE (BOM)
		"\u200B", "",   // ZERO WIDTH SPACE
		// Dashes/minus variants to ASCII hyphen
		"\u2013", "-", // EN DASH
		"\u2014", "-", // EM DASH
		"\u2212", "-", // MINUS SIGN
		"–", "-",
		"—", "-",
	)
	out := replacer.Replace(s)
	// Collapse multiple spaces
	out = strings.TrimSpace(out)
	for strings.Contains(out, "  ") {
		out = strings.ReplaceAll(out, "  ", " ")
	}
	return out
}

func getPopularTimes(darray []any) map[string]map[int]int {
	items := getNthElementAndCast[[]any](darray, 84, 0) //nolint:gomnd // it's ok, I need the indexes
	popularTimes := make(map[string]map[int]int, len(items))

	dayOfWeek := map[int]string{
		1: "Monday",
		2: "Tuesday",
		3: "Wednesday",
		4: "Thursday",
		5: "Friday",
		6: "Saturday",
		7: "Sunday",
	}

	for ii := range items {
		item, ok := items[ii].([]any)
		if !ok {
			return nil
		}

		day := int(getNthElementAndCast[float64](item, 0))

		timesI := getNthElementAndCast[[]any](item, 1)

		times := make(map[int]int, len(timesI))

		for i := range timesI {
			t, ok := timesI[i].([]any)
			if !ok {
				return nil
			}

			v, ok := t[1].(float64)
			if !ok {
				return nil
			}

			h, ok := t[0].(float64)
			if !ok {
				return nil
			}

			times[int(h)] = int(v)
		}

		popularTimes[dayOfWeek[day]] = times
	}

	return popularTimes
}

func getNthElementAndCast[T any](arr []any, indexes ...int) T {
	var (
		defaultVal T
		idx        int
	)

	if len(indexes) == 0 {
		return defaultVal
	}

	for len(indexes) > 1 {
		idx, indexes = indexes[0], indexes[1:]

		if idx >= len(arr) {
			return defaultVal
		}

		next := arr[idx]

		if next == nil {
			return defaultVal
		}

		var ok bool

		arr, ok = next.([]any)
		if !ok {
			return defaultVal
		}
	}

	if len(indexes) == 0 || len(arr) == 0 {
		return defaultVal
	}

	ans, ok := arr[indexes[0]].(T)
	if !ok {
		return defaultVal
	}

	return ans
}

func stringSliceToString(s []string) string {
	return strings.Join(s, ", ")
}

func stringify(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return fmt.Sprintf("%f", val)
	case nil:
		return ""
	default:
		d, _ := json.Marshal(v)
		return string(d)
	}
}

func decodeURL(url string) (string, error) {
	quoted := `"` + strings.ReplaceAll(url, `"`, `\"`) + `"`

	unquoted, err := strconv.Unquote(quoted)
	if err != nil {
		return "", fmt.Errorf("failed to decode URL: %v", err)
	}

	return unquoted, nil
}

type EntryWithDistance struct {
	Entry    *Entry
	Distance float64
}

func filterAndSortEntriesWithinRadius(entries []*Entry, lat, lon, radius float64) []*Entry {
	withinRadiusIterator := func(yield func(EntryWithDistance) bool) {
		for _, entry := range entries {
			distance := entry.haversineDistance(lat, lon)
			if distance <= radius {
				if !yield(EntryWithDistance{Entry: entry, Distance: distance}) {
					return
				}
			}
		}
	}

	entriesWithDistance := slices.Collect(iter.Seq[EntryWithDistance](withinRadiusIterator))

	slices.SortFunc(entriesWithDistance, func(a, b EntryWithDistance) int {
		switch {
		case a.Distance < b.Distance:
			return -1
		case a.Distance > b.Distance:
			return 1
		default:
			return 0
		}
	})

	resultIterator := func(yield func(*Entry) bool) {
		for _, e := range entriesWithDistance {
			if !yield(e.Entry) {
				return
			}
		}
	}

	return slices.Collect(iter.Seq[*Entry](resultIterator))
}

// cleanWebsiteURL extracts the actual URL from Google redirect URLs
// Google Maps often returns URLs like "/url?q=http://example.com/&..."
// This function extracts the actual URL from the "q" parameter
func cleanWebsiteURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	// Check if this is a Google redirect URL
	if strings.HasPrefix(rawURL, "/url?") {
		// Parse the query parameters
		parsedURL, err := url.Parse(rawURL)
		if err != nil {
			return rawURL
		}

		// Extract the "q" parameter which contains the actual URL
		actualURL := parsedURL.Query().Get("q")
		if actualURL != "" {
			return actualURL
		}
	}

	return rawURL
}
