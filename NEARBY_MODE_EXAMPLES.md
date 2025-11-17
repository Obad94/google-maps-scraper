# Nearby Mode - Usage Examples

## Understanding the Parameters

### `-zoom-for-url` (Map View)
Controls how zoomed in the map appears when loading. This affects:
- Initial map view distance
- What Google Maps displays in the viewport
- Does NOT filter results

**Default:** 2000 meters

### `-radius` (Result Filter)  
Filters which places get saved to your results file. This affects:
- Final results in CSV
- Only places within this distance are kept
- Applied AFTER scraping

**Default:** 10000 meters

---

## Example Commands

### Example 1: Tight Zoom, Wide Filter
**Use Case:** Fast loading, but capture all nearby places
```bash
MSYS_NO_PATHCONV=1 docker run --rm --shm-size=1g \
  -v "${PWD}/gmapsdata:/gmapsdata" \
  google-maps-scraper \
  -nearby-mode \
  -geo "24.93584,67.13801" \
  -input /gmapsdata/nearby-categories.txt \
  -results /gmapsdata/results.csv \
  -zoom-for-url 500 \
  -radius 2000 \
  -depth 20
```
**Result:** Map loads at 500m zoom, but saves all places within 2000m

---

### Example 2: Match Zoom and Radius
**Use Case:** What you see is what you get
```bash
MSYS_NO_PATHCONV=1 docker run --rm --shm-size=1g \
  -v "${PWD}/gmapsdata:/gmapsdata" \
  google-maps-scraper \
  -nearby-mode \
  -geo "24.93584,67.13801" \
  -input /gmapsdata/nearby-categories.txt \
  -results /gmapsdata/results.csv \
  -zoom-for-url 1000 \
  -radius 1000 \
  -depth 15
```
**Result:** Map at 1000m zoom, saves places within 1000m

---

### Example 3: Wide View, Tight Filter  
**Use Case:** See context but only save closest places
```bash
MSYS_NO_PATHCONV=1 docker run --rm --shm-size=1g \
  -v "${PWD}/gmapsdata:/gmapsdata" \
  google-maps-scraper \
  -nearby-mode \
  -geo "24.93584,67.13801" \
  -input /gmapsdata/nearby-categories.txt \
  -results /gmapsdata/results.csv \
  -zoom-for-url 3000 \
  -radius 500 \
  -depth 10 \
  -email
```
**Result:** Map loads at 3000m zoom, but only saves places within 500m

---

### Example 4: Very Local Search
**Use Case:** Find only extremely close businesses
```bash
MSYS_NO_PATHCONV=1 docker run --rm --shm-size=1g \
  -v "${PWD}/gmapsdata:/gmapsdata" \
  google-maps-scraper \
  -nearby-mode \
  -geo "40.7128,-74.0060" \
  -input /gmapsdata/nearby-categories.txt \
  -results /gmapsdata/local_results.csv \
  -zoom-for-url 200 \
  -radius 200 \
  -depth 5 \
  -email \
  -exit-on-inactivity 5m
```
**Result:** Very tight 200m search radius for hyperlocal results

---

## PowerShell Examples (Windows)

### Basic Nearby Search
```powershell
docker run --rm --shm-size=1g `
  -v ${PWD}\gmapsdata:/gmapsdata `
  google-maps-scraper `
  -nearby-mode `
  -geo "24.93584,67.13801" `
  -input /gmapsdata/nearby-categories.txt `
  -results /gmapsdata/results.csv `
  -zoom-for-url 500 `
  -radius 1000 `
  -depth 20
```

---

## What You'll See in Logs

When the scraper runs, you'll see output like:

```
Navigating to initial URL: https://www.google.com/maps/search/restaurant/@24.9358,67.1380,500m/...
Using zoom: 500m for map view, radius: 1000.0m for filtering results
✓ Google Maps redirected to final URL: https://www.google.com/maps/search/...
Starting to scroll (max depth: 20) to find nearby places...
Depth 1: found 23 new places (23 total)
Depth 2: found 15 new places (38 total)
...
Nearby search for 'restaurant' found 38 places (scrolled 20 times, radius: 1000.0m)
```

Key things to notice:
1. **Initial URL** with your zoom-for-url parameter
2. **Final URL** after Google Maps redirect
3. **Scroll progress** showing places found
4. **Final summary** with radius used for filtering

---

## Tips

1. **Start with defaults** and adjust based on results
2. **Smaller zoom-for-url** = faster loading but may miss some places
3. **Larger radius** = more results but may include irrelevant places  
4. **Adjust depth** based on how thorough you want the search
5. **Use -email** flag to extract emails (slower but more data)
6. **Monitor logs** to see the actual URLs being used

---

## Troubleshooting

**Too few results?**
- Increase `-radius` to capture more places
- Increase `-depth` to scroll more
- Check your geo coordinates are correct

**Too many irrelevant results?**
- Decrease `-radius` to filter more strictly
- Check the categories in your input file

**Slow scraping?**
- Decrease `-zoom-for-url` for faster map loading
- Reduce `-depth` to scroll less
- Remove `-email` flag if not needed
