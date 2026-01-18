package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/joho/godotenv"
)

// This utility verifies that your proxy is working correctly
// It checks your IP with and without the proxy to ensure they're different

func main() {
	// Load .env file
	_ = godotenv.Load()

	fmt.Println("=== Proxy Verification Tool ===")
	fmt.Println()

	// Get your real IP (without proxy)
	fmt.Println("1. Checking your real IP (without proxy)...")
	realIP, err := getIP(nil)
	if err != nil {
		fmt.Printf("Error getting real IP: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("   Your real IP: %s\n\n", realIP)

	// Get proxy from environment
	proxyURL := os.Getenv("PROXY")
	if proxyURL == "" {
		fmt.Println("ERROR: No PROXY variable found in .env file")
		os.Exit(1)
	}

	fmt.Printf("2. Testing with proxy: %s\n", proxyURL)

	// Parse proxy URL
	proxy, err := url.Parse(proxyURL)
	if err != nil {
		fmt.Printf("Error parsing proxy URL: %v\n", err)
		os.Exit(1)
	}

	// Get IP through proxy
	proxyIP, err := getIP(proxy)
	if err != nil {
		fmt.Printf("Error getting IP through proxy: %v\n", err)
		fmt.Println("\n❌ PROXY FAILED - Check your proxy configuration!")
		os.Exit(1)
	}
	fmt.Printf("   IP through proxy: %s\n\n", proxyIP)

	// Compare IPs
	if realIP == proxyIP {
		fmt.Println("⚠️  WARNING: IPs are the SAME!")
		fmt.Println("Your proxy is NOT working - your real IP is being exposed!")
		os.Exit(1)
	} else {
		fmt.Println("✅ SUCCESS: IPs are DIFFERENT!")
		fmt.Println("Your proxy is working correctly - your real IP is hidden.")
		fmt.Printf("\nReal IP:  %s\n", realIP)
		fmt.Printf("Proxy IP: %s\n", proxyIP)
	}
}

func getIP(proxy *url.URL) (string, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	if proxy != nil {
		client.Transport = &http.Transport{
			Proxy: http.ProxyURL(proxy),
		}
	}

	// Use multiple IP checking services as fallback
	services := []string{
		"https://api.ipify.org",
		"https://icanhazip.com",
		"https://ifconfig.me/ip",
	}

	for _, service := range services {
		resp, err := client.Get(service)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			continue
		}

		ip := string(body)
		// Clean up response
		ip = trimSpace(ip)

		if ip != "" {
			return ip, nil
		}
	}

	return "", fmt.Errorf("failed to get IP from all services")
}

func trimSpace(s string) string {
	result := ""
	for _, c := range s {
		if c != ' ' && c != '\n' && c != '\r' && c != '\t' {
			result += string(c)
		}
	}
	return result
}
