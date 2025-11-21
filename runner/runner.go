package runner

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-runewidth"
	"golang.org/x/term"

	"github.com/gosom/google-maps-scraper/s3uploader"
	"github.com/gosom/google-maps-scraper/tlmt"
	"github.com/gosom/google-maps-scraper/tlmt/gonoop"
	"github.com/gosom/google-maps-scraper/tlmt/goposthog"
)

const (
	RunModeFile = iota + 1
	RunModeDatabase
	RunModeDatabaseProduce
	RunModeInstallPlaywright
	RunModeWeb
	RunModeAwsLambda
	RunModeAwsLambdaInvoker
)

var (
	ErrInvalidRunMode = errors.New("invalid run mode")
)

type Runner interface {
	Run(context.Context) error
	Close(context.Context) error
}

type S3Uploader interface {
	Upload(ctx context.Context, bucketName, key string, body io.Reader) error
}

type Config struct {
	Concurrency              int
	CacheDir                 string
	MaxDepth                 int
	InputFile                string
	ResultsFile              string
	JSON                     bool
	LangCode                 string
	Debug                    bool
	Dsn                      string
	ProduceOnly              bool
	ExitOnInactivityDuration time.Duration
	Email                    bool
	CustomWriter             string
	GeoCoordinates           string
	ZoomLevel                int
	ZoomMeters               int
	RunMode                  int
	DisableTelemetry         bool
	WebRunner                bool
	AwsLamdbaRunner          bool
	DataFolder               string
	Proxies                  []string
	AwsAccessKey             string
	AwsSecretKey             string
	AwsRegion                string
	S3Uploader               S3Uploader
	S3Bucket                 string
	AwsLambdaInvoker         bool
	FunctionName             string
	AwsLambdaChunkSize       int
	FastMode                 bool
	NearbyMode               bool
	Radius                   float64
	Addr                     string
	DisablePageReuse         bool
	ExtraReviews             bool
	GoogleMapsAPIKey         string
}

// parseZoomLevel parses a zoom level string and returns zoomLevel (0-21) or zoomMeters (51+)
// Supports formats: "15z", "2000m", "15", "2000"
// Auto-detection: 1-21 = zoom level, 51+ = meters
func parseZoomLevel(zoomStr string) (zoomLevel int, zoomMeters int, err error) {
	zoomStr = strings.TrimSpace(zoomStr)

	// Check for explicit unit suffix
	if strings.HasSuffix(zoomStr, "z") {
		// Zoom level: "15z"
		val, err := strconv.Atoi(strings.TrimSuffix(zoomStr, "z"))
		if err != nil {
			return 0, 0, fmt.Errorf("invalid zoom level format: %s", zoomStr)
		}
		if val < 0 || val > 21 {
			return 0, 0, fmt.Errorf("zoom level must be between 0-21z, got: %s", zoomStr)
		}
		return val, 0, nil
	}

	if strings.HasSuffix(zoomStr, "m") {
		// Meters: "2000m"
		val, err := strconv.Atoi(strings.TrimSuffix(zoomStr, "m"))
		if err != nil {
			return 0, 0, fmt.Errorf("invalid zoom meters format: %s", zoomStr)
		}
		if val < 51 {
			return 0, 0, fmt.Errorf("zoom meters must be 51m or greater, got: %s", zoomStr)
		}
		return 0, val, nil
	}

	// No unit - auto-detect based on value
	val, err := strconv.Atoi(zoomStr)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid zoom value: %s (use format like '15z' or '2000m')", zoomStr)
	}

	if val >= 1 && val <= 21 {
		// Assume zoom level
		return val, 0, nil
	} else if val >= 51 {
		// Assume meters
		return 0, val, nil
	} else if val == 0 {
		// Special case: 0 is valid zoom level
		return 0, 0, nil
	} else {
		// Ambiguous range 22-50
		return 0, 0, fmt.Errorf("ambiguous zoom value: %d (use explicit unit: '15z' for zoom level or '2000m' for meters)", val)
	}
}

func ParseConfig() *Config {
	cfg := Config{}

	if os.Getenv("PLAYWRIGHT_INSTALL_ONLY") == "1" {
		cfg.RunMode = RunModeInstallPlaywright

		return &cfg
	}

	var (
		proxies  string
		zoomStr  string
	)

	flag.IntVar(&cfg.Concurrency, "c", min(runtime.NumCPU()/2, 1), "sets the concurrency [default: half of CPU cores]")
	flag.StringVar(&cfg.CacheDir, "cache", "cache", "sets the cache directory [no effect at the moment]")
	flag.IntVar(&cfg.MaxDepth, "depth", 10, "maximum scroll depth in search results [default: 10]")
	flag.StringVar(&cfg.ResultsFile, "results", "stdout", "path to the results file [default: stdout]")
	flag.StringVar(&cfg.InputFile, "input", "", "path to the input file with queries (one per line) [default: empty]")
	flag.StringVar(&cfg.LangCode, "lang", "en", "language code for Google (e.g., 'de' for German) [default: en]")
	flag.BoolVar(&cfg.Debug, "debug", false, "enable headful crawl (opens browser window) [default: false]")
	flag.StringVar(&cfg.Dsn, "dsn", "", "database connection string [only valid with database provider]")
	flag.BoolVar(&cfg.ProduceOnly, "produce", false, "produce seed jobs only (requires dsn)")
	flag.DurationVar(&cfg.ExitOnInactivityDuration, "exit-on-inactivity", 0, "exit after inactivity duration (e.g., '5m')")
	flag.BoolVar(&cfg.JSON, "json", false, "produce JSON output instead of CSV")
	flag.BoolVar(&cfg.Email, "email", false, "extract emails from websites")
	flag.StringVar(&cfg.CustomWriter, "writer", "", "use custom writer plugin (format: 'dir:pluginName')")
	flag.StringVar(&cfg.GeoCoordinates, "geo", "", "set geo coordinates for search (e.g., '37.7749,-122.4194')")
	flag.StringVar(&zoomStr, "zoom", "15z", "zoom level (1-21z) or distance in meters (51m+). Examples: '15z', '2000m', '500m'. Auto-detects: 1-21=zoom level, 51+=meters")
	flag.BoolVar(&cfg.WebRunner, "web", false, "run web server instead of crawling")
	flag.StringVar(&cfg.DataFolder, "data-folder", "webdata", "data folder for web runner")
	flag.StringVar(&proxies, "proxies", "", "comma separated list of proxies to use in the format protocol://user:pass@host:port example: socks5://localhost:9050 or http://user:pass@localhost:9050")
	flag.BoolVar(&cfg.AwsLamdbaRunner, "aws-lambda", false, "run as AWS Lambda function")
	flag.BoolVar(&cfg.AwsLambdaInvoker, "aws-lambda-invoker", false, "run as AWS Lambda invoker")
	flag.StringVar(&cfg.FunctionName, "function-name", "", "AWS Lambda function name")
	flag.StringVar(&cfg.AwsAccessKey, "aws-access-key", "", "AWS access key")
	flag.StringVar(&cfg.AwsSecretKey, "aws-secret-key", "", "AWS secret key")
	flag.StringVar(&cfg.AwsRegion, "aws-region", "", "AWS region")
	flag.StringVar(&cfg.S3Bucket, "s3-bucket", "", "S3 bucket name")
	flag.IntVar(&cfg.AwsLambdaChunkSize, "aws-lambda-chunk-size", 100, "AWS Lambda chunk size")
	flag.BoolVar(&cfg.FastMode, "fast-mode", false, "fast mode (reduced data collection)")
	flag.BoolVar(&cfg.NearbyMode, "nearby-mode", false, "nearby search mode (right-click search nearby simulation)")
	flag.Float64Var(&cfg.Radius, "radius", 10000, "search radius in meters for filtering results. Default is 10000 meters")
	flag.StringVar(&cfg.Addr, "addr", ":8080", "address to listen on for web server")
	flag.BoolVar(&cfg.DisablePageReuse, "disable-page-reuse", false, "disable page reuse in playwright")
	flag.BoolVar(&cfg.ExtraReviews, "extra-reviews", false, "enable extra reviews collection")

	flag.Parse()

	// Parse zoom level
	zoomLevel, zoomMeters, err := parseZoomLevel(zoomStr)
	if err != nil {
		panic(err)
	}
	cfg.ZoomLevel = zoomLevel
	cfg.ZoomMeters = zoomMeters

	if cfg.AwsAccessKey == "" {
		cfg.AwsAccessKey = os.Getenv("MY_AWS_ACCESS_KEY")
	}

	if cfg.AwsSecretKey == "" {
		cfg.AwsSecretKey = os.Getenv("MY_AWS_SECRET_KEY")
	}

	if cfg.AwsRegion == "" {
		cfg.AwsRegion = os.Getenv("MY_AWS_REGION")
	}

	if cfg.GoogleMapsAPIKey == "" {
		cfg.GoogleMapsAPIKey = os.Getenv("GOOGLE_MAPS_API_KEY")
	}

	if cfg.AwsLambdaInvoker && cfg.FunctionName == "" {
		panic("FunctionName must be provided when using AwsLambdaInvoker")
	}

	if cfg.AwsLambdaInvoker && cfg.S3Bucket == "" {
		panic("S3Bucket must be provided when using AwsLambdaInvoker")
	}

	if cfg.AwsLambdaInvoker && cfg.InputFile == "" {
		panic("InputFile must be provided when using AwsLambdaInvoker")
	}

	if cfg.Concurrency < 1 {
		panic("Concurrency must be greater than 0")
	}

	if cfg.MaxDepth < 1 {
		panic("MaxDepth must be greater than 0")
	}

	if cfg.Dsn == "" && cfg.ProduceOnly {
		panic("Dsn must be provided when using ProduceOnly")
	}

	if proxies != "" {
		cfg.Proxies = strings.Split(proxies, ",")
	}

	if cfg.AwsAccessKey != "" && cfg.AwsSecretKey != "" && cfg.AwsRegion != "" {
		cfg.S3Uploader = s3uploader.New(cfg.AwsAccessKey, cfg.AwsSecretKey, cfg.AwsRegion)
	}

	switch {
	case cfg.AwsLambdaInvoker:
		cfg.RunMode = RunModeAwsLambdaInvoker
	case cfg.AwsLamdbaRunner:
		cfg.RunMode = RunModeAwsLambda
	case cfg.WebRunner || (cfg.Dsn == "" && cfg.InputFile == ""):
		cfg.RunMode = RunModeWeb
	case cfg.Dsn == "":
		cfg.RunMode = RunModeFile
	case cfg.ProduceOnly:
		cfg.RunMode = RunModeDatabaseProduce
	case cfg.Dsn != "":
		cfg.RunMode = RunModeDatabase
	default:
		panic("Invalid configuration")
	}

	return &cfg
}

var (
	telemetryOnce sync.Once
	telemetry     tlmt.Telemetry
)

func Telemetry() tlmt.Telemetry {
	telemetryOnce.Do(func() {
		disableTel := func() bool {
			return os.Getenv("DISABLE_TELEMETRY") == "1"
		}()

		if disableTel {
			telemetry = gonoop.New()

			return
		}

		val, err := goposthog.New("phc_CHYBGEd1eJZzDE7ZWhyiSFuXa9KMLRnaYN47aoIAY2A", "https://eu.i.posthog.com")
		if err != nil || val == nil {
			telemetry = gonoop.New()

			return
		}

		telemetry = val
	})

	return telemetry
}

func wrapText(text string, width int) []string {
	var lines []string

	currentLine := ""
	currentWidth := 0

	for _, r := range text {
		runeWidth := runewidth.RuneWidth(r)
		if currentWidth+runeWidth > width {
			lines = append(lines, currentLine)
			currentLine = string(r)
			currentWidth = runeWidth
		} else {
			currentLine += string(r)
			currentWidth += runeWidth
		}
	}

	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	return lines
}

func banner(messages []string, width int) string {
	if width <= 0 {
		var err error

		width, _, err = term.GetSize(0)
		if err != nil {
			width = 80
		}
	}

	if width < 20 {
		width = 20
	}

	contentWidth := width - 4

	var wrappedLines []string
	for _, message := range messages {
		wrappedLines = append(wrappedLines, wrapText(message, contentWidth)...)
	}

	var builder strings.Builder

	builder.WriteString("╔" + strings.Repeat("═", width-2) + "╗\n")

	for _, line := range wrappedLines {
		lineWidth := runewidth.StringWidth(line)
		paddingRight := contentWidth - lineWidth

		if paddingRight < 0 {
			paddingRight = 0
		}

		builder.WriteString(fmt.Sprintf("║ %s%s ║\n", line, strings.Repeat(" ", paddingRight)))
	}

	builder.WriteString("╚" + strings.Repeat("═", width-2) + "╝\n")

	return builder.String()
}

func Banner() {
	message1 := "🌍 Google Maps Scraper"
	message2 := "⭐ If you find this project useful, please star it on GitHub: https://github.com/gosom/google-maps-scraper"
	message3 := "💖 Consider sponsoring to support development: https://github.com/sponsors/gosom"

	fmt.Fprintln(os.Stderr, banner([]string{message1, message2, message3}, 0))
}
