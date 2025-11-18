package gmaps

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
