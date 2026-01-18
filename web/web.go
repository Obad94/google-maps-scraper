package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

//go:embed static
var static embed.FS

type Server struct {
	tmpl       map[string]*template.Template
	srv        *http.Server
	svc        *Service
	apiKeySvc  *APIKeyService

	// Multi-tenancy services
	authSvc    *AuthService
	orgSvc     *OrganizationService
	memberSvc  *MemberService

	// Repositories
	userRepo       UserRepository
	orgRepo        OrganizationRepository
	memberRepo     OrganizationMemberRepository
	sessionRepo    UserSessionRepository
	invitationRepo OrganizationInvitationRepository
	auditRepo      AuditLogRepository
}

// ServerOptions contains optional dependencies for the server
type ServerOptions struct {
	MemberRepo OrganizationMemberRepository
}

func New(svc *Service, addr string) (*Server, error) {
	return NewWithAPIKeys(svc, nil, addr)
}

func NewWithAPIKeys(svc *Service, apiKeySvc *APIKeyService, addr string) (*Server, error) {
	return NewWithAPIKeysAndAuth(svc, apiKeySvc, nil, addr)
}

func NewWithAPIKeysAndAuth(svc *Service, apiKeySvc *APIKeyService, authSvc *AuthService, addr string) (*Server, error) {
	return NewWithOptions(svc, apiKeySvc, authSvc, addr, nil)
}

func NewWithOptions(svc *Service, apiKeySvc *APIKeyService, authSvc *AuthService, addr string, opts *ServerOptions) (*Server, error) {
	ans := Server{
		svc:       svc,
		apiKeySvc: apiKeySvc,
		authSvc:   authSvc,
		tmpl:      make(map[string]*template.Template),
		srv: &http.Server{
			Addr:              addr,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       60 * time.Second,
			WriteTimeout:      60 * time.Second,
			IdleTimeout:       120 * time.Second,
			MaxHeaderBytes:    1 << 20,
		},
	}

	// Set optional dependencies
	if opts != nil {
		ans.memberRepo = opts.MemberRepo
	}

	staticFS, err := fs.Sub(static, "static")
	if err != nil {
		return nil, err
	}

	fileServer := http.FileServer(http.FS(staticFS))
	mux := http.NewServeMux()

	mux.Handle("/static/", http.StripPrefix("/static/", fileServer))
	
	// Protected routes - require authentication
	mux.HandleFunc("/scrape", ans.WebAuthMiddleware(ans.scrape))
	mux.HandleFunc("/import", ans.WebAuthMiddleware(ans.importData))
	mux.HandleFunc("/download", ans.WebAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		r = requestWithID(r)

		ans.download(w, r)
	}))
	mux.HandleFunc("/delete", ans.WebAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		r = requestWithID(r)

		ans.delete(w, r)
	}))
	mux.HandleFunc("/map", ans.WebAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		r = requestWithID(r)

		ans.showMap(w, r)
	}))
	mux.HandleFunc("/jobs", ans.WebAuthMiddleware(ans.getJobs))

	// Authentication UI routes
	mux.HandleFunc("/login", ans.showLoginPage)
	mux.HandleFunc("/register", ans.showRegisterPage)

	// Authentication API routes
	mux.HandleFunc("/api/v1/auth/register", ans.handleRegister)
	mux.HandleFunc("/api/v1/auth/login", ans.handleLogin)
	mux.HandleFunc("/api/v1/auth/logout", ans.handleLogout)
	mux.HandleFunc("/api/v1/auth/me", ans.handleGetMe)
	mux.HandleFunc("/api/v1/auth/change-password", ans.handleChangePassword)

	// Main page - protected
	mux.HandleFunc("/", ans.WebAuthMiddleware(ans.index))

	// api routes (public - no auth required)
	mux.HandleFunc("/api/docs", ans.redocHandler)
	mux.HandleFunc("/api/swagger", ans.swaggerHandler)

	// Protected API routes - require authentication (supports both API key and session)
	mux.Handle("/api/v1/jobs", ans.APIOrSessionAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			ans.apiScrape(w, r)
		case http.MethodGet:
			ans.apiGetJobs(w, r)
		default:
			ans := apiError{
				Code:    http.StatusMethodNotAllowed,
				Message: "Method not allowed",
			}

			renderJSON(w, http.StatusMethodNotAllowed, ans)
		}
	})))

	mux.Handle("/api/v1/jobs/{id}", ans.APIOrSessionAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = requestWithID(r)

		switch r.Method {
		case http.MethodGet:
			ans.apiGetJob(w, r)
		case http.MethodDelete:
			ans.apiDeleteJob(w, r)
		default:
			ans := apiError{
				Code:    http.StatusMethodNotAllowed,
				Message: "Method not allowed",
			}

			renderJSON(w, http.StatusMethodNotAllowed, ans)
		}
	})))

	mux.Handle("/api/v1/jobs/{id}/download", ans.APIOrSessionAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = requestWithID(r)

		if r.Method != http.MethodGet {
			ans := apiError{
				Code:    http.StatusMethodNotAllowed,
				Message: "Method not allowed",
			}

			renderJSON(w, http.StatusMethodNotAllowed, ans)

			return
		}

		ans.download(w, r)
	})))

	mux.Handle("/api/v1/jobs/{id}/results", ans.APIOrSessionAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = requestWithID(r)

		if r.Method != http.MethodGet {
			ans := apiError{
				Code:    http.StatusMethodNotAllowed,
				Message: "Method not allowed",
			}

			renderJSON(w, http.StatusMethodNotAllowed, ans)

			return
		}

		ans.apiGetResults(w, r)
	})))

	mux.Handle("/api/v1/jobs/{id}/retry", ans.APIOrSessionAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = requestWithID(r)

		if r.Method != http.MethodPost {
			ans := apiError{
				Code:    http.StatusMethodNotAllowed,
				Message: "Method not allowed",
			}

			renderJSON(w, http.StatusMethodNotAllowed, ans)

			return
		}

		ans.apiRetryJob(w, r)
	})))

	mux.Handle("/api/v1/jobs/import", ans.APIOrSessionAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			ans := apiError{
				Code:    http.StatusMethodNotAllowed,
				Message: "Method not allowed",
			}

			renderJSON(w, http.StatusMethodNotAllowed, ans)

			return
		}

		ans.apiImportData(w, r)
	})))

	// API Key Management routes (requires apiKeySvc to be set) - Protected
	if ans.apiKeySvc != nil {
		mux.Handle("/api/v1/apikeys", ans.SessionAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost:
				ans.apiCreateAPIKey(w, r)
			case http.MethodGet:
				ans.apiListAPIKeys(w, r)
			default:
				renderJSON(w, http.StatusMethodNotAllowed, apiError{
					Code:    http.StatusMethodNotAllowed,
					Message: "Method not allowed",
				})
			}
		})))

		mux.Handle("/api/v1/apikeys/{id}", ans.SessionAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = requestWithID(r)

			switch r.Method {
			case http.MethodGet:
				ans.apiGetAPIKey(w, r)
			case http.MethodDelete:
				ans.apiDeleteAPIKey(w, r)
			default:
				renderJSON(w, http.StatusMethodNotAllowed, apiError{
					Code:    http.StatusMethodNotAllowed,
					Message: "Method not allowed",
				})
			}
		})))

		mux.Handle("/api/v1/apikeys/{id}/revoke", ans.SessionAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = requestWithID(r)

			if r.Method != http.MethodPost {
				renderJSON(w, http.StatusMethodNotAllowed, apiError{
					Code:    http.StatusMethodNotAllowed,
					Message: "Method not allowed",
				})
				return
			}

			ans.apiRevokeAPIKey(w, r)
		})))

		// Web UI routes for API key management - protected
		mux.HandleFunc("/apikeys", ans.WebAuthMiddleware(ans.apiKeysPage))
		mux.HandleFunc("/apikeys/create", ans.WebAuthMiddleware(ans.createAPIKeyWeb))
		mux.HandleFunc("/apikeys/{id}/revoke", ans.WebAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
			r = requestWithID(r)
			ans.revokeAPIKeyWeb(w, r)
		}))
		mux.HandleFunc("/apikeys/{id}/delete", ans.WebAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
			r = requestWithID(r)
			ans.deleteAPIKeyWeb(w, r)
		}))
	}

	mux.HandleFunc("/retry", ans.WebAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		r = requestWithID(r)

		ans.retry(w, r)
	}))

	// Apply security headers
	handler := securityHeaders(mux)

	// If API key service is enabled, wrap API routes with authentication middleware
	if ans.apiKeySvc != nil {
		handler = ans.applyAPIKeyAuth(handler)
	}

	ans.srv.Handler = handler

	tmplsKeys := []string{
		"static/templates/index.html",
		"static/templates/job_rows.html",
		"static/templates/job_row.html",
		"static/templates/redoc.html",
		"static/templates/swagger.html",
		"static/templates/map.html",
		"static/templates/apikeys.html",
		"static/templates/apikey_created.html",
		"static/templates/login.html",
		"static/templates/register.html",
	}

	for _, key := range tmplsKeys {
		tmp, err := template.ParseFS(static, key)
		if err != nil {
			return nil, err
		}

		ans.tmpl[key] = tmp
	}

	return &ans, nil
}

// applyAPIKeyAuth wraps the handler with API key authentication for API routes
// It only applies authentication to routes starting with /api/v1/ (except /api/docs and /api/swagger)
// It also accepts session-based authentication as a fallback for web UI users
func (s *Server) applyAPIKeyAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if this is an API route that requires authentication
		path := r.URL.Path

		// Skip authentication for:
		// - Documentation routes (/api/docs, /api/swagger)
		// - Authentication routes (/api/v1/auth/*)
		// - API key management routes (bootstrap scenario)
		// - Web UI routes
		if !strings.HasPrefix(path, "/api/v1/") ||
			path == "/api/docs" ||
			path == "/api/swagger" ||
			strings.HasPrefix(path, "/api/v1/auth/") {
			next.ServeHTTP(w, r)
			return
		}

		// Apply authentication to all other API routes
		ctx := r.Context()

		// First, try to validate session token (for web UI users)
		if s.authSvc != nil {
			sessionToken := extractSessionToken(r)
			if sessionToken != "" {
				user, _, err := s.authSvc.ValidateSession(ctx, sessionToken)
				if err == nil {
					// Valid session, inject user into context and proceed
					ctx = context.WithValue(ctx, contextKeyUser, user)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
		}

		// If no valid session, try API key authentication
		// Extract API key from request
		apiKey := extractAPIKey(r)

		if apiKey == "" {
			renderJSON(w, http.StatusUnauthorized, apiError{
				Code:    http.StatusUnauthorized,
				Message: "API key is required. Provide it via Authorization header (Bearer token), X-API-Key header, or api_key query parameter.",
			})
			return
		}

		// Validate API key
		_, newCtx, err := s.apiKeySvc.Validate(ctx, apiKey)
		if err != nil {
			renderJSON(w, http.StatusUnauthorized, apiError{
				Code:    http.StatusUnauthorized,
				Message: "Invalid or expired API key",
			})
			return
		}

		// API key is valid, proceed to next handler with updated context
		next.ServeHTTP(w, r.WithContext(newCtx))
	})
}

func (s *Server) Start(ctx context.Context) error {
	go func() {
		<-ctx.Done()

		err := s.srv.Shutdown(context.Background())
		if err != nil {
			log.Println(err)

			return
		}

		log.Println("server stopped")
	}()

	fmt.Fprintf(os.Stderr, "visit http://localhost%s\n", s.srv.Addr)

	err := s.srv.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}

	return nil
}

type formData struct {
	Name             string
	MaxTime          string
	ExitOnInactivity string
	Keywords         []string
	Language         string
	Zoom             int
	FastMode         bool
	NearbyMode       bool
	HybridMode       bool
	BrowserAPIMode   bool
	Radius           int
	Lat              string
	Lon              string
	Depth            int
	Email            bool
	Proxies          []string
	Concurrency      int
}

type ctxKey string

const idCtxKey ctxKey = "id"

func requestWithID(r *http.Request) *http.Request {
	id := r.PathValue("id")
	if id == "" {
		id = r.URL.Query().Get("id")
	}

	parsed, err := uuid.Parse(id)
	if err == nil {
		r = r.WithContext(context.WithValue(r.Context(), idCtxKey, parsed))
	}

	return r
}

func getIDFromRequest(r *http.Request) (uuid.UUID, bool) {
	id, ok := r.Context().Value(idCtxKey).(uuid.UUID)

	return id, ok
}

//nolint:gocritic // this is used in template
func (f formData) ProxiesString() string {
	return strings.Join(f.Proxies, "\n")
}

//nolint:gocritic // this is used in template
func (f formData) KeywordsString() string {
	return strings.Join(f.Keywords, "\n")
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)

		return
	}

	tmpl, ok := s.tmpl["static/templates/index.html"]
	if !ok {
		http.Error(w, "missing tpl", http.StatusInternalServerError)

		return
	}

	data := formData{
		Name:             "",
		MaxTime:          "10m",
		ExitOnInactivity: "",
		Keywords:         []string{},
		Language:         "en",
		Zoom:             15,
		FastMode:         false,
		NearbyMode:       false,
		HybridMode:       false,
		BrowserAPIMode:   false,
		Radius:           10000,
		Lat:              "0",
		Lon:              "0",
		Depth:            10,
		Email:            false,
		Concurrency:      0,
	}

	_ = tmpl.Execute(w, data)
}

func (s *Server) scrape(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)

		return
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	newJob := Job{
		ID:     uuid.New().String(),
		Name:   r.Form.Get("name"),
		Date:   time.Now().UTC(),
		Status: StatusPending,
		Data:   JobData{},
	}

	maxTimeStr := r.Form.Get("maxtime")

	maxTime, err := time.ParseDuration(maxTimeStr)
	if err != nil {
		http.Error(w, "invalid max time", http.StatusUnprocessableEntity)

		return
	}

	if maxTime < time.Minute*3 {
		http.Error(w, "max time must be more than 3m", http.StatusUnprocessableEntity)

		return
	}

	newJob.Data.MaxTime = maxTime

	// Parse exit on inactivity (optional)
	// Only set if user explicitly provides it - otherwise leave at 0 (disabled)
	// The webrunner will handle inactivity detection intelligently based on mode and depth
	exitOnInactivityStr := r.Form.Get("exitoninactivity")
	if exitOnInactivityStr != "" {
		exitOnInactivity, err := time.ParseDuration(exitOnInactivityStr)
		if err != nil {
			http.Error(w, "invalid exit on inactivity duration", http.StatusUnprocessableEntity)
			return
		}
		newJob.Data.ExitOnInactivity = exitOnInactivity
	}
	// If empty, ExitOnInactivity stays at 0 (disabled)

	// Parse mode from radio buttons (regular/fast/nearby/hybrid/browserapi)
	mode := r.Form.Get("mode")
	switch mode {
	case "fast":
		newJob.Data.FastMode = true
	case "nearby":
		newJob.Data.NearbyMode = true
	case "hybrid":
		newJob.Data.HybridMode = true
	case "browserapi":
		newJob.Data.BrowserAPIMode = true
	// default case is "regular" mode - all mode flags remain false
	}

	// Handle keywords - for BrowserAPI mode, use place_types dropdown; otherwise use keywords textarea
	if newJob.Data.BrowserAPIMode {
		placeTypes, ok := r.Form["place_types"]
		if !ok || len(placeTypes) == 0 {
			http.Error(w, "please select at least one place type", http.StatusUnprocessableEntity)
			return
		}
		// Each selected option comes as a separate item in the array
		for _, pt := range placeTypes {
			pt = strings.TrimSpace(pt)
			if pt != "" {
				newJob.Data.Keywords = append(newJob.Data.Keywords, pt)
			}
		}
	} else {
		keywordsStr, ok := r.Form["keywords"]
		if !ok {
			http.Error(w, "missing keywords", http.StatusUnprocessableEntity)
			return
		}

		keywords := strings.Split(keywordsStr[0], "\n")
		for _, k := range keywords {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}

			newJob.Data.Keywords = append(newJob.Data.Keywords, k)
		}
	}

	newJob.Data.Lang = r.Form.Get("lang")

	newJob.Data.Zoom, err = strconv.Atoi(r.Form.Get("zoom"))
	if err != nil {
		http.Error(w, "invalid zoom", http.StatusUnprocessableEntity)
		return
	}

	newJob.Data.Radius, err = strconv.Atoi(r.Form.Get("radius"))
	if err != nil {
		http.Error(w, "invalid radius", http.StatusUnprocessableEntity)

		return
	}

	newJob.Data.Lat = r.Form.Get("latitude")
	newJob.Data.Lon = r.Form.Get("longitude")

	newJob.Data.Depth, err = strconv.Atoi(r.Form.Get("depth"))
	if err != nil {
		http.Error(w, "invalid depth", http.StatusUnprocessableEntity)

		return
	}

	// Parse concurrency (optional, defaults to 0 which means use global config)
	concurrencyStr := r.Form.Get("concurrency")
	if concurrencyStr != "" {
		newJob.Data.Concurrency, err = strconv.Atoi(concurrencyStr)
		if err != nil {
			http.Error(w, "invalid concurrency", http.StatusUnprocessableEntity)
			return
		}
		if newJob.Data.Concurrency < 0 {
			http.Error(w, "concurrency must be greater than 0 or leave empty for auto", http.StatusUnprocessableEntity)
			return
		}
	}

	newJob.Data.Email = r.Form.Get("email") == "on"

	proxies := strings.Split(r.Form.Get("proxies"), "\n")
	if len(proxies) > 0 {
		for _, p := range proxies {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}

			newJob.Data.Proxies = append(newJob.Data.Proxies, p)
		}
	}

	// Log all received parameters for debugging
	log.Printf("Job %s (%s) parameters received from web UI:", newJob.ID, newJob.Name)
	log.Printf("  - Keywords: %v", newJob.Data.Keywords)
	log.Printf("  - Mode: NearbyMode=%v, FastMode=%v, HybridMode=%v, BrowserAPIMode=%v", newJob.Data.NearbyMode, newJob.Data.FastMode, newJob.Data.HybridMode, newJob.Data.BrowserAPIMode)
	log.Printf("  - Location: lat=%s, lon=%s, zoom=%d, radius=%dm", newJob.Data.Lat, newJob.Data.Lon, newJob.Data.Zoom, newJob.Data.Radius)
	log.Printf("  - Scraping: depth=%d, email=%v, lang=%s, concurrency=%d", newJob.Data.Depth, newJob.Data.Email, newJob.Data.Lang, newJob.Data.Concurrency)
	log.Printf("  - Timeouts: MaxTime=%v, ExitOnInactivity=%v", newJob.Data.MaxTime, newJob.Data.ExitOnInactivity)
	log.Printf("  - Proxies: %d configured", len(newJob.Data.Proxies))

	err = newJob.Validate()
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)

		return
	}

	err = s.svc.Create(r.Context(), &newJob)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	tmpl, ok := s.tmpl["static/templates/job_row.html"]
	if !ok {
		http.Error(w, "missing tpl", http.StatusInternalServerError)

		return
	}

	// Wrap job with HasResults info
	jobView := JobView{
		Job:        newJob,
		HasResults: s.svc.HasResults(newJob.ID),
	}

	_ = tmpl.Execute(w, jobView)
}

// JobView wraps a Job with additional display information
type JobView struct {
	Job
	HasResults bool // Whether the job has results available for download (even if failed)
}

func (s *Server) getJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)

		return
	}

	tmpl, ok := s.tmpl["static/templates/job_rows.html"]
	if !ok {
		http.Error(w, "missing tpl", http.StatusInternalServerError)
		return
	}

	jobs, err := s.svc.All(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	// Wrap jobs with HasResults info for display
	jobViews := make([]JobView, len(jobs))
	for i, job := range jobs {
		jobViews[i] = JobView{
			Job:        job,
			HasResults: s.svc.HasResults(job.ID),
		}
	}

	_ = tmpl.Execute(w, jobViews)
}

func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)

		return
	}

	ctx := r.Context()

	id, ok := getIDFromRequest(r)
	if !ok {
		http.Error(w, "Invalid ID", http.StatusUnprocessableEntity)

		return
	}

	filePath, err := s.svc.GetCSV(ctx, id.String())
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	file, err := os.Open(filePath)
	if err != nil {
		http.Error(w, "Failed to open file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	fileName := filepath.Base(filePath)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")

	_, err = io.Copy(w, file)
	if err != nil {
		http.Error(w, "Failed to send file", http.StatusInternalServerError)
		return
	}
}

func (s *Server) delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)

		return
	}

	deleteID, ok := getIDFromRequest(r)
	if !ok {
		http.Error(w, "Invalid ID", http.StatusUnprocessableEntity)

		return
	}

	err := s.svc.Delete(r.Context(), deleteID.String())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) retry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)

		return
	}

	retryID, ok := getIDFromRequest(r)
	if !ok {
		http.Error(w, "Invalid ID", http.StatusUnprocessableEntity)

		return
	}

	err := s.svc.Retry(r.Context(), retryID.String())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	// Return the updated job row as HTML using HTMX pattern
	job, err := s.svc.Get(r.Context(), retryID.String())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	tmpl, ok := s.tmpl["static/templates/job_row.html"]
	if !ok {
		http.Error(w, "missing template", http.StatusInternalServerError)

		return
	}

	// Wrap job with HasResults info
	jobView := JobView{
		Job:        job,
		HasResults: s.svc.HasResults(job.ID),
	}

	w.Header().Set("Content-Type", "text/html")
	_ = tmpl.Execute(w, jobView)
}

func (s *Server) showMap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)

		return
	}

	mapID, ok := getIDFromRequest(r)
	if !ok {
		http.Error(w, "Invalid ID", http.StatusUnprocessableEntity)

		return
	}

	// Get job details
	job, err := s.svc.Get(r.Context(), mapID.String())
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)

		return
	}

	// Allow map view for completed jobs OR failed jobs with partial results
	hasResults := s.svc.HasResults(job.ID)
	if job.Status != StatusOK && !(job.Status == StatusFailed && hasResults) {
		http.Error(w, "Map view is only available for completed jobs or failed jobs with partial results", http.StatusBadRequest)

		return
	}

	// Get Google Maps API key from environment
	apiKey := os.Getenv("GOOGLE_MAPS_API_KEY")
	if apiKey == "" {
		http.Error(w, "Google Maps API key not configured. Please set GOOGLE_MAPS_API_KEY environment variable.", http.StatusInternalServerError)

		return
	}

	// Determine the mode
	mode := "Normal"
	if job.Data.FastMode {
		mode = "Fast Mode"
	} else if job.Data.NearbyMode {
		mode = "Nearby Mode"
	} else if job.Data.HybridMode {
		mode = "Hybrid Mode"
	} else if job.Data.BrowserAPIMode {
		mode = "Browser API Mode"
	}

	tmpl, ok := s.tmpl["static/templates/map.html"]
	if !ok {
		http.Error(w, "missing template", http.StatusInternalServerError)

		return
	}

	data := struct {
		JobID   string
		JobName string
		Mode    string
		APIKey  string
	}{
		JobID:   job.ID,
		JobName: job.Name,
		Mode:    mode,
		APIKey:  apiKey,
	}

	w.Header().Set("Content-Type", "text/html")
	_ = tmpl.Execute(w, data)
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type apiScrapeRequest struct {
	Name string
	JobData
}

type apiScrapeResponse struct {
	ID string `json:"id"`
}

func (s *Server) redocHandler(w http.ResponseWriter, _ *http.Request) {
	tmpl, ok := s.tmpl["static/templates/redoc.html"]
	if !ok {
		http.Error(w, "missing tpl", http.StatusInternalServerError)

		return
	}

	_ = tmpl.Execute(w, nil)
}

func (s *Server) swaggerHandler(w http.ResponseWriter, _ *http.Request) {
	tmpl, ok := s.tmpl["static/templates/swagger.html"]
	if !ok {
		http.Error(w, "missing tpl", http.StatusInternalServerError)

		return
	}

	_ = tmpl.Execute(w, nil)
}

func (s *Server) apiScrape(w http.ResponseWriter, r *http.Request) {
	var req apiScrapeRequest

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		ans := apiError{
			Code:    http.StatusUnprocessableEntity,
			Message: err.Error(),
		}

		renderJSON(w, http.StatusUnprocessableEntity, ans)

		return
	}

	newJob := Job{
		ID:     uuid.New().String(),
		Name:   req.Name,
		Date:   time.Now().UTC(),
		Status: StatusPending,
		Data:   req.JobData,
	}

	// convert to seconds
	newJob.Data.MaxTime *= time.Second

	err = newJob.Validate()
	if err != nil {
		ans := apiError{
			Code:    http.StatusUnprocessableEntity,
			Message: err.Error(),
		}

		renderJSON(w, http.StatusUnprocessableEntity, ans)

		return
	}

	err = s.svc.Create(r.Context(), &newJob)
	if err != nil {
		ans := apiError{
			Code:    http.StatusInternalServerError,
			Message: err.Error(),
		}

		renderJSON(w, http.StatusInternalServerError, ans)

		return
	}

	ans := apiScrapeResponse{
		ID: newJob.ID,
	}

	renderJSON(w, http.StatusCreated, ans)
}

func (s *Server) apiGetJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.svc.All(r.Context())
	if err != nil {
		apiError := apiError{
			Code:    http.StatusInternalServerError,
			Message: err.Error(),
		}

		renderJSON(w, http.StatusInternalServerError, apiError)

		return
	}

	renderJSON(w, http.StatusOK, jobs)
}

func (s *Server) apiGetJob(w http.ResponseWriter, r *http.Request) {
	id, ok := getIDFromRequest(r)
	if !ok {
		apiError := apiError{
			Code:    http.StatusUnprocessableEntity,
			Message: "Invalid ID",
		}

		renderJSON(w, http.StatusUnprocessableEntity, apiError)

		return
	}

	job, err := s.svc.Get(r.Context(), id.String())
	if err != nil {
		apiError := apiError{
			Code:    http.StatusNotFound,
			Message: http.StatusText(http.StatusNotFound),
		}

		renderJSON(w, http.StatusNotFound, apiError)

		return
	}

	renderJSON(w, http.StatusOK, job)
}

func (s *Server) apiDeleteJob(w http.ResponseWriter, r *http.Request) {
	id, ok := getIDFromRequest(r)
	if !ok {
		apiError := apiError{
			Code:    http.StatusUnprocessableEntity,
			Message: "Invalid ID",
		}

		renderJSON(w, http.StatusUnprocessableEntity, apiError)

		return
	}

	err := s.svc.Delete(r.Context(), id.String())
	if err != nil {
		apiError := apiError{
			Code:    http.StatusInternalServerError,
			Message: err.Error(),
		}

		renderJSON(w, http.StatusInternalServerError, apiError)

		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) apiRetryJob(w http.ResponseWriter, r *http.Request) {
	id, ok := getIDFromRequest(r)
	if !ok {
		apiError := apiError{
			Code:    http.StatusUnprocessableEntity,
			Message: "Invalid ID",
		}

		renderJSON(w, http.StatusUnprocessableEntity, apiError)

		return
	}

	err := s.svc.Retry(r.Context(), id.String())
	if err != nil {
		apiError := apiError{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		}

		renderJSON(w, http.StatusBadRequest, apiError)

		return
	}

	// Get the updated job and return it
	job, err := s.svc.Get(r.Context(), id.String())
	if err != nil {
		apiError := apiError{
			Code:    http.StatusInternalServerError,
			Message: err.Error(),
		}

		renderJSON(w, http.StatusInternalServerError, apiError)

		return
	}

	renderJSON(w, http.StatusOK, job)
}

func (s *Server) apiGetResults(w http.ResponseWriter, r *http.Request) {
	id, ok := getIDFromRequest(r)
	if !ok {
		apiError := apiError{
			Code:    http.StatusUnprocessableEntity,
			Message: "Invalid ID",
		}

		renderJSON(w, http.StatusUnprocessableEntity, apiError)

		return
	}

	results, err := s.svc.GetResults(r.Context(), id.String())
	if err != nil {
		apiError := apiError{
			Code:    http.StatusNotFound,
			Message: err.Error(),
		}

		renderJSON(w, http.StatusNotFound, apiError)

		return
	}

	renderJSON(w, http.StatusOK, results)
}

func (s *Server) importData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form with max 50MB file size
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		http.Error(w, "Failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Get job name
	jobName := r.FormValue("import_job_name")
	if jobName == "" {
		jobName = "Imported Data - " + time.Now().Format("2006-01-02 15:04:05")
	}

	// Get uploaded file
	file, header, err := r.FormFile("import_file")
	if err != nil {
		http.Error(w, "Failed to get file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Read file content
	fileData, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Failed to read file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Determine file type from extension
	var job *Job
	filename := strings.ToLower(header.Filename)

	if strings.HasSuffix(filename, ".csv") {
		job, err = s.svc.ImportFromCSV(r.Context(), jobName, fileData)
	} else if strings.HasSuffix(filename, ".json") {
		job, err = s.svc.ImportFromJSON(r.Context(), jobName, fileData)
	} else {
		http.Error(w, "Unsupported file type. Please upload a CSV or JSON file.", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, "Import failed: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}

	// Return the job row HTML for HTMX
	tmpl, ok := s.tmpl["static/templates/job_row.html"]
	if !ok {
		http.Error(w, "missing tpl", http.StatusInternalServerError)
		return
	}

	_ = tmpl.Execute(w, job)
}

func (s *Server) apiImportData(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form with max 50MB file size
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		apiError := apiError{
			Code:    http.StatusBadRequest,
			Message: "Failed to parse form: " + err.Error(),
		}
		renderJSON(w, http.StatusBadRequest, apiError)
		return
	}

	// Get job name
	jobName := r.FormValue("job_name")
	if jobName == "" {
		jobName = "Imported Data - " + time.Now().Format("2006-01-02 15:04:05")
	}

	// Get uploaded file
	file, header, err := r.FormFile("file")
	if err != nil {
		apiError := apiError{
			Code:    http.StatusBadRequest,
			Message: "Failed to get file: " + err.Error(),
		}
		renderJSON(w, http.StatusBadRequest, apiError)
		return
	}
	defer file.Close()

	// Read file content
	fileData, err := io.ReadAll(file)
	if err != nil {
		apiError := apiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to read file: " + err.Error(),
		}
		renderJSON(w, http.StatusInternalServerError, apiError)
		return
	}

	// Determine file type from extension
	var job *Job
	filename := strings.ToLower(header.Filename)

	if strings.HasSuffix(filename, ".csv") {
		job, err = s.svc.ImportFromCSV(r.Context(), jobName, fileData)
	} else if strings.HasSuffix(filename, ".json") {
		job, err = s.svc.ImportFromJSON(r.Context(), jobName, fileData)
	} else {
		apiError := apiError{
			Code:    http.StatusBadRequest,
			Message: "Unsupported file type. Please upload a CSV or JSON file.",
		}
		renderJSON(w, http.StatusBadRequest, apiError)
		return
	}

	if err != nil {
		apiError := apiError{
			Code:    http.StatusUnprocessableEntity,
			Message: "Import failed: " + err.Error(),
		}
		renderJSON(w, http.StatusUnprocessableEntity, apiError)
		return
	}

	renderJSON(w, http.StatusCreated, job)
}

func renderJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	_ = json.NewEncoder(w).Encode(data)
}

func formatDate(t time.Time) string {
	return t.Format("Jan 02, 2006 15:04:05")
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' cdn.redoc.ly cdnjs.cloudflare.com unpkg.com maps.googleapis.com 'unsafe-inline' 'unsafe-eval'; "+
				"worker-src 'self' blob:; "+
				"style-src 'self' 'unsafe-inline' fonts.googleapis.com unpkg.com; "+
				"img-src 'self' data: cdn.redoc.ly *.googleapis.com *.gstatic.com maps.gstatic.com *.google.com; "+
				"font-src 'self' fonts.gstatic.com; "+
				"connect-src 'self' *.googleapis.com maps.googleapis.com")

		next.ServeHTTP(w, r)
	})
}
