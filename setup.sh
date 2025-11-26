#!/bin/bash
# Google Maps Scraper - Local Setup Script for Linux/Mac
# This script helps you set up the scraper without Docker

set -e

echo "==========================================================="
echo "   Google Maps Scraper - Local Setup (No Docker)"
echo "==========================================================="
echo ""

# Check if Go is installed
echo "[1/4] Checking Go installation..."
if command -v go &> /dev/null; then
    GO_VERSION=$(go version)
    echo "✓ Go is installed: $GO_VERSION"
else
    echo "✗ Go is not installed!"
    echo ""
    echo "Please install Go from: https://go.dev/dl/"
    echo "Recommended version: Go 1.23.0 or higher"
    echo ""
    echo "After installing Go, restart your terminal and run this script again."
    exit 1
fi

echo ""

# Download dependencies
echo "[2/4] Downloading Go dependencies..."
go mod download
echo "✓ Dependencies downloaded successfully"
echo ""

# Build the application
echo "[3/4] Building the application..."
go build -o google-maps-scraper
echo "✓ Application built successfully: google-maps-scraper"
echo ""

# Install Playwright
echo "[4/4] Installing Playwright and Chromium browser..."
echo "This may take a few minutes..."
PLAYWRIGHT_INSTALL_ONLY=1 ./google-maps-scraper
echo "✓ Playwright installed successfully"
echo ""

# Setup complete
echo "==========================================================="
echo "   Setup Complete!"
echo "==========================================================="
echo ""
echo "You can now run the scraper with:"
echo ""
echo "  Command Line Mode:"
echo "  ./google-maps-scraper -input example-queries.txt -results results.csv -exit-on-inactivity 3m"
echo ""
echo "  Web UI Mode:"
echo "  ./google-maps-scraper -web -data-folder gmapsdata"
echo "  Then open http://localhost:8080 in your browser"
echo ""
echo "  With Email Extraction:"
echo "  ./google-maps-scraper -input example-queries.txt -results results.csv -email -exit-on-inactivity 3m"
echo ""
echo "For more options, run: ./google-maps-scraper -h"
echo ""
