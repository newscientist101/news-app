package srv

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"srv.exe.dev/db"
	"srv.exe.dev/db/dbgen"
)

type Server struct {
	DB           *sql.DB
	Hostname     string
	TemplatesDir string
	StaticDir    string
	ArticlesDir  string
	templates    map[string]*template.Template
	rateLimiter  *RateLimiter
	csrfTokens   *CSRFStore
}

// CSRFStore manages CSRF tokens per user
type CSRFStore struct {
	mu     sync.RWMutex
	tokens map[string]csrfEntry // userID -> token entry
}

type csrfEntry struct {
	token     string
	expiresAt time.Time
}

const csrfTokenLength = 32
const csrfTokenTTL = 24 * time.Hour
const csrfHeaderName = "X-CSRF-Token"

func NewCSRFStore() *CSRFStore {
	return &CSRFStore{
		tokens: make(map[string]csrfEntry),
	}
}

// GetOrCreateToken returns a valid CSRF token for the user, creating one if needed
func (cs *CSRFStore) GetOrCreateToken(userID string) string {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	
	entry, exists := cs.tokens[userID]
	if exists && time.Now().Before(entry.expiresAt) {
		return entry.token
	}
	
	// Generate new token
	b := make([]byte, csrfTokenLength)
	rand.Read(b)
	token := base64.URLEncoding.EncodeToString(b)
	
	cs.tokens[userID] = csrfEntry{
		token:     token,
		expiresAt: time.Now().Add(csrfTokenTTL),
	}
	
	return token
}

// ValidateToken checks if the provided token is valid for the user
func (cs *CSRFStore) ValidateToken(userID, token string) bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	
	entry, exists := cs.tokens[userID]
	if !exists {
		return false
	}
	if time.Now().After(entry.expiresAt) {
		return false
	}
	return entry.token == token
}

// RateLimiter implements a simple per-user rate limiter
type RateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	window   time.Duration
	limit    int
}

// NewRateLimiter creates a rate limiter with the given window and limit
func NewRateLimiter(window time.Duration, limit int) *RateLimiter {
	return &RateLimiter{
		requests: make(map[string][]time.Time),
		window:   window,
		limit:    limit,
	}
}

// Allow checks if a request from the given key should be allowed
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	now := time.Now()
	cutoff := now.Add(-rl.window)
	
	// Filter out old requests
	var recent []time.Time
	for _, t := range rl.requests[key] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	
	if len(recent) >= rl.limit {
		rl.requests[key] = recent
		return false
	}
	
	rl.requests[key] = append(recent, now)
	return true
}
// HandleRoot is a placeholder for the actual root handler implementation
func (s *Server) HandleRoot(w *httptest.ResponseRecorder, req *http.Request) {
	panic("unimplemented")
}

func New(dbPath, hostname string) (*Server, error) {
	_, thisFile, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(thisFile)
	
	// Use environment variable for articles dir, with fallback
	articlesDir := "/home/exedev/news-app/articles"
	if dir := getEnvOrDefault("NEWS_APP_ARTICLES_DIR", ""); dir != "" {
		articlesDir = dir
	}
	
	srv := &Server{
		Hostname:     hostname,
		TemplatesDir: filepath.Join(baseDir, "templates"),
		StaticDir:    filepath.Join(baseDir, "static"),
		ArticlesDir:  articlesDir,
		templates:    make(map[string]*template.Template),
		rateLimiter:  NewRateLimiter(time.Minute, 10), // 10 requests per minute
		csrfTokens:   NewCSRFStore(),
	}
	if err := srv.setUpDatabase(dbPath); err != nil {
		return nil, err
	}
	if err := srv.loadTemplates(); err != nil {
		return nil, err
	}
	return srv, nil
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := strings.TrimSpace(os.Getenv(key)); val != "" {
		return val
	}
	return defaultVal
}

func (s *Server) setUpDatabase(dbPath string) error {
	wdb, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open db: %w", err)
	}
	s.DB = wdb
	if err := db.RunMigrations(wdb); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	return nil
}

func (s *Server) Serve(addr string) error {
	mux := http.NewServeMux()

	// Pages
	mux.HandleFunc("GET /{$}", s.handleDashboard)
	mux.HandleFunc("GET /jobs", s.handleJobsList)
	mux.HandleFunc("GET /jobs/new", s.handleJobNew)
	mux.HandleFunc("GET /jobs/{id}/edit", s.handleJobEdit)
	mux.HandleFunc("GET /jobs/{id}", s.handleJobDetail)
	mux.HandleFunc("GET /articles", s.handleArticlesList)
	mux.HandleFunc("GET /articles/{id}", s.handleArticleDetail)
	mux.HandleFunc("GET /preferences", s.handlePreferences)
	mux.HandleFunc("GET /runs", s.handleRuns)

	// API (protected by CSRF)
	mux.HandleFunc("POST /api/jobs", s.csrfProtect(s.handleCreateJob))
	mux.HandleFunc("PUT /api/jobs/{id}", s.csrfProtect(s.handleUpdateJob))
	mux.HandleFunc("DELETE /api/jobs/{id}", s.csrfProtect(s.handleDeleteJob))
	mux.HandleFunc("POST /api/jobs/{id}/run", s.csrfProtect(s.handleRunJob))
	mux.HandleFunc("POST /api/jobs/{id}/stop", s.csrfProtect(s.handleStopJob))
	mux.HandleFunc("POST /api/runs/{id}/cancel", s.csrfProtect(s.handleCancelRun))
	mux.HandleFunc("POST /api/articles/delete", s.csrfProtect(s.handleDeleteArticles))
	mux.HandleFunc("POST /api/preferences", s.csrfProtect(s.handleUpdatePreferences))
	mux.HandleFunc("GET /api/articles/{id}/content", s.handleArticleContent)
	mux.HandleFunc("GET /api/runs/{id}/log", s.handleRunLog)

	// Static files
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(s.StaticDir))))

	slog.Info("starting server", "addr", addr)
	return http.ListenAndServe(addr, mux)
}

// csrfProtect wraps a handler with CSRF token validation
func (s *Server) csrfProtect(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := strings.TrimSpace(r.Header.Get("X-ExeDev-UserID"))
		if userID == "" {
			s.jsonError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		
		token := r.Header.Get(csrfHeaderName)
		if token == "" {
			s.jsonError(w, "Missing CSRF token", http.StatusForbidden)
			return
		}
		
		if !s.csrfTokens.ValidateToken(userID, token) {
			s.jsonError(w, "Invalid CSRF token", http.StatusForbidden)
			return
		}
		
		next(w, r)
	}
}

// getOrCreateUser ensures a user exists and returns their ID
func (s *Server) getOrCreateUser(r *http.Request) (*dbgen.User, error) {
	exeUserID := strings.TrimSpace(r.Header.Get("X-ExeDev-UserID"))
	email := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))

	if exeUserID == "" {
		return nil, fmt.Errorf("not authenticated")
	}

	q := dbgen.New(s.DB)
	user, err := q.GetUserByExeID(r.Context(), exeUserID)
	if err == sql.ErrNoRows {
		user, err = q.CreateUser(r.Context(), dbgen.CreateUserParams{
			ExeUserID: exeUserID,
			Email:     email,
		})
		if err != nil {
			return nil, fmt.Errorf("create user: %w", err)
		}
		// Create default preferences
		_, err = q.CreatePreferences(r.Context(), user.ID)
		if err != nil {
			slog.Warn("create preferences", "error", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	return &user, nil
}

// getCSRFToken returns a CSRF token for the current user
func (s *Server) getCSRFToken(r *http.Request) string {
	userID := strings.TrimSpace(r.Header.Get("X-ExeDev-UserID"))
	if userID == "" {
		return ""
	}
	return s.csrfTokens.GetOrCreateToken(userID)
}

// templateFuncMap returns the function map used in templates
func templateFuncMap() template.FuncMap {
	return template.FuncMap{
		"add":      func(a, b int) int { return a + b },
		"subtract": func(a, b int) int { return a - b },
		"multiply": func(a, b int) int64 { return int64(a) * int64(b) },
	}
}

// loadTemplates parses all templates at startup
func (s *Server) loadTemplates() error {
	layoutPath := filepath.Join(s.TemplatesDir, "layout.html")
	
	templateFiles := []string{
		"dashboard.html",
		"jobs.html",
		"job_new.html",
		"job_edit.html",
		"job_detail.html",
		"articles.html",
		"article_detail.html",
		"preferences.html",
		"runs.html",
		"welcome.html",
	}
	
	for _, name := range templateFiles {
		path := filepath.Join(s.TemplatesDir, name)
		tmpl, err := template.New("").Funcs(templateFuncMap()).ParseFiles(layoutPath, path)
		if err != nil {
			return fmt.Errorf("parse template %q: %w", name, err)
		}
		s.templates[name] = tmpl
	}
	
	slog.Info("loaded templates", "count", len(s.templates))
	return nil
}

func (s *Server) renderTemplate(w http.ResponseWriter, name string, data any) error {
	tmpl, ok := s.templates[name]
	if !ok {
		return fmt.Errorf("template %q not found", name)
	}
	if err := tmpl.ExecuteTemplate(w, "layout", data); err != nil {
		return fmt.Errorf("execute template %q: %w", name, err)
	}
	return nil
}

func (s *Server) jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (s *Server) jsonOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
