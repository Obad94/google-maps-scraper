# Map View Feature

## Overview
The Google Maps Scraper now includes a "View on Map" button that allows you to visualize all scraped places on an interactive Google Map. This feature works with all scraping modes (Normal, Fast, Nearby, Hybrid, and Browser API).

## Features
- 📍 Interactive Google Maps visualization
- 🗺️ Custom markers for each place
- 📊 Info windows with detailed place information
- 🎯 Automatic map centering and bounds fitting
- 🔄 Support for all scraping modes
- 📱 Responsive design

## Setup

### 1. Configure Google Maps API Key
You need a Google Maps API key with the Maps JavaScript API enabled.

1. Go to [Google Cloud Console](https://console.cloud.google.com/google/maps-apis)
2. Create a new project or select an existing one
3. Enable the **Maps JavaScript API**
4. Create credentials (API Key)
5. Copy your API key

### 2. Set Environment Variable
Add your API key to the `.env` file in the project root:

```bash
GOOGLE_MAPS_API_KEY=your_api_key_here
```

The `.env` file is automatically loaded when you start the application.

### 3. Restart the Application
After setting the API key, restart your application:

```bash
# If running locally
./google-maps-scraper -data-folder gmapsdata

# If using Docker
docker run -v ${PWD}/gmapsdata:/gmapsdata -v ${PWD}/.env:/.env -p 8080:8080 google-maps-scraper -data-folder /gmapsdata
```

## Usage

### Web Interface
1. Navigate to http://localhost:8080
2. Complete a scraping job (wait for status to show "ok")
3. Click the **"View on Map"** button next to the Download button
4. A new tab will open showing all scraped places on the map

### Map Features
- **Click on markers** to see detailed information:
  - Place name
  - Category
  - Address
  - Rating and review count
  - Phone number
  - Website link
  - Direct link to Google Maps

- **Map automatically adjusts** to show all markers
- **Different modes** are displayed in the header for context

## API Endpoints

### Map View Endpoint
```
GET /map?id={job_id}
```
Opens the map view for a completed job.

**Requirements:**
- Job must have status "ok"
- GOOGLE_MAPS_API_KEY environment variable must be set

**Response:**
- HTML page with interactive Google Maps

### Results API (used by map)
```
GET /api/v1/jobs/{id}/results
```
Returns JSON array of all scraped places with coordinates.

## Supported Modes
The map view works with all scraping modes:
- ✅ **Normal Mode** - Standard Google Maps scraping
- ✅ **Fast Mode** - HTTP-based scraping
- ✅ **Nearby Mode** - Proximity-based search
- ✅ **Hybrid Mode** - Combined approach
- ✅ **Browser API Mode** - Browser-based API scraping

The mode is displayed in the map header for context.

## Troubleshooting

### "Google Maps API key not configured" Error
**Solution:** Make sure you've set the `GOOGLE_MAPS_API_KEY` environment variable in your `.env` file and restarted the application.

### "Map view is only available for completed jobs" Error
**Solution:** Wait for the job to complete (status should be "ok"). The map button only appears for completed jobs.

### Map doesn't load or shows blank
**Solutions:**
1. Check browser console for errors
2. Verify your API key is valid and the Maps JavaScript API is enabled
3. Check if you have any billing issues with Google Cloud
4. Ensure your API key has no restrictions that block localhost

### No markers appear on map
**Solutions:**
1. Check if the CSV file contains latitude/longitude data
2. Verify that the scraping completed successfully
3. Some places may not have valid coordinates - check the browser console for warnings

## Technical Details

### Files Modified
- `web/static/templates/map.html` - New map view template
- `web/static/templates/job_row.html` - Added "View on Map" button
- `web/static/css/main.css` - Added styling for map button
- `web/web.go` - Added map endpoint and updated CSP headers

### Security
Content Security Policy (CSP) headers have been updated to allow:
- Google Maps JavaScript API (`maps.googleapis.com`)
- Google Maps images and tiles (`*.gstatic.com`, `*.google.com`)
- Required API connections

### Data Flow
1. User clicks "View on Map" button
2. Backend fetches job details and validates status
3. Map template is rendered with job info and API key
4. JavaScript loads Google Maps API
5. Frontend fetches results from `/api/v1/jobs/{id}/results`
6. Markers are placed for each place with valid coordinates
7. Map auto-zooms to fit all markers

## Example

After running a scraping job:
```bash
# The job table will show:
Job ID: abc-123
Job Name: Restaurants in NYC
Status: ok
Actions: [Download] [View on Map] [Delete]
```

Click "View on Map" to see all restaurants plotted on an interactive map of NYC!

## Future Enhancements
Possible improvements for future versions:
- [ ] Clustering for large datasets
- [ ] Custom marker colors by category
- [ ] Export map as image
- [ ] Filter markers by rating/category
- [ ] Heatmap view
- [ ] Directions between places
