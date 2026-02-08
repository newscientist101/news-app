package srv

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"srv.exe.dev/db/dbgen"
)

// parseSearchTerms splits a search query into terms, keeping quoted phrases together
func parseSearchTerms(query string) []string {
	var terms []string
	// Match quoted strings or non-space sequences
	re := regexp.MustCompile(`"([^"]+)"|'([^']+)'|(\S+)`)
	matches := re.FindAllStringSubmatch(query, -1)
	for _, match := range matches {
		if match[1] != "" {
			terms = append(terms, match[1]) // double-quoted
		} else if match[2] != "" {
			terms = append(terms, match[2]) // single-quoted
		} else if match[3] != "" {
			terms = append(terms, match[3]) // unquoted word
		}
	}
	return terms
}

// articlesFilter holds parsed filter parameters for article listing
type articlesFilter struct {
	Page           int
	Limit          int64
	Offset         int64
	SearchQuery    string
	JobFilter      int64
	DateFilter     string
	DateFrom       string
	DateTo         string
	SinceTime      time.Time
	UntilTime      time.Time
	UseCustomRange bool
}

// parseArticlesFilters extracts filter parameters from the request.
func parseArticlesFilters(r *http.Request) articlesFilter {
	q := r.URL.Query()

	page, _ := strconv.Atoi(q.Get("page"))
	jobFilter, _ := strconv.ParseInt(q.Get("job"), 10, 64)

	f := articlesFilter{
		Page:        max(page, 1),
		Limit:       int64(DefaultPageLimit),
		SearchQuery: q.Get("q"),
		JobFilter:   jobFilter,
		DateFilter:  q.Get("filter"),
		DateFrom:    q.Get("from"),
		DateTo:      q.Get("to"),
	}
	f.Offset = int64(f.Page-1) * f.Limit

	f.parseDateFilters()
	return f
}

// parseDateFilters sets SinceTime/UntilTime based on date filter params.
func (f *articlesFilter) parseDateFilters() {
	// Custom date range takes priority
	if f.DateFrom != "" || f.DateTo != "" {
		f.parseCustomDateRange()
		return
	}

	// Predefined filters
	f.SinceTime = f.predefinedDateOffset()
	if f.SinceTime.IsZero() {
		f.DateFilter = "" // Invalid filter
	}
}

func (f *articlesFilter) parseCustomDateRange() {
	f.DateFilter = "custom"
	f.UseCustomRange = true

	if f.DateFrom != "" {
		f.SinceTime, _ = time.Parse("2006-01-02", f.DateFrom)
	}
	if f.DateTo != "" {
		f.UntilTime, _ = time.Parse("2006-01-02", f.DateTo)
		f.UntilTime = f.UntilTime.Add(24*time.Hour - time.Second) // End of day
	} else {
		f.UntilTime = time.Now()
	}
}

func (f *articlesFilter) predefinedDateOffset() time.Time {
	now := time.Now()
	switch f.DateFilter {
	case "day":
		return now.AddDate(0, 0, -1)
	case "week":
		return now.AddDate(0, 0, -7)
	case "month":
		return now.AddDate(0, -1, 0)
	default:
		return time.Time{}
	}
}


type PageData struct {
	User         *dbgen.User
	Preferences  *dbgen.Preference
	Jobs         []dbgen.Job
	Job          *dbgen.Job
	Articles     []dbgen.Article
	Article      *dbgen.Article
	RunningRuns  []dbgen.ListRunningJobRunsRow
	RecentRuns   []dbgen.ListRecentJobRunsRow
	TotalCount   int64
	Page         int
	DateFilter   string
	DateFrom     string
	DateTo       string
	SearchQuery  string
	JobFilter    int64
	LoginURL     string
	CSRFToken    string
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	user, err := s.getOrCreateUser(r)
	if err != nil {
		http.Redirect(w, r, "/__exe.dev/login?redirect=/", http.StatusFound)
		return
	}
	
	q := s.Queries
	jobs, err := q.ListJobsByUser(r.Context(), user.ID)
	if err != nil {
		slog.Error("failed to list jobs", "error", err, "user_id", user.ID)
	}
	count, err := q.CountArticlesByUser(r.Context(), user.ID)
	if err != nil {
		slog.Error("failed to count articles", "error", err, "user_id", user.ID)
	}
	
	data := PageData{
		User:       user,
		Jobs:       jobs,
		TotalCount: count,
		CSRFToken:  s.getCSRFToken(r),
	}
	
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "dashboard.html", data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func (s *Server) handleJobsList(w http.ResponseWriter, r *http.Request) {
	user, err := s.getOrCreateUser(r)
	if err != nil {
		http.Redirect(w, r, "/__exe.dev/login?redirect=/jobs", http.StatusFound)
		return
	}
	
	q := s.Queries
	jobs, err := q.ListJobsByUser(r.Context(), user.ID)
	if err != nil {
		slog.Error("failed to list jobs", "error", err, "user_id", user.ID)
	}
	
	data := PageData{User: user, Jobs: jobs, CSRFToken: s.getCSRFToken(r)}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "jobs.html", data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func (s *Server) handleJobNew(w http.ResponseWriter, r *http.Request) {
	user, err := s.getOrCreateUser(r)
	if err != nil {
		http.Redirect(w, r, "/__exe.dev/login?redirect=/jobs/new", http.StatusFound)
		return
	}
	
	data := PageData{User: user, CSRFToken: s.getCSRFToken(r)}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "job_new.html", data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func (s *Server) handleJobDetail(w http.ResponseWriter, r *http.Request) {
	user, err := s.getOrCreateUser(r)
	if err != nil {
		http.Redirect(w, r, "/__exe.dev/login?redirect="+r.URL.Path, http.StatusFound)
		return
	}
	
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid job ID", http.StatusBadRequest)
		return
	}
	
	// Pagination
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := int64(DefaultPageLimit)
	offset := int64((page - 1)) * limit
	
	q := s.Queries
	job, err := q.GetJob(r.Context(), dbgen.GetJobParams{ID: id, UserID: user.ID})
	if err != nil {
		http.Error(w, "Job not found", 404)
		return
	}
	
	articles, err := q.ListArticlesByJobPaginated(r.Context(), dbgen.ListArticlesByJobPaginatedParams{
		JobID:  job.ID,
		UserID: user.ID,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		slog.Error("failed to list articles for job", "error", err, "job_id", job.ID)
	}
	
	count, err := q.CountArticlesByJob(r.Context(), dbgen.CountArticlesByJobParams{
		JobID:  job.ID,
		UserID: user.ID,
	})
	if err != nil {
		slog.Error("failed to count articles for job", "error", err, "job_id", job.ID)
	}
	
	data := PageData{User: user, Job: &job, Articles: articles, TotalCount: count, Page: page, CSRFToken: s.getCSRFToken(r)}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "job_detail.html", data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func (s *Server) handleJobEdit(w http.ResponseWriter, r *http.Request) {
	user, err := s.getOrCreateUser(r)
	if err != nil {
		http.Redirect(w, r, "/__exe.dev/login?redirect="+r.URL.Path, http.StatusFound)
		return
	}
	
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid job ID", http.StatusBadRequest)
		return
	}
	
	q := s.Queries
	job, err := q.GetJob(r.Context(), dbgen.GetJobParams{ID: id, UserID: user.ID})
	if err != nil {
		http.Error(w, "Job not found", 404)
		return
	}
	
	data := PageData{User: user, Job: &job, CSRFToken: s.getCSRFToken(r)}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "job_edit.html", data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

// queryArticles builds and executes a dynamic query based on filters.
// This replaces multiple sqlc queries with a single flexible implementation.
func (s *Server) queryArticles(r *http.Request, userID int64, f articlesFilter) ([]dbgen.Article, int64) {
	qb := newArticleQueryBuilder(userID, f)

	// Get count
	var count int64
	countQuery, countArgs := qb.buildCountQuery()
	s.DB.QueryRowContext(r.Context(), countQuery, countArgs...).Scan(&count)

	// Get articles
	articlesQuery, articlesArgs := qb.buildSelectQuery()
	rows, err := s.DB.QueryContext(r.Context(), articlesQuery, articlesArgs...)
	if err != nil {
		return nil, count
	}
	defer rows.Close()

	var articles []dbgen.Article
	for rows.Next() {
		var a dbgen.Article
		rows.Scan(&a.ID, &a.JobID, &a.UserID, &a.Title, &a.Url, &a.Summary, &a.ContentPath, &a.RetrievedAt)
		articles = append(articles, a)
	}
	return articles, count
}

// articleQueryBuilder constructs SQL queries for article listing with filters.
type articleQueryBuilder struct {
	conditions []string
	args       []interface{}
	limit      int64
	offset     int64
}

func newArticleQueryBuilder(userID int64, f articlesFilter) *articleQueryBuilder {
	qb := &articleQueryBuilder{
		conditions: []string{"user_id = ?"},
		args:       []interface{}{userID},
		limit:      f.Limit,
		offset:     f.Offset,
	}

	// Add filters (priority: search > job > date)
	switch {
	case f.SearchQuery != "":
		qb.addSearchFilter(f.SearchQuery)
	case f.JobFilter > 0:
		qb.conditions = append(qb.conditions, "job_id = ?")
		qb.args = append(qb.args, f.JobFilter)
	case f.UseCustomRange:
		qb.conditions = append(qb.conditions, "retrieved_at >= ?", "retrieved_at <= ?")
		qb.args = append(qb.args, f.SinceTime, f.UntilTime)
	case f.DateFilter != "":
		qb.conditions = append(qb.conditions, "retrieved_at >= ?")
		qb.args = append(qb.args, f.SinceTime)
	}

	return qb
}

func (qb *articleQueryBuilder) addSearchFilter(query string) {
	terms := parseSearchTerms(query)
	for _, term := range terms {
		pattern := "%" + term + "%"
		qb.conditions = append(qb.conditions, "(title LIKE ? OR summary LIKE ?)")
		qb.args = append(qb.args, pattern, pattern)
	}
}

func (qb *articleQueryBuilder) whereClause() string {
	return strings.Join(qb.conditions, " AND ")
}

func (qb *articleQueryBuilder) buildCountQuery() (string, []interface{}) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM articles WHERE %s", qb.whereClause())
	return query, qb.args
}

func (qb *articleQueryBuilder) buildSelectQuery() (string, []interface{}) {
	query := fmt.Sprintf(
		"SELECT id, job_id, user_id, title, url, summary, content_path, retrieved_at "+
			"FROM articles WHERE %s ORDER BY retrieved_at DESC LIMIT ? OFFSET ?",
		qb.whereClause(),
	)
	args := append(qb.args, qb.limit, qb.offset)
	return query, args
}

func (s *Server) handleArticlesList(w http.ResponseWriter, r *http.Request) {
	user, err := s.getOrCreateUser(r)
	if err != nil {
		http.Redirect(w, r, "/__exe.dev/login?redirect=/articles", http.StatusFound)
		return
	}
	
	f := parseArticlesFilters(r)
	articles, count := s.queryArticles(r, user.ID, f)
	
	// Get jobs list for the filter dropdown
	q := s.Queries
	jobs, _ := q.ListJobsByUser(r.Context(), user.ID)
	
	data := PageData{
		User:        user,
		Jobs:        jobs,
		Articles:    articles,
		TotalCount:  count,
		Page:        f.Page,
		DateFilter:  f.DateFilter,
		DateFrom:    f.DateFrom,
		DateTo:      f.DateTo,
		SearchQuery: f.SearchQuery,
		JobFilter:   f.JobFilter,
		CSRFToken:   s.getCSRFToken(r),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "articles.html", data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func (s *Server) handleArticleDetail(w http.ResponseWriter, r *http.Request) {
	user, err := s.getOrCreateUser(r)
	if err != nil {
		http.Redirect(w, r, "/__exe.dev/login?redirect="+r.URL.Path, http.StatusFound)
		return
	}
	
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid article ID", http.StatusBadRequest)
		return
	}
	
	q := s.Queries
	article, err := q.GetArticle(r.Context(), dbgen.GetArticleParams{ID: id, UserID: user.ID})
	if err != nil {
		http.Error(w, "Article not found", 404)
		return
	}
	
	data := PageData{User: user, Article: &article, CSRFToken: s.getCSRFToken(r)}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "article_detail.html", data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func (s *Server) handlePreferences(w http.ResponseWriter, r *http.Request) {
	user, err := s.getOrCreateUser(r)
	if err != nil {
		http.Redirect(w, r, "/__exe.dev/login?redirect=/preferences", http.StatusFound)
		return
	}
	
	q := s.Queries
	prefs, err := q.GetPreferences(r.Context(), user.ID)
	if err == sql.ErrNoRows {
		prefs, _ = q.CreatePreferences(r.Context(), user.ID)
	}
	
	data := PageData{User: user, Preferences: &prefs, CSRFToken: s.getCSRFToken(r)}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "preferences.html", data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request) {
	user, err := s.getOrCreateUser(r)
	if err != nil {
		http.Redirect(w, r, "/__exe.dev/login?redirect=/runs", http.StatusFound)
		return
	}
	
	q := s.Queries
	runningRuns, err := q.ListRunningJobRuns(r.Context(), user.ID)
	if err != nil {
		slog.Error("failed to list running job runs", "error", err, "user_id", user.ID)
	}
	recentRuns, err := q.ListRecentJobRuns(r.Context(), dbgen.ListRecentJobRunsParams{
		UserID: user.ID,
		Limit:  DefaultPageLimit,
	})
	if err != nil {
		slog.Error("failed to list recent job runs", "error", err, "user_id", user.ID)
	}
	
	data := PageData{User: user, RunningRuns: runningRuns, RecentRuns: recentRuns, CSRFToken: s.getCSRFToken(r)}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "runs.html", data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}
