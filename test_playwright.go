//go:build ignore

package main

import (
	"fmt"
	"log"
	"time"

	"github.com/playwright-community/playwright-go"
)

func main() {
	fmt.Println("Starting Playwright test...")

	fmt.Println("Launching browser...")

	// Start Playwright
	pw, err := playwright.Run()
	if err != nil {
		log.Fatalf("could not start playwright: %v", err)
	}
	defer pw.Stop()

	// Launch browser in headful mode (visible)
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(false), // Show the browser window
		Args: []string{
			"--disable-blink-features=AutomationControlled",
			"--no-sandbox",
			"--disable-dev-shm-usage",
		},
	})
	if err != nil {
		log.Fatalf("could not launch browser: %v", err)
	}
	defer browser.Close()

	fmt.Println("Browser launched successfully!")

	// Create a new page
	page, err := browser.NewPage()
	if err != nil {
		log.Fatalf("could not create page: %v", err)
	}

	fmt.Println("Navigating to Google Maps search...")

	// Navigate to Google Maps with a search query (like the scraper does)
	url := "https://www.google.com/maps/search/restaurant?hl=en"
	fmt.Printf("URL: %s\n", url)

	_, err = page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded, // Same as scraper
		Timeout:   playwright.Float(30000),                   // 30 second timeout
	})
	if err != nil {
		log.Fatalf("could not navigate to Google Maps: %v", err)
	}

	fmt.Println("Successfully loaded Google Maps!")

	// Wait a bit for the page to render
	time.Sleep(2 * time.Second)

	// Take a screenshot
	_, err = page.Screenshot(playwright.PageScreenshotOptions{
		Path: playwright.String("google_maps_test.png"),
	})
	if err != nil {
		log.Printf("Warning: could not take screenshot: %v", err)
	} else {
		fmt.Println("Screenshot saved to google_maps_test.png")
	}

	// Get the page title
	title, err := page.Title()
	if err != nil {
		log.Printf("Warning: could not get title: %v", err)
	} else {
		fmt.Printf("Page title: %s\n", title)
	}

	// Check for the feed element (like the scraper does)
	fmt.Println("Looking for results feed...")
	sel := `div[role='feed']`
	_, err = page.WaitForSelector(sel, playwright.PageWaitForSelectorOptions{
		Timeout: playwright.Float(10000),
	})
	if err != nil {
		fmt.Printf("Feed not found: %v\n", err)
	} else {
		fmt.Println("Found results feed! Scraper should work.")
	}

	// Wait for 10 seconds so you can see the browser
	fmt.Println("Keeping browser open for 10 seconds...")
	time.Sleep(10 * time.Second)

	fmt.Println("Test completed successfully!")
}
