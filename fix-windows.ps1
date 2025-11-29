# Windows Fix Script for Google Maps Scraper
# This script fixes Chrome flags that cause browser crashes on Windows
# It patches the scrapemate library to use OS-aware browser flags

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
    Write-Host "Patching scrapemate library for Windows compatibility..." -ForegroundColor Yellow

    $content = Get-Content $filePath -Raw

    # Step 2a: Add runtime import if not already present
    if ($content -notmatch 'import \(\s*"runtime"') {
        Write-Host "  - Adding runtime import..." -ForegroundColor Gray
        $content = $content -replace '(package jshttp\s+import \(\s+"context")', "`$1`n`t`"runtime`""
    }

    # Step 2b: Replace the browser args section with OS-aware implementation
    Write-Host "  - Replacing browser args with OS-aware implementation..." -ForegroundColor Gray

    $oldArgsPattern = [regex]::Escape('Args: []string{
			`--start-maximized`,
			`--no-default-browser-check`,
			`--disable-dev-shm-usage`,
			`--no-sandbox`,
			`--disable-setuid-sandbox`,
			`--no-zygote`,
			`--disable-gpu`,
			`--mute-audio`,
			`--disable-extensions`,
			// `--single-process`, // REMOVED: Causes browser crashes on Windows
			`--disable-breakpad`,
			`--disable-features=TranslateUI,BlinkGenPropertyTrees`,
			`--disable-ipc-flooding-protection`,
			`--enable-features=NetworkService,NetworkServiceInProcess`,
			"--enable-features=NetworkService",
			`--disable-default-apps`,
			`--disable-notifications`,
			`--disable-webgl`,
			`--disable-blink-features=AutomationControlled`,
			"--ignore-certificate-errors",
			"--ignore-certificate-errors-spki-list",
			"--disable-web-security",
		},')

    # New OS-aware implementation
    $newArgsImpl = @'
Args: func() []string {
			// Base args that work on all platforms
			args := []string{
				`--start-maximized`,
				`--no-default-browser-check`,
				`--no-sandbox`,
				`--disable-gpu`,
				`--mute-audio`,
				`--disable-extensions`,
				`--disable-breakpad`,
				`--disable-features=TranslateUI,BlinkGenPropertyTrees`,
				`--disable-ipc-flooding-protection`,
				`--enable-features=NetworkService,NetworkServiceInProcess`,
				"--enable-features=NetworkService",
				`--disable-default-apps`,
				`--disable-notifications`,
				`--disable-webgl`,
				`--disable-blink-features=AutomationControlled`,
				"--ignore-certificate-errors",
				"--ignore-certificate-errors-spki-list",
				"--disable-web-security",
			}

			// Add Linux-specific flags only on Linux
			// These flags cause crashes on Windows:
			// - --disable-dev-shm-usage: Uses /dev/shm which doesn't exist on Windows
			// - --disable-setuid-sandbox: Linux-specific sandbox feature
			// - --no-zygote: Linux-specific process forking optimization
			// - --single-process: Causes browser crashes on Windows
			if runtime.GOOS == "linux" {
				args = append(args,
					`--disable-dev-shm-usage`,
					`--disable-setuid-sandbox`,
					`--no-zygote`,
				)
			}

			return args
		}(),
'@

    # Check if the old pattern exists (might already be patched)
    if ($content -match [regex]::Escape('Args: []string{')) {
        # Find and replace the Args section
        $content = $content -replace 'Args: \[\]string\{[^}]+\}(?:,|\s*})', $newArgsImpl
        Write-Host "  - Browser args updated with OS detection" -ForegroundColor Green
    } else {
        Write-Host "  - File appears to be already patched or has different format" -ForegroundColor Yellow
    }

    Set-Content $filePath -Value $content -NoNewline

    Write-Host "Patch applied successfully!" -ForegroundColor Green
} else {
    Write-Host "Error: Could not find $filePath" -ForegroundColor Red
    Write-Host "Make sure you're running this script from the google-maps-scraper directory" -ForegroundColor Red
    exit 1
}

# Step 3: Build the scraper
Write-Host ""
Write-Host "Building google-maps-scraper.exe..." -ForegroundColor Yellow
go build -mod=vendor -o google-maps-scraper.exe

if ($LASTEXITCODE -eq 0) {
    Write-Host ""
    Write-Host "========================================" -ForegroundColor Green
    Write-Host "Build successful!" -ForegroundColor Green
    Write-Host "========================================" -ForegroundColor Green
    Write-Host ""
    Write-Host "Windows-incompatible Chrome flags removed:" -ForegroundColor Cyan
    Write-Host "  - --disable-dev-shm-usage (Linux /dev/shm)" -ForegroundColor Gray
    Write-Host "  - --disable-setuid-sandbox (Linux-specific)" -ForegroundColor Gray
    Write-Host "  - --no-zygote (Linux process forking)" -ForegroundColor Gray
    Write-Host "  - --single-process (causes crashes)" -ForegroundColor Gray
    Write-Host ""
    Write-Host "These flags are now only used on Linux systems." -ForegroundColor Cyan
    Write-Host ""
    Write-Host "You can now run the scraper with:" -ForegroundColor Cyan
    Write-Host "  .\google-maps-scraper.exe -BrowserAPI -geo `"lat,lon`" -input gmapsdata\categories.txt -results results.csv -zoom 21 -depth 1 -radius 20000 -c 3 -exit-on-inactivity 2m" -ForegroundColor White
} else {
    Write-Host ""
    Write-Host "Build failed!" -ForegroundColor Red
    Write-Host "Check the error messages above for details." -ForegroundColor Red
    exit 1
}
