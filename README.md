# Google Maps Scraper

<p align="center">
  <a href="https://github.com/gosom/google-maps-scraper/stargazers"><img src="https://img.shields.io/github/stars/gosom/google-maps-scraper?style=social" alt="GitHub Stars"></a>
  <a href="https://github.com/gosom/google-maps-scraper/network/members"><img src="https://img.shields.io/github/forks/gosom/google-maps-scraper?style=social" alt="GitHub Forks"></a>
</p>

[![Build Status](https://github.com/gosom/google-maps-scraper/actions/workflows/build.yml/badge.svg)](https://github.com/gosom/google-maps-scraper/actions/workflows/build.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/gosom/google-maps-scraper)](https://goreportcard.com/report/github.com/gosom/google-maps-scraper)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Discord](https://img.shields.io/badge/Discord-Join%20Chat-7289DA?logo=discord&logoColor=white)](https://discord.gg/fpaAVhNCCu)

**A powerful, free, and open-source Google Maps scraper** for extracting business data at scale. Available as CLI, Web UI, REST API, or deployable to Kubernetes/AWS Lambda.

> This is a customized fork with additional features: **BrowserAPI Mode**, **Hybrid Mode**, **Nearby Mode**, and **Windows compatibility fixes**.

![Example GIF](img/example.gif)

> **Love this project?** A star helps others discover it and motivates continued development. [Become a sponsor](https://github.com/sponsors/gosom) to directly support new features and maintenance.

---

## 🎯 What Problem Are You Solving?

I'd love to understand how you're using this tool! 
Please comment on [this discussion](https://github.com/gosom/google-maps-scraper/discussions/184) with your use case:

- 🎯 **Lead Generation** - Finding potential customers
- 📊 **Market Research** - Understanding competitors/markets
- 📁 **Database Building** - Creating/maintaining business lists
- 💡 **Other** - Tell me more!

---

## Join Our Community

[![Discord](https://img.shields.io/badge/Discord-Join%20Our%20Server-7289DA?logo=discord&logoColor=white&style=for-the-badge)](https://discord.gg/fpaAVhNCCu)

Join our Discord to get help, share ideas, and connect with other users!

---

## Sponsors

<p align="center"><i>This project is made possible by our amazing sponsors</i></p>

### Premium Sponsors

**No time for code? Extract ALL Google Maps listings at country-scale in 2 clicks, without keywords or limits** 👉 [Try it now for free](https://scrap.io?utm_medium=ads&utm_source=github_gosom_gmap_scraper)

[![Extract ALL Google Maps Listings](./img/premium_scrap_io.png)](https://scrap.io?utm_medium=ads&utm_source=github_gosom_gmap_scraper)

<hr>

<table>
<tr>
<td><img src="./img/SerpApi-logo-w.png" alt="SerpApi Logo" width="100"></td>
<td>
<b>At SerpApi, we scrape public data from Google Maps and other top search engines.</b>

You can find the full list of our APIs here: [https://serpapi.com/search-api](https://serpapi.com/search-api)
</td>
</tr>
</table>

[![SerpApi Banner](./img/SerpApi-banner.png)](https://serpapi.com/?utm_source=google-maps-scraper)

<hr>

**G Maps Extractor**  
A no-code Google Maps scraper that pulls business leads from Google Maps in one click.

- 📇 **Includes** emails, social profiles, phone numbers, addresses, reviews, images and more.
- 📥 **Export** to CSV · Excel · JSON  
- 🔌 **API** Support: Extract data via [API](https://gmapsextractor.com/google-maps-api?utm_source=github&utm_medium=banner&utm_campaign=gosom)
- 🎁 **Free**: Get your first **1,000 leads** today  
[Get Started for Free](https://gmapsextractor.com?utm_source=github&utm_medium=banner&utm_campaign=gosom)

[![Gmaps Extractor](./img/gmaps-extractor-banner.png)](https://gmapsextractor.com?utm_source=github&utm_medium=banner&utm_campaign=gosom)

<hr>

### Special Thanks to:

[![Google Maps API for easy SERP scraping](https://www.searchapi.io/press/v1/svg/searchapi_logo_black_h.svg)](https://www.searchapi.io/google-maps?via=gosom)
**Google Maps API for easy SERP scraping**

<hr>

[Evomi](https://evomi.com?utm_source=github&utm_medium=banner&utm_campaign=gosom-maps) is your Swiss Quality Proxy Provider, starting at **$0.49/GB**

[![Evomi Banner](https://my.evomi.com/images/brand/cta.png)](https://evomi.com?utm_source=github&utm_medium=banner&utm_campaign=gosom-maps)

<hr>

[Scrapeless](https://www.scrapeless.com/): One-click to scrape Google search results, supporting 15+ SERP scenarios such as Google Maps/Scholars/Jobs, $0.1/thousand queries, 0.2s response.

**[👉 Free Trial](https://app.scrapeless.com/passport/login?utm_source=gosom&utm_campaign=google-maps)**

![Scrapeless](./img/scrapeless_dark.png#gh-dark-mode-only)
![Scrapeless](./img/scrapeless_light.png#gh-light-mode-only)

<hr>

[Decodo's proxies](https://visit.decodo.com/APVbbx) with #1 response time in the market

Collect data without facing CAPTCHAs, IP bans, or geo-restrictions
- ● 125M+ IP pool
- ● 195+ locations worldwide  
- ● 24/7 tech support
- ● Extensive documentation

**[Start your 3-day free trial with 100MB →](https://visit.decodo.com/APVbbx)**

![Decodo](./img/decodo.png)

---

## Why Use This Scraper?

| | |
|---|---|
| **Completely Free & Open Source** | MIT licensed, no hidden costs or usage limits |
| **Multiple Interfaces** | CLI, Web UI, REST API - use what fits your workflow |
| **High Performance** | ~120 places/minute with optimized concurrency |
| **34+ Data Points** | Business details, reviews, emails, coordinates, place_id, and more |
| **Production Ready** | Scale from a single machine to Kubernetes clusters |
| **Flexible Output** | CSV, JSON, PostgreSQL, S3, LeadsDB, or custom plugins |
| **Proxy Support** | Built-in SOCKS5/HTTP/HTTPS proxy rotation |
| **Multiple Modes** | Normal, Fast, Nearby, Hybrid, and BrowserAPI modes |
| **Browser Engines** | Playwright (default) or go-rod (lightweight alternative) |

---

## What's Next After Scraping?

Once you've collected your data, you'll need to manage, deduplicate, and work with your leads. **[LeadsDB](https://getleadsdb.com/)** is a companion tool designed exactly for this:

- **Automatic Deduplication** - Import from multiple scrapes without worrying about duplicates
- **AI Agent Ready** - Query and manage leads with natural language via MCP
- **Advanced Filtering** - Combine filters with AND/OR logic on any field
- **Export Anywhere** - CSV, JSON, or use the REST API

The scraper has [built-in LeadsDB integration](#export-to-leadsdb) - just add your API key and leads flow directly into your database.

**[Start free with 500 leads](https://getleadsdb.com/)**

---

## Table of Contents

- [Quick Start](#quick-start)
  - [Web UI](#web-ui)
  - [Command Line](#command-line)
  - [REST API](#rest-api)
- [Installation](#installation)
  - [Using Docker](#using-docker)
  - [Local Setup (Without Docker)](#local-setup-without-docker)
  - [Windows Compatibility](#windows-compatibility)
- [Features](#features)
- [Scraping Modes](#scraping-modes)
  - [Normal Mode](#normal-mode)
  - [Fast Mode](#fast-mode)
  - [Nearby Mode](#nearby-mode)
  - [Hybrid Mode](#hybrid-mode)
  - [BrowserAPI Mode](#browserapi-mode)
- [Extracted Data Points](#extracted-data-points)
- [Configuration](#configuration)
  - [Command Line Options](#command-line-options)
  - [Using Proxies](#using-proxies)
  - [Email Extraction](#email-extraction)
- [Advanced Usage](#advanced-usage)
  - [PostgreSQL Database Provider](#postgresql-database-provider)
  - [Kubernetes Deployment](#kubernetes-deployment)
  - [Custom Writer Plugins](#custom-writer-plugins)
  - [Export to LeadsDB](#export-to-leadsdb)
- [Performance](#performance)
- [References](#references)
- [License](#license)

---

## Quick Start

### Web UI

> **⚠️ PostgreSQL Required:** The Web UI requires a PostgreSQL database. Set the `DATABASE_URL` environment variable before starting:
> ```bash
> export DATABASE_URL="postgres://user:password@localhost:5432/dbname"
> ```

Start the web interface with a single command:

```bash
mkdir -p gmapsdata && docker run \
  -e DATABASE_URL="postgres://user:password@localhost:5432/dbname" \
  -v $PWD/gmapsdata:/gmapsdata \
  -p 8080:8080 \
  gosom/google-maps-scraper -data-folder /gmapsdata
```

Then open http://localhost:8080 in your browser.

Or download the [binary release](https://github.com/gosom/google-maps-scraper/releases) for your platform.

> **Note:** Results take at least 3 minutes to appear (minimum configured runtime).
> 
> **macOS Users:** Docker command may not work. See [MacOS Instructions](MacOS%20instructions.md).

### Command Line

```bash
touch results.csv && docker run \
  -v $PWD/example-queries.txt:/example-queries \
  -v $PWD/results.csv:/results.csv \
  gosom/google-maps-scraper \
  -depth 1 \
  -input /example-queries \
  -results /results.csv \
  -exit-on-inactivity 3m
```

> **Tip:** Use `gosom/google-maps-scraper:latest-rod` for the Rod version with faster container startup.

**Want emails?** Add the `-email` flag.

**Want all reviews (up to ~300)?** Add `--extra-reviews` and use `-json` output.

### REST API

When running the web server, a full REST API is available:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/jobs` | POST | Create a new scraping job |
| `/api/v1/jobs` | GET | List all jobs |
| `/api/v1/jobs/{id}` | GET | Get job details |
| `/api/v1/jobs/{id}` | DELETE | Delete a job |
| `/api/v1/jobs/{id}/download` | GET | Download results as CSV |
| `/api/v1/jobs/{id}/results` | GET | Get job results as JSON (Google Maps API format) |

Full OpenAPI 3.0.3 documentation available at:
- **Swagger UI** (Interactive API testing): http://localhost:8080/api/swagger
- **Redoc** (Read-only documentation): http://localhost:8080/api/docs

---

## 🔐 Multi-Tenancy & Authentication

The scraper includes a built-in **organization-based multi-tenancy system** with secure user authentication and API key management.

### Architecture Overview

**Organization-Based Isolation:**
- All data (jobs, API keys) is scoped to **organizations**, not individual users
- Users can belong to multiple organizations with different roles
- Complete data isolation ensures users only see their organization's resources

**Authentication Methods:**
- **Session-based** (cookies) for Web UI access
- **API Keys** for programmatic/REST API access
- Hybrid support: APIs work with both methods

### Getting Started

#### 1. User Registration

```bash
# Register a new account (Web UI)
Visit http://localhost:8080/register

# Or use the API
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "password": "secure_password",
    "first_name": "John",
    "last_name": "Doe"
  }'
```

**On registration:**
- A default organization is automatically created for the user
- The user becomes the organization **owner** with full permissions

#### 2. User Login

```bash
# Login (Web UI)
Visit http://localhost:8080/login

# Or use the API
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "password": "secure_password"
  }'
```

Returns a `session_token` stored in an HTTP-only cookie for web access.

#### 3. API Key Management

**Create an API Key** (for programmatic access):

```bash
# Via Web UI: http://localhost:8080/apikeys

# Or via API (requires session authentication)
curl -X POST http://localhost:8080/api/v1/apikeys \
  -H "Cookie: session_token=YOUR_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Production API Key",
    "expires_at": null
  }'
```

**Use API Key in Requests:**

```bash
# Option 1: Authorization Bearer header (recommended)
curl -H "Authorization: Bearer gms_your_api_key_here" \
  http://localhost:8080/api/v1/jobs

# Option 2: X-API-Key header
curl -H "X-API-Key: gms_your_api_key_here" \
  http://localhost:8080/api/v1/jobs

# Option 3: Query parameter
curl "http://localhost:8080/api/v1/jobs?api_key=gms_your_api_key_here"
```

### Roles & Permissions

| Role | Permissions |
|------|-------------|
| **Owner** | Full control: manage organization, billing, delete org, all admin permissions |
| **Admin** | Manage members, jobs, API keys, and organization settings |
| **Member** | Create and manage own jobs, view organization jobs |
| **Viewer** | Read-only access to jobs and data |

### Data Isolation

**How it works:**
- Jobs are created with `organization_id` and `created_by` fields
- API keys belong to a specific organization
- All database queries filter by organization automatically
- Cross-organization access is denied (returns 404 to prevent info leakage)

**Security guarantees:**
- ✅ Users can only access their organization's jobs and API keys
- ✅ API keys are scoped to one organization
- ✅ Database queries enforce organization filtering
- ✅ Role-based permissions restrict destructive operations

### Organization Management

**Invite Users to Your Organization:**

```bash
# Via API
curl -X POST http://localhost:8080/api/v1/organizations/{org_id}/members \
  -H "Authorization: Bearer YOUR_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "teammate@example.com",
    "role": "member"
  }'
```

**List Organizations:**

```bash
curl http://localhost:8080/api/v1/organizations \
  -H "Authorization: Bearer YOUR_SESSION_TOKEN"
```

### Security Best Practices

1. **API Key Security:**
   - Store API keys securely (environment variables, secret managers)
   - Never commit API keys to version control
   - Rotate keys periodically
   - Use separate keys for different environments (dev, staging, prod)

2. **Password Security:**
   - Passwords are hashed with bcrypt (cost factor 10)
   - Minimum 8 characters recommended
   - Consider using password managers

3. **Session Security:**
   - Sessions expire after 30 days
   - Stored as SHA-256 hashes
   - HTTP-only cookies prevent XSS attacks
   - SameSite: Strict mode enabled

4. **Access Control:**
   - Assign minimum required roles (principle of least privilege)
   - Review organization members regularly
   - Revoke access for departing team members

### Migration from Single-User Setup

If upgrading from a version without multi-tenancy:
1. Existing jobs/API keys will be assigned to a "Legacy Organization"
2. First registered user becomes the owner
3. No data is lost during migration

### Troubleshooting

**"Organization context required" errors:**
- Ensure you're authenticated (session cookie or API key)
- Verify your account belongs to an organization
- Check that your session hasn't expired

**Can't see jobs/API keys:**
- Confirm you're viewing the correct organization
- Check your role has sufficient permissions
- Verify the resources belong to your organization

**API key authentication failing:**
- Ensure the key starts with `gms_`
- Check the key hasn't been revoked or expired
- Verify the key belongs to the correct organization

---

## Installation

### Using Docker

Two Docker image variants are available:

| Image | Tag | Browser Engine | Best For |
|-------|-----|----------------|----------|
| Playwright (default) | `latest`, `vX.X.X` | Playwright | Most users, better stability |
| Rod | `latest-rod`, `vX.X.X-rod` | Rod/Chromium | Lightweight, faster startup |

```bash
# Playwright version (default)
docker pull gosom/google-maps-scraper

# Rod version (alternative)
docker pull gosom/google-maps-scraper:latest-rod
```

### Local Setup (Without Docker)

Running directly on your machine with Go installed is the recommended approach for development.

> 📖 **For detailed setup instructions, troubleshooting, and advanced configuration, see [SETUP_LOCAL.md](SETUP_LOCAL.md)**

#### Prerequisites
- **Go 1.23.0 or higher** installed ([Download Go](https://go.dev/dl/))
- **Git** installed

#### Quick Setup (Automated)

We provide setup scripts that automate the entire process:

**Windows (PowerShell):**
```powershell
.\setup.ps1
```

**Linux/Mac:**
```bash
chmod +x setup.sh
./setup.sh
```

#### Manual Setup Steps

1. **Clone the repository**
   ```bash
   git clone https://github.com/gosom/google-maps-scraper.git
   cd google-maps-scraper
   ```

2. **Download Go dependencies**
   ```bash
   go mod download
   ```

3. **Build the application**

   **Windows (PowerShell) - IMPORTANT:**

   On Windows, you must use the fix-windows.ps1 script to build the application. This script patches the vendor code to remove Linux-specific Chrome flags that cause browser crashes on Windows.

   ```powershell
   .\fix-windows.ps1
   ```

   This script will:
   - Create and patch the vendor folder
   - Remove Windows-incompatible Chrome flags (--no-zygote, --disable-dev-shm-usage, --disable-setuid-sandbox, --single-process)
   - Add OS detection to use these flags only on Linux
   - Build google-maps-scraper.exe

   **Linux/Mac (Playwright - default):**
   ```bash
   go build -o google-maps-scraper
   ```

   **Linux/Mac (Rod - alternative):**
   ```bash
   go build -tags rod -o google-maps-scraper
   ```

4. **Install Playwright and Chromium** (one-time setup)

   **Windows (PowerShell):**
   ```powershell
   $env:PLAYWRIGHT_INSTALL_ONLY="1"
   .\google-maps-scraper.exe
   ```

   **Linux/Mac:**
   ```bash
   PLAYWRIGHT_INSTALL_ONLY=1 ./google-maps-scraper
   ```

5. **Run the scraper**
   ```bash
   ./google-maps-scraper -input example-queries.txt -results results.csv -exit-on-inactivity 3m
   ```

6. **Run the Web UI**

   **PostgreSQL Setup (Required for Web UI):**
   ```bash
   # Start PostgreSQL (using Docker Compose)
   docker-compose -f docker-compose.dev.yaml up -d

   # Or set DATABASE_URL to your existing PostgreSQL instance
   export DATABASE_URL="postgres://postgres:postgres@localhost:5432/postgres"
   ```

   **Start the Web UI:**
   ```bash
   mkdir -p gmapsdata
   ./google-maps-scraper -web -data-folder gmapsdata
   ```
   Then open http://localhost:8080 in your browser.

### Windows Compatibility

**IMPORTANT for Windows Users:**

The scraper uses Chromium browser via Playwright, which requires specific configuration on Windows. The vendor library includes Linux-specific Chrome flags that cause browser crashes on Windows.

#### The Problem
These Chrome flags cause crashes on Windows:
- `--disable-dev-shm-usage` - Uses Linux `/dev/shm` which doesn't exist on Windows
- `--disable-setuid-sandbox` - Linux-specific sandbox feature
- `--no-zygote` - Linux-specific process forking optimization
- `--single-process` - Known to cause browser crashes on Windows

#### The Solution
Use the `fix-windows.ps1` script which:
1. Patches the vendor code to add OS detection
2. Removes Windows-incompatible flags automatically
3. Ensures these flags are only used on Linux
4. Builds a stable Windows executable

```powershell
# Run this from PowerShell in the project directory
.\fix-windows.ps1
```

#### After Fixing
You can safely run with multiple categories:
```powershell
.\google-maps-scraper.exe -BrowserAPI -geo "lat,lon" -input gmapsdata\categories.txt -results results.csv -zoom 21 -depth 1 -radius 20000 -c 3 -exit-on-inactivity 2m
```

**Note:** If you build without using `fix-windows.ps1`, you may experience browser crashes when scraping multiple categories or a large number of places (>20).

---

## Features

| Feature | Description |
|---------|-------------|
| **34+ Data Points** | Business name, address, phone, website, reviews, coordinates, place_id, and more |
| **Email Extraction** | Optional crawling of business websites for email addresses |
| **Multiple Output Formats** | CSV, JSON, PostgreSQL, S3, LeadsDB, or custom plugins |
| **Proxy Support** | SOCKS5, HTTP, HTTPS with authentication |
| **Scalable Architecture** | Single machine to Kubernetes cluster |
| **REST API** | Programmatic control for automation |
| **Web UI** | User-friendly browser interface |
| **Multiple Scraping Modes** | Normal, Fast, Nearby, Hybrid, BrowserAPI |
| **Browser Engines** | Playwright (default) or go-rod (lightweight) |
| **AWS Lambda** | Serverless execution support (experimental) |

---

## Scraping Modes

### Normal Mode
The default mode. Performs standard Google Maps searches with scrolling to extract results.

```bash
./google-maps-scraper -input queries.txt -results results.csv -depth 10
```

### Fast Mode

Fast mode returns up to 21 results per query, ordered by distance. Useful for quick data collection with basic fields.

```bash
./google-maps-scraper \
  -input queries.txt \
  -results results.csv \
  -fast-mode \
  -zoom 15 \
  -radius 5000 \
  -geo '37.7749,-122.4194'
```

Required parameters:
- `-zoom` - Zoom level
- `-radius` - Search radius in meters
- `-geo` - Latitude,longitude coordinates

> **Warning:** Fast mode is in Beta. You may experience blocking.

### Nearby Mode

Nearby mode simulates the "Search Nearby" feature in Google Maps, returning places closest to your specified coordinates.

```bash
./google-maps-scraper \
  -nearby-mode \
  -geo "21.030625,105.819332" \
  -input categories.txt \
  -results nearby_results.csv \
  -zoom 500m \
  -radius 1000 \
  -depth 17 \
  -email
```

#### Important Parameters

**`-zoom`** (Default: 2000m for nearby mode, 15z for regular mode)
- **Unified zoom parameter** that auto-detects between zoom levels (z) and meters (m)
- **For nearby mode**: Use meters (e.g., `500m`, `2000m`) to control map view distance
- **For regular mode**: Use zoom levels (e.g., `15z`, `18z`) for Google Maps zoom (0-21)
- **Auto-detection**: Values 1-21 = zoom level, 51+ = meters

**`-radius`** (Default: 10000 meters)
- Filters which places are saved to your results
- Only places within this distance from center point are kept

### Hybrid Mode

Hybrid mode combines multiple search strategies for maximum coverage:
1. **Phase 1a**: Fast Mode (HTTP API) - quick, ~21 results per query
2. **Phase 1b**: Normal Mode (Browser) - comprehensive browser-based search
3. **Phase 1c**: Initial Nearby Mode (Browser) - proximity search at input coordinates
4. **Phase 2**: Nested Nearby Mode (Browser) - nearby search at all found locations

```bash
./google-maps-scraper \
  -hybrid-mode \
  -geo "21.030625,105.819332" \
  -input categories.txt \
  -results hybrid_results.csv \
  -zoom 21 \
  -depth 2 \
  -email \
  -radius 20000 \
  -c 3 \
  -exit-on-inactivity 3m
```

### BrowserAPI Mode

BrowserAPI mode uses Google Places API to get nearby places, then scrapes each place and runs nearby search for comprehensive coverage.

```bash
./google-maps-scraper \
  -BrowserAPI \
  -geo "21.030625,105.819332" \
  -input categories.txt \
  -results browserapi_results.csv \
  -zoom 21 \
  -depth 2 \
  -email \
  -radius 20000 \
  -c 3 \
  -exit-on-inactivity 1m
```

#### API Example (Create Job)
```json
{
  "name": "Job 1", 
  "keywords": ["restaurant"],
  "lang": "en", 
  "zoom": 20, 
  "lat": "21.030625", 
  "lon": "105.819332", 
  "browserapi_mode": true,
  "radius": 10000, 
  "depth": 1, 
  "email": true,
  "max_time": 10000, 
  "proxies": []
}
```

---

## Extracted Data Points

<details>
<summary><strong>Click to expand all 34 data points</strong></summary>

| # | Field | Description |
|---|-------|-------------|
| 1 | `input_id` | Internal identifier for the input query |
| 2 | `link` | Direct URL to the Google Maps listing |
| 3 | `title` | Business name |
| 4 | `category` | Business type (e.g., Restaurant, Hotel) |
| 5 | `address` | Street address |
| 6 | `open_hours` | Operating hours |
| 7 | `popular_times` | Visitor traffic patterns |
| 8 | `website` | Official business website |
| 9 | `phone` | Contact phone number |
| 10 | `plus_code` | Location shortcode |
| 11 | `review_count` | Total number of reviews |
| 12 | `review_rating` | Average star rating |
| 13 | `reviews_per_rating` | Breakdown by star rating |
| 14 | `latitude` | GPS latitude |
| 15 | `longitude` | GPS longitude |
| 16 | `cid` | Google's unique Customer ID |
| 17 | `status` | Business status (open/closed/temporary) |
| 18 | `descriptions` | Business description |
| 19 | `reviews_link` | Direct link to reviews |
| 20 | `thumbnail` | Thumbnail image URL |
| 21 | `timezone` | Business timezone |
| 22 | `price_range` | Price level ($, $$, $$$) |
| 23 | `data_id` | Internal Google Maps identifier |
| 24 | `images` | Associated image URLs |
| 25 | `reservations` | Reservation booking link |
| 26 | `order_online` | Online ordering link |
| 27 | `menu` | Menu link |
| 28 | `owner` | Owner-claimed status |
| 29 | `complete_address` | Full formatted address |
| 30 | `about` | Additional business info |
| 31 | `user_reviews` | Customer reviews (text, rating, timestamp) |
| 32 | `emails` | Extracted email addresses (requires `-email` flag) |
| 33 | `user_reviews_extended` | Extended reviews up to ~300 (requires `-extra-reviews`) |
| 34 | `place_id` | Google's unique place ID |
| 35 | `place_id_url` | Direct URL using place ID |

</details>

**Custom Input IDs:** Define your own IDs in the input file:
```
Matsuhisa Athens #!#MyCustomID
```

---

## Configuration

### Command Line Options

```
Usage: google-maps-scraper [options]

Core Options:
  -input string       Path to input file with queries (one per line)
  -results string     Output file path (default: stdout)
  -json              Output JSON instead of CSV
  -depth int         Max scroll depth in results (default: 10)
  -c int             Concurrency level (default: half of CPU cores)

Email & Reviews:
  -email             Extract emails from business websites
  -extra-reviews     Collect extended reviews (up to ~300)

Location Settings:
  -lang string       Language code, e.g., 'de' for German (default: "en")
  -geo string        Coordinates for search, e.g., '37.7749,-122.4194'
  -zoom string       Zoom level (1-21z) or distance in meters (51m+)
  -radius float      Search radius in meters (default: 10000)

Scraping Modes:
  -fast-mode         Fast mode with reduced data (~21 results)
  -nearby-mode       Nearby search mode (right-click search simulation)
  -hybrid-mode       Hybrid mode: fast + normal + nearby combined
  -BrowserAPI        BrowserAPI mode: Places API + browser scraping

Web Server:
  -web               Run web server mode
  -addr string       Server address (default: ":8080")
  -data-folder       Data folder for web runner (default: "webdata")

Database:
  -dsn string        PostgreSQL connection string
  -produce           Produce seed jobs only (requires -dsn)

Proxy:
  -proxies string    Comma-separated proxy list
                     Format: protocol://user:pass@host:port

Export:
  -leadsdb-api-key   Export directly to LeadsDB

Advanced:
  -exit-on-inactivity duration    Exit after inactivity (e.g., '5m')
  -debug                          Show browser window
  -writer string                  Custom writer plugin
  -disable-page-reuse             Disable page reuse in playwright
```

Run `./google-maps-scraper -h` for the complete list.

### Using Proxies

For larger scraping jobs, proxies help avoid rate limiting:

```bash
./google-maps-scraper \
  -input queries.txt \
  -results results.csv \
  -proxies 'socks5://user:pass@host:port,http://host2:port2' \
  -depth 1 -c 2
```

**Supported protocols:** `socks5`, `socks5h`, `http`, `https`

#### Proxy Providers

| Provider | Highlight | Offer |
|----------|-----------|-------|
| [Decodo](https://visit.decodo.com/APVbbx) | #1 response time, 125M+ IPs | [3-day free trial](https://visit.decodo.com/APVbbx) |
| [Evomi](https://evomi.com?utm_source=github&utm_medium=banner&utm_campaign=gosom-maps) | Swiss quality, 150+ countries | From $0.49/GB |

See the [Decodo integration guide](decodo.md) for setup instructions.

### Email Extraction

Email extraction is **disabled by default**. When enabled, the scraper visits each business website to find email addresses.

```bash
./google-maps-scraper -input queries.txt -results results.csv -email
```

> **Note:** Email extraction increases processing time significantly.

---

## Advanced Usage

### PostgreSQL Database Provider

> **📌 Important:** PostgreSQL is **required** for the Web UI and multi-tenancy features. The CLI can still run without PostgreSQL when using file-based output (CSV/JSON).

For distributed scraping across multiple machines:

**1. Start PostgreSQL:**
```bash
docker-compose -f docker-compose.dev.yaml up -d
```

**2. Seed the jobs:**
```bash
./google-maps-scraper \
  -dsn "postgres://postgres:postgres@localhost:5432/postgres" \
  -produce \
  -input example-queries.txt \
  -lang en
```

**3. Run scrapers (on multiple machines):**
```bash
./google-maps-scraper \
  -c 2 \
  -depth 1 \
  -dsn "postgres://postgres:postgres@localhost:5432/postgres"
```

### Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: google-maps-scraper
spec:
  replicas: 3
  selector:
    matchLabels:
      app: google-maps-scraper
  template:
    metadata:
      labels:
        app: google-maps-scraper
    spec:
      containers:
      - name: google-maps-scraper
        image: gosom/google-maps-scraper:latest
        args: ["-c", "1", "-depth", "10", "-dsn", "postgres://user:pass@host:5432/db"]
        resources:
          requests:
            memory: "512Mi"
            cpu: "500m"
```

> **Note:** The headless browser requires significant CPU/memory resources.

### Custom Writer Plugins

Create custom output handlers using Go plugins:

**1. Write the plugin** (see `examples/plugins/example_writer.go`)

**2. Build:**
```bash
go build -buildmode=plugin -tags=plugin -o myplugin.so myplugin.go
```

**3. Run:**
```bash
./google-maps-scraper -writer ~/plugins:MyWriter -input queries.txt
```

### Export to LeadsDB

Skip the CSV files and send leads directly to a managed database:

```bash
./google-maps-scraper \
  -input queries.txt \
  -leadsdb-api-key "your-api-key" \
  -exit-on-inactivity 3m
```

Or via environment variable:
```bash
export LEADSDB_API_KEY="your-api-key"
./google-maps-scraper -input queries.txt -exit-on-inactivity 3m
```

Get your API key at [getleadsdb.com/settings](https://getleadsdb.com/settings).

---

## Performance

**Expected throughput:** ~120 places/minute (with `-c 8 -depth 1`)

| Keywords | Results/Keyword | Total Jobs | Estimated Time |
|----------|-----------------|------------|----------------|
| 100 | 16 | 1,600 | ~13 minutes |
| 1,000 | 16 | 16,000 | ~2.5 hours |
| 10,000 | 16 | 160,000 | ~22 hours |

For large-scale scraping, use the PostgreSQL provider with Kubernetes.

### Telemetry

Anonymous usage statistics are collected for improvement purposes. Opt out:
```bash
export DISABLE_TELEMETRY=1
```

---

## Docker Examples

### PowerShell (Windows)
```powershell
docker run -v ${PWD}\gmapsdata:/gmapsdata -p 8080:8080 google-maps-scraper -data-folder /gmapsdata
```

### Git Bash (Windows)
```bash
MSYS_NO_PATHCONV=1 docker run --rm --shm-size=1g \
  -v "${PWD}/gmapsdata:/gmapsdata" \
  -p 8080:8080 \
  google-maps-scraper \
  -nearby-mode \
  -geo "21.030625,105.819332" \
  -input /gmapsdata/categories.txt \
  -results /gmapsdata/nearby_results.csv \
  -depth 17 \
  -email \
  -zoom 500m
```

### Hybrid Mode with Proxy
```bash
MSYS_NO_PATHCONV=1 docker run --rm --shm-size=1g \
  -v "${PWD}/gmapsdata:/gmapsdata" \
  google-maps-scraper \
  -hybrid-mode \
  -geo "21.030625,105.819332" \
  -input /gmapsdata/categories.txt \
  -results /gmapsdata/hybrid_results.csv \
  -zoom 21 \
  -depth 2 \
  -email \
  -radius 20000 \
  -c 3 \
  -exit-on-inactivity 3m \
  -proxies http://user:pass@proxy.example.com:8080
```

---

## References

- [How to Extract Data from Google Maps Using Golang](https://blog.gkomninos.com/how-to-extract-data-from-google-maps-using-golang)
- [Distributed Google Maps Scraping](https://blog.gkomninos.com/distributed-google-maps-scraping)
- [scrapemate](https://github.com/gosom/scrapemate) - The underlying web crawling framework
- [omkarcloud/google-maps-scraper](https://github.com/omkarcloud/google-maps-scraper) - Inspiration for JS data extraction

---

## Contributing

Contributions are welcome! Please:

1. Open an issue to discuss your idea
2. Fork the repository
3. Create a pull request

See [AGENTS.md](AGENTS.md) for development guidelines.

---

## License

This project is licensed under the [MIT License](LICENSE).

---

## Star History

<a href="https://star-history.com/#gosom/google-maps-scraper&Date">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=gosom/google-maps-scraper&type=Date&theme=dark" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=gosom/google-maps-scraper&type=Date" />
   <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=gosom/google-maps-scraper&type=Date" />
 </picture>
</a>

---

## Legal Notice

Please use this scraper responsibly and in accordance with applicable laws and regulations. Unauthorized scraping may violate terms of service.

---

<p align="center">
  <b>If this project saved you time, consider <a href="https://github.com/gosom/google-maps-scraper">starring it</a> or <a href="https://github.com/sponsors/gosom">sponsoring</a> its development!</b>
</p>

> **Note:** If you register via the links on this page, the project may receive a commission. This is another way to support the work.
