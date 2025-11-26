# Windows Fix Script for Google Maps Scraper
# This script fixes the --single-process Chrome flag issue that causes browser crashes on Windows

Write-Host "Fixing Google Maps Scraper for Windows..." -ForegroundColor Cyan

# Set GOWORK off to avoid workspace issues
$env:GOWORK = "off"

# Step 1: Create vendor folder
Write-Host "Creating vendor folder..." -ForegroundColor Yellow
go mod tidy
go mod vendor

# Step 2: Fix the scrapemate library
$filePath = "vendor\github.com\gosom\scrapemate\adapters\fetchers\jshttp\jshttp.go"

if (Test-Path $filePath) {
    Write-Host "Patching scrapemate library..." -ForegroundColor Yellow
    
    $content = Get-Content $filePath -Raw
    
    # Replace the problematic --single-process flag
    $oldPattern = '`--single-process`,'
    $newPattern = '// `--single-process`, // REMOVED: Causes browser crashes on Windows'
    
    $newContent = $content -replace [regex]::Escape($oldPattern), $newPattern
    
    Set-Content $filePath -Value $newContent -NoNewline
    
    Write-Host "Patch applied successfully!" -ForegroundColor Green
} else {
    Write-Host "Error: Could not find $filePath" -ForegroundColor Red
    exit 1
}

# Step 3: Build the scraper
Write-Host "Building google-maps-scraper.exe..." -ForegroundColor Yellow
go build -mod=vendor -o google-maps-scraper.exe

if ($LASTEXITCODE -eq 0) {
    Write-Host ""
    Write-Host "========================================" -ForegroundColor Green
    Write-Host "Build successful!" -ForegroundColor Green
    Write-Host "========================================" -ForegroundColor Green
    Write-Host ""
    Write-Host "You can now run the scraper with:" -ForegroundColor Cyan
    Write-Host "  .\google-maps-scraper.exe -input gmapsdata\categories.txt -results results.csv -depth 3 -exit-on-inactivity 3m" -ForegroundColor White
} else {
    Write-Host "Build failed!" -ForegroundColor Red
    exit 1
}
