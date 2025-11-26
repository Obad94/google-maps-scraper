# Google Maps Scraper - Local Setup Script for Windows
# This script helps you set up the scraper without Docker

Write-Host "===========================================================" -ForegroundColor Cyan
Write-Host "   Google Maps Scraper - Local Setup (No Docker)" -ForegroundColor Cyan
Write-Host "===========================================================" -ForegroundColor Cyan
Write-Host ""

# Check if Go is installed
Write-Host "[1/4] Checking Go installation..." -ForegroundColor Yellow
$goVersion = $null
try {
    $goVersion = & go version 2>&1
    if ($LASTEXITCODE -eq 0) {
        Write-Host "Go is installed: $goVersion" -ForegroundColor Green
    }
    else {
        throw "Go not found"
    }
}
catch {
    Write-Host "Go is not installed!" -ForegroundColor Red
    Write-Host ""
    Write-Host "Please install Go from: https://go.dev/dl/" -ForegroundColor Yellow
    Write-Host "Recommended version: Go 1.23.0 or higher" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "After installing Go, restart PowerShell and run this script again." -ForegroundColor Yellow
    exit 1
}

Write-Host ""

# Download dependencies
Write-Host "[2/4] Downloading Go dependencies..." -ForegroundColor Yellow
& go mod download
if ($LASTEXITCODE -ne 0) {
    Write-Host "Failed to download dependencies" -ForegroundColor Red
    exit 1
}
Write-Host "Dependencies downloaded successfully" -ForegroundColor Green
Write-Host ""

# Build the application
Write-Host "[3/4] Building the application..." -ForegroundColor Yellow
& go build -o google-maps-scraper.exe
if ($LASTEXITCODE -ne 0) {
    Write-Host "Build failed" -ForegroundColor Red
    exit 1
}
Write-Host "Application built successfully: google-maps-scraper.exe" -ForegroundColor Green
Write-Host ""

# Install Playwright
Write-Host "[4/4] Installing Playwright and Chromium browser..." -ForegroundColor Yellow
Write-Host "This may take a few minutes..." -ForegroundColor Gray
$env:PLAYWRIGHT_INSTALL_ONLY = "1"
& .\google-maps-scraper.exe
if ($LASTEXITCODE -ne 0) {
    Write-Host "Playwright installation failed" -ForegroundColor Red
    exit 1
}
Write-Host "Playwright installed successfully" -ForegroundColor Green
Write-Host ""

# Setup complete
Write-Host "===========================================================" -ForegroundColor Green
Write-Host "   Setup Complete!" -ForegroundColor Green
Write-Host "===========================================================" -ForegroundColor Green
Write-Host ""
Write-Host "You can now run the scraper with:" -ForegroundColor Cyan
Write-Host ""
Write-Host "  Command Line Mode:" -ForegroundColor Yellow
Write-Host "  .\google-maps-scraper.exe -input example-queries.txt -results results.csv -exit-on-inactivity 3m" -ForegroundColor White
Write-Host ""
Write-Host "  Web UI Mode:" -ForegroundColor Yellow
Write-Host "  .\google-maps-scraper.exe -web -data-folder gmapsdata" -ForegroundColor White
Write-Host "  Then open http://localhost:8080 in your browser" -ForegroundColor Gray
Write-Host ""
Write-Host "  With Email Extraction:" -ForegroundColor Yellow
Write-Host "  .\google-maps-scraper.exe -input example-queries.txt -results results.csv -email -exit-on-inactivity 3m" -ForegroundColor White
Write-Host ""
Write-Host "For more options, run: .\google-maps-scraper.exe -h" -ForegroundColor Cyan
Write-Host ""
