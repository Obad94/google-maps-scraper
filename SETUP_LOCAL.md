# Local Setup Guide (Without Docker)

This guide walks you through setting up Google Maps Scraper on your local machine without requiring Docker. This is the recommended approach for development and testing.

## Table of Contents
- [Why Run Locally?](#why-run-locally)
- [Prerequisites](#prerequisites)
- [Quick Setup (Automated)](#quick-setup-automated)
- [Manual Setup](#manual-setup)
- [Troubleshooting](#troubleshooting)
- [Next Steps](#next-steps)

## Why Run Locally?

Running without Docker provides several advantages:
- **Faster startup** - No container overhead
- **Easier debugging** - Direct access to logs and processes
- **Better performance** - Native execution
- **Simpler development** - Edit code and rebuild quickly
- **Lower resource usage** - No Docker daemon required

## Prerequisites

Before you begin, ensure you have:

1. **Go 1.23.0 or higher**
   - Download from: https://go.dev/dl/
   - Verify installation: `go version`

2. **Git**
   - Download from: https://git-scm.com/downloads
   - Verify installation: `git --version`

3. **Sufficient Disk Space**
   - ~500MB for Go dependencies
   - ~300MB for Playwright/Chromium

## Quick Setup (Automated)

We provide setup scripts that handle everything automatically:

### Windows (PowerShell)

```powershell
# Clone the repository
git clone https://github.com/gosom/google-maps-scraper.git
cd google-maps-scraper

# Run the setup script
.\setup.ps1
```

### Linux/Mac

```bash
# Clone the repository
git clone https://github.com/gosom/google-maps-scraper.git
cd google-maps-scraper

# Make script executable and run
chmod +x setup.sh
./setup.sh
```

The script will:
1. ✅ Check if Go is installed
2. ✅ Download all Go dependencies
3. ✅ Build the application
4. ✅ Install Playwright and Chromium browser

## Manual Setup

If you prefer to set up manually or want to understand each step:

### Step 1: Clone the Repository

```bash
git clone https://github.com/gosom/google-maps-scraper.git
cd google-maps-scraper
```

### Step 2: Download Dependencies

```bash
go mod download
```

This downloads all required Go packages specified in `go.mod`.

### Step 3: Build the Application

**Windows (PowerShell):**
```powershell
go build -o google-maps-scraper.exe
```

**Linux/Mac:**
```bash
go build -o google-maps-scraper
```

This creates the executable in your current directory.

### Step 4: Install Playwright and Chromium

This is a **one-time setup** that downloads the Chromium browser for web scraping:

**Windows (PowerShell):**
```powershell
$env:PLAYWRIGHT_INSTALL_ONLY="1"
.\google-maps-scraper.exe
```

**Linux/Mac:**
```bash
PLAYWRIGHT_INSTALL_ONLY=1 ./google-maps-scraper
```

The first time you run this, it will download ~200MB of browser files.

### Step 5: Verify Installation

Check that everything works:

**Windows:**
```powershell
.\google-maps-scraper.exe -h
```

**Linux/Mac:**
```bash
./google-maps-scraper -h
```

You should see a list of available command-line options.

## Running the Scraper

Once setup is complete, you can run the scraper in various modes:

### Command Line Mode

**Basic scraping:**
```bash
# Windows
.\google-maps-scraper.exe -input example-queries.txt -results results.csv -exit-on-inactivity 3m

# Linux/Mac
./google-maps-scraper -input example-queries.txt -results results.csv -exit-on-inactivity 3m
```

**With email extraction:**
```bash
# Windows
.\google-maps-scraper.exe -input example-queries.txt -results results.csv -email -exit-on-inactivity 3m

# Linux/Mac
./google-maps-scraper -input example-queries.txt -results results.csv -email -exit-on-inactivity 3m
```

### Web UI Mode

Start the web interface:

**Windows:**
```powershell
mkdir gmapsdata
.\google-maps-scraper.exe -web -data-folder gmapsdata
```

**Linux/Mac:**
```bash
mkdir -p gmapsdata
./google-maps-scraper -web -data-folder gmapsdata
```

Then open http://localhost:8080 in your browser.

### Using Proxies

```bash
# Single proxy
./google-maps-scraper -input queries.txt -results output.csv -proxies "http://user:pass@proxy.com:8080"

# Multiple proxies (comma-separated)
./google-maps-scraper -input queries.txt -results output.csv -proxies "http://proxy1.com:8080,http://proxy2.com:8080"

# Proxies from file
./google-maps-scraper -input queries.txt -results output.csv -proxies proxies.txt
```

### Advanced Options

```bash
# Nearby mode with custom radius
./google-maps-scraper -nearby-mode -geo "40.7128,-74.0060" -input queries.txt -radius 5000 -zoom 500m

# Fast mode (reduced data, faster results)
./google-maps-scraper -fast-mode -input queries.txt -results output.csv

# Extract all reviews (up to ~300 per place)
./google-maps-scraper -input queries.txt -results output.json -json -extra-reviews

# Custom concurrency and depth
./google-maps-scraper -input queries.txt -results output.csv -c 4 -depth 20
```

## Troubleshooting

### "go: command not found"

**Problem:** Go is not installed or not in your PATH.

**Solution:**
1. Download Go from https://go.dev/dl/
2. Follow the installation instructions for your OS
3. Restart your terminal
4. Verify: `go version`

### "Playwright installation failed"

**Problem:** Playwright/Chromium download failed.

**Solutions:**
1. Check your internet connection
2. Try running the install command again:
   ```bash
   PLAYWRIGHT_INSTALL_ONLY=1 ./google-maps-scraper
   ```
3. On Linux, you may need to install system dependencies:
   ```bash
   # Ubuntu/Debian
   sudo apt-get update
   sudo apt-get install -y libnss3 libatk-bridge2.0-0 libdrm2 libxkbcommon0 libgbm1
   ```

### "Build failed" or compilation errors

**Problem:** Go modules or dependencies are missing or outdated.

**Solution:**
```bash
# Clean module cache
go clean -modcache

# Re-download dependencies
go mod download

# Try building again
go build
```

### Permission denied (Linux/Mac)

**Problem:** The executable doesn't have execute permissions.

**Solution:**
```bash
chmod +x google-maps-scraper
chmod +x setup.sh
```

### Chromium not found at runtime

**Problem:** Playwright was not properly installed.

**Solution:**
Run the Playwright installation again:
```bash
PLAYWRIGHT_INSTALL_ONLY=1 ./google-maps-scraper
```

### Windows Defender or Antivirus blocking

**Problem:** Antivirus software may flag the executable or Chromium.

**Solution:**
1. Add the project folder to your antivirus exclusions
2. This is a false positive - the scraper uses Chromium for legitimate web scraping

## Next Steps

After successful setup:

1. **Read the full documentation** in [README.md](README.md)
2. **Try the examples** - Start with `example-queries.txt`
3. **Explore command-line options** - Run `./google-maps-scraper -h`
4. **Join the community** - [Discord](https://discord.gg/fpaAVhNCCu)
5. **Star the project** - https://github.com/gosom/google-maps-scraper

## Environment Variables

Optional environment variables you can set:

- `DISABLE_TELEMETRY=1` - Disable anonymous usage statistics
- `PLAYWRIGHT_BROWSERS_PATH` - Custom path for Playwright browsers
- `GOOGLE_MAPS_API_KEY` - For Browser API mode

## Updating

To update to the latest version:

```bash
# Pull latest changes
git pull origin main

# Rebuild
go build

# If dependencies changed, download them first
go mod download
go build
```

## Performance Tips

1. **Adjust concurrency** - Use `-c` flag to match your CPU cores
2. **Use appropriate depth** - Higher `-depth` values scrape more results but take longer
3. **Enable fast mode** - For quick scans, use `-fast-mode`
4. **Use proxies** - Rotate IPs to avoid rate limiting
5. **Monitor resources** - Each browser instance uses ~200-300MB RAM

## Need Help?

- **Issues**: https://github.com/gosom/google-maps-scraper/issues
- **Discussions**: https://github.com/gosom/google-maps-scraper/discussions
- **Discord**: https://discord.gg/fpaAVhNCCu

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
