package gmaps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// PlacesAPIResponse represents the response from Google Places API
type PlacesAPIResponse struct {
	Places []struct {
		ID string `json:"id"`
	} `json:"places"`
}

// FindPlaceID calls the Google Places API to find a place ID based on a text query
// The query should be a combination of the place title and address
func FindPlaceID(query, apiKey string) (string, error) {
	if apiKey == "" {
		return "", fmt.Errorf("API key is empty")
	}

	if query == "" {
		return "", fmt.Errorf("query is empty")
	}

	// Define the API endpoint
	url := "https://places.googleapis.com/v1/places:searchText"

	// Define the headers
	headers := map[string]string{
		"Content-Type":      "application/json",
		"X-Goog-Api-Key":    apiKey,
		"X-Goog-FieldMask":  "places.id",
	}

	// Define the data payload for the POST request
	payload := map[string]interface{}{
		"textQuery": query,
	}

	// Convert payload to JSON
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request payload: %v", err)
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	// Set headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Make the request
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %v", err)
	}

	// Check if the request was successful
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API error: %d, %s", resp.StatusCode, string(body))
	}

	// Parse response
	var response PlacesAPIResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("failed to parse response: %v", err)
	}

	// Check if we got any results
	if len(response.Places) == 0 {
		return "", fmt.Errorf("no place found for query: %s", query)
	}

	// Return the first place ID
	return response.Places[0].ID, nil
}

// EnrichEntryWithPlaceID enriches an Entry with Google Place ID and generates the URL
func EnrichEntryWithPlaceID(entry *Entry, apiKey string) error {
	if entry == nil {
		return fmt.Errorf("entry is nil")
	}

	// Build the query from title and address
	query := entry.Title
	if entry.Address != "" {
		query = fmt.Sprintf("%s %s", entry.Title, entry.Address)
	}

	// Call the Places API
	placeID, err := FindPlaceID(query, apiKey)
	if err != nil {
		// Log the error but don't fail - we'll just leave the fields empty
		fmt.Printf("Warning: Failed to get Place ID for '%s': %v\n", query, err)
		return nil
	}

	// Set the Place ID
	entry.PlaceID = placeID

	// Generate the Google Maps URL with Place ID
	// Format: https://www.google.com/maps/search/?api=1&query=Google&query_place_id={place_id}
	entry.PlaceIDURL = fmt.Sprintf("https://www.google.com/maps/search/?api=1&query=%s&query_place_id=%s",
		url.QueryEscape(entry.Title), placeID)

	return nil
}

// PlacesNearbyRequest represents the request body for Nearby Search (New) API
type PlacesNearbyRequest struct {
	LocationRestriction LocationRestriction `json:"locationRestriction"`
	IncludedTypes       []string            `json:"includedTypes,omitempty"`
	MaxResultCount      int                 `json:"maxResultCount"`
	RankPreference      string              `json:"rankPreference"`
	PageToken           string              `json:"pageToken,omitempty"`
}

// LocationRestriction defines the search area
type LocationRestriction struct {
	Circle Circle `json:"circle"`
}

// Circle defines a circular search area
type Circle struct {
	Center GeoCoordinates `json:"center"`
	Radius float64        `json:"radius"`
}

// GeoCoordinates represents lat/lng coordinates
type GeoCoordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// PlacesNearbyResponse represents the API response from Nearby Search (New)
type PlacesNearbyResponse struct {
	Places        []PlaceResult `json:"places"`
	NextPageToken string        `json:"nextPageToken,omitempty"`
}

// PlaceResult represents a single place from the Nearby Search API
type PlaceResult struct {
	ID          string          `json:"id"`
	DisplayName DisplayNameInfo `json:"displayName,omitempty"`
	Location    LocationInfo    `json:"location,omitempty"`
}

// DisplayNameInfo represents the place name
type DisplayNameInfo struct {
	Text string `json:"text"`
}

// LocationInfo represents the place coordinates
type LocationInfo struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// normalizeToPlaceType converts common search keywords to valid Google Places API types
// See: https://developers.google.com/maps/documentation/places/web-service/place-types
var keywordToPlaceType = map[string]string{
	// Common plurals to singular
	"restaurants":      "restaurant",
	"cafes":            "cafe",
	"bars":             "bar",
	"hotels":           "hotel",
	"hospitals":        "hospital",
	"pharmacies":       "pharmacy",
	"banks":            "bank",
	"atms":             "atm",
	"gyms":             "gym",
	"parks":            "park",
	"schools":          "school",
	"churches":         "church",
	"mosques":          "mosque",
	"temples":          "hindu_temple",
	"supermarkets":     "supermarket",
	"gas stations":     "gas_station",
	"gas_stations":     "gas_station",
	"parking lots":     "parking",
	"parking_lots":     "parking",
	"shopping malls":   "shopping_mall",
	"shopping_malls":   "shopping_mall",
	"movie theaters":   "movie_theater",
	"movie_theaters":   "movie_theater",
	"bakeries":         "bakery",
	"dentists":         "dentist",
	"doctors":          "doctor",
	"lawyers":          "lawyer",
	"salons":           "beauty_salon",
	"spas":             "spa",
	"museums":          "museum",
	"libraries":        "library",
	"airports":         "airport",
	"train stations":   "train_station",
	"train_stations":   "train_station",
	"bus stations":     "bus_station",
	"bus_stations":     "bus_station",
	"car rentals":      "car_rental",
	"car_rentals":      "car_rental",
	"car washes":       "car_wash",
	"car_washes":       "car_wash",
	"laundries":        "laundry",
	"pet stores":       "pet_store",
	"pet_stores":       "pet_store",
	"electronics stores": "electronics_store",
	"electronics_stores": "electronics_store",
	"clothing stores":  "clothing_store",
	"clothing_stores":  "clothing_store",
	"book stores":      "book_store",
	"book_stores":      "book_store",
	"florists":         "florist",
	"furniture stores": "furniture_store",
	"furniture_stores": "furniture_store",
	"hardware stores":  "hardware_store",
	"hardware_stores":  "hardware_store",
	"jewelry stores":   "jewelry_store",
	"jewelry_stores":   "jewelry_store",
	"liquor stores":    "liquor_store",
	"liquor_stores":    "liquor_store",
	"shoe stores":      "shoe_store",
	"shoe_stores":      "shoe_store",
	"convenience stores": "convenience_store",
	"convenience_stores": "convenience_store",
}

// normalizeKeywordToType converts a keyword to a valid Google Places type
func normalizeKeywordToType(keyword string) string {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if placeType, ok := keywordToPlaceType[keyword]; ok {
		return placeType
	}
	return keyword // Return as-is if no mapping found
}

// SearchNearbyPlaces searches for places near a location using Google Places API (New)
// Returns all places (with pagination) and the total request count
// Uses the searchNearby endpoint: https://developers.google.com/maps/documentation/places/web-service/nearby-search
func SearchNearbyPlaces(ctx context.Context, lat, lng, radius float64, categories []string, apiKey string) ([]PlaceResult, int, error) {
	if apiKey == "" {
		return nil, 0, fmt.Errorf("API key is required")
	}

	const baseURL = "https://places.googleapis.com/v1/places:searchNearby"

	var allPlaces []PlaceResult
	requestCount := 0
	nextPageToken := ""

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	for {
		reqBody := PlacesNearbyRequest{
			LocationRestriction: LocationRestriction{
				Circle: Circle{
					Center: GeoCoordinates{
						Latitude:  lat,
						Longitude: lng,
					},
					Radius: radius,
				},
			},
			MaxResultCount: 20, // Maximum allowed by API
			RankPreference: "DISTANCE",
		}

		// Add categories if provided
		if len(categories) > 0 {
			reqBody.IncludedTypes = categories
		}

		// Add page token for pagination
		if nextPageToken != "" {
			reqBody.PageToken = nextPageToken
		}

		// Marshal request body
		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return nil, requestCount, fmt.Errorf("failed to marshal request: %w", err)
		}

		// Create HTTP request
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, requestCount, fmt.Errorf("failed to create request: %w", err)
		}

		// Set headers - minimum fields to reduce cost ($0.017 per request)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Goog-Api-Key", apiKey)
		req.Header.Set("X-Goog-FieldMask", "places.id,places.displayName,places.location")

		// Execute request
		resp, err := client.Do(req)
		if err != nil {
			return nil, requestCount, fmt.Errorf("failed to execute request: %w", err)
		}

		requestCount++

		// Read response body
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, requestCount, fmt.Errorf("failed to read response: %w", err)
		}

		// Check status code
		if resp.StatusCode != http.StatusOK {
			return nil, requestCount, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
		}

		// Parse response
		var apiResp PlacesNearbyResponse
		if err := json.Unmarshal(body, &apiResp); err != nil {
			return nil, requestCount, fmt.Errorf("failed to parse response: %w", err)
		}

		// Append places
		allPlaces = append(allPlaces, apiResp.Places...)

		// Check for next page
		if apiResp.NextPageToken == "" {
			break
		}
		nextPageToken = apiResp.NextPageToken
	}

	return allPlaces, requestCount, nil
}

// PlaceIDToURL converts a Place ID to a Google Maps URL
func PlaceIDToURL(placeID string) string {
	return fmt.Sprintf("https://www.google.com/maps/place/?q=place_id:%s", placeID)
}
