package gmaps

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/google/uuid"
	"github.com/gosom/google-maps-scraper/exiter"
	"github.com/gosom/scrapemate"
	"github.com/mcnijman/go-emailaddress"
	"github.com/playwright-community/playwright-go"
)

// Maximum number of emails to extract from a single page to avoid false positives
const maxEmailsPerPage = 50

type EmailExtractJobOptions func(*EmailExtractJob)

type EmailExtractJob struct {
	scrapemate.Job

	Entry       *Entry
	ExitMonitor exiter.Exiter
}

func NewEmailJob(parentID string, entry *Entry, opts ...EmailExtractJobOptions) *EmailExtractJob {
	const (
		defaultPrio       = scrapemate.PriorityHigh
		defaultMaxRetries = 2 // Retry up to 2 times for transient errors (increased from 0)
	)

	job := EmailExtractJob{
		Job: scrapemate.Job{
			ID:         uuid.New().String(),
			ParentID:   parentID,
			Method:     "GET",
			URL:        entry.WebSite,
			MaxRetries: defaultMaxRetries,
			Priority:   defaultPrio,
		},
	}

	job.Entry = entry

	for _, opt := range opts {
		opt(&job)
	}

	return &job
}

func WithEmailJobExitMonitor(exitMonitor exiter.Exiter) EmailExtractJobOptions {
	return func(j *EmailExtractJob) {
		j.ExitMonitor = exitMonitor
	}
}

func (j *EmailExtractJob) Process(ctx context.Context, resp *scrapemate.Response) (any, []scrapemate.IJob, error) {
	defer func() {
		resp.Document = nil
		resp.Body = nil
	}()

	defer func() {
		if j.ExitMonitor != nil {
			j.ExitMonitor.IncrPlacesCompleted(1)
		}
	}()

	log := scrapemate.GetLoggerFromContext(ctx)

	log.Info("Processing email job", "jobid", j.ID, "url", j.URL)

	// if html fetch failed just return the entry without email
	if resp.Error != nil {
		log.Info("Email extraction failed due to fetch error", "jobid", j.ID, "error", resp.Error)
		return j.Entry, nil, nil
	}

	doc, ok := resp.Document.(*goquery.Document)
	if !ok {
		log.Info("Email extraction skipped - invalid document", "jobid", j.ID)
		return j.Entry, nil, nil
	}

	emails := docEmailExtractor(doc)
	if len(emails) == 0 {
		emails = regexEmailExtractor(resp.Body)
	}

	j.Entry.Emails = emails

	if len(emails) > 0 {
		log.Info("Extracted emails", "jobid", j.ID, "count", len(emails))
	} else {
		log.Info("No emails found", "jobid", j.ID)
	}

	return j.Entry, nil, nil
}

func (j *EmailExtractJob) ProcessOnFetchError() bool {
	return true
}

func (j *EmailExtractJob) BrowserActions(ctx context.Context, page playwright.Page) scrapemate.Response {
	var resp scrapemate.Response

	log := scrapemate.GetLoggerFromContext(ctx)
	log.Info("Processing email job", "jobid", j.ID, "url", j.GetURL())

	pageResponse, err := page.Goto(j.GetURL(), playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(30000), // 30 second timeout for website loading (increased from 20s)
	})
	if err != nil {
		// Distinguish between permanent and temporary failures
		errorStr := err.Error()
		isPermanentError := strings.Contains(errorStr, "ERR_NAME_NOT_RESOLVED") ||
			strings.Contains(errorStr, "ERR_CONNECTION_REFUSED") ||
			strings.Contains(errorStr, "ERR_ADDRESS_UNREACHABLE") ||
			strings.Contains(errorStr, "ERR_TOO_MANY_REDIRECTS") ||
			strings.Contains(errorStr, "SSL") ||
			strings.Contains(errorStr, "ERR_CERT_")

		if isPermanentError {
			log.Info("Email extraction failed - permanent error (DNS/SSL/redirect/connection)", "jobid", j.ID, "error", err)
			// Set MaxRetries to 0 for this specific job to avoid wasting retries
			j.MaxRetries = 0
		} else {
			log.Info("Email extraction navigation failed - may be retryable", "jobid", j.ID, "error", err)
		}
		resp.Error = err

		return resp
	}

	const defaultTimeout = 5000

	err = page.WaitForURL(page.URL(), playwright.PageWaitForURLOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(defaultTimeout),
	})
	if err != nil {
		log.Info("Email extraction URL wait failed", "jobid", j.ID, "error", err)
		resp.Error = err

		return resp
	}

	resp.URL = pageResponse.URL()
	resp.StatusCode = pageResponse.Status()
	resp.Headers = make(http.Header, len(pageResponse.Headers()))

	for k, v := range pageResponse.Headers() {
		resp.Headers.Add(k, v)
	}

	// Check for HTTP error status codes (4xx and 5xx)
	if resp.StatusCode >= 400 {
		log.Info("Email extraction failed - HTTP error", "jobid", j.ID, "statusCode", resp.StatusCode)
		resp.Error = fmt.Errorf("status code %d", resp.StatusCode)
		// Don't retry for 404 (Not Found), 410 (Gone), 403 (Forbidden) - these are permanent
		if resp.StatusCode == 404 || resp.StatusCode == 410 || resp.StatusCode == 403 {
			j.MaxRetries = 0
		}
		return resp
	}

	body, err := page.Content()
	if err != nil {
		log.Info("Email extraction content retrieval failed", "jobid", j.ID, "error", err)
		resp.Error = err

		return resp
	}

	resp.Body = []byte(body)

	return resp
}

func docEmailExtractor(doc *goquery.Document) []string {
	seen := map[string]bool{}

	var emails []string

	doc.Find("a[href^='mailto:']").Each(func(_ int, s *goquery.Selection) {
		if len(emails) >= maxEmailsPerPage {
			return
		}
		mailto, exists := s.Attr("href")
		if exists {
			value := strings.TrimPrefix(mailto, "mailto:")
			if email, err := getValidEmail(value); err == nil {
				if !seen[email] && isLikelyRealEmail(email) {
					emails = append(emails, email)
					seen[email] = true
				}
			}
		}
	})

	return emails
}

func regexEmailExtractor(body []byte) []string {
	seen := map[string]bool{}

	var emails []string

	addresses := emailaddress.Find(body, false)
	for i := range addresses {
		if len(emails) >= maxEmailsPerPage {
			break
		}
		email := addresses[i].String()
		if !seen[email] && isLikelyRealEmail(email) {
			emails = append(emails, email)
			seen[email] = true
		}
	}

	return emails
}

// isLikelyRealEmail filters out common false positive email patterns
func isLikelyRealEmail(email string) bool {
	email = strings.ToLower(email)

	// Filter out common false positive patterns
	falsePositivePatterns := []string{
		"@example.com",
		"@test.com",
		"@localhost",
		"@sentry.io",
		"@wixpress.com",
		"@email.com",
		"@domain.com",
		"@yourdomain",
		"@placeholder",
		"noreply@",
		"no-reply@",
		"donotreply@",
		"mailer-daemon@",
		"postmaster@",
		"@2x.", // Image naming convention (e.g., icon@2x.png)
		"@3x.",
		".png",
		".jpg",
		".gif",
		".svg",
		".webp",
	}

	for _, pattern := range falsePositivePatterns {
		if strings.Contains(email, pattern) {
			return false
		}
	}

	// Filter out emails that look like version strings or hashes
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}

	localPart := parts[0]
	// Filter out very long local parts (likely hashes or encoded data)
	if len(localPart) > 64 {
		return false
	}

	// Filter out local parts that are just numbers or hex strings
	if isHexString(localPart) && len(localPart) > 8 {
		return false
	}

	return true
}

// isHexString checks if a string looks like a hex hash
func isHexString(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func getValidEmail(s string) (string, error) {
	email, err := emailaddress.Parse(strings.TrimSpace(s))
	if err != nil {
		return "", err
	}

	return email.String(), nil
}
