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

// parseArticlesFilters extracts filter parameters from the request
func parseArticlesFilters(r *http.Request) articlesFilter {
	f := articlesFilter{}
	
	// Pagination
	f.Page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	if f.Page < 1 {
		f.Page = 1
	}
	f.Limit = int64(DefaultPageLimit)
	f.Offset = int64((f.Page - 1)) * f.Limit
	
	// Search, job, and date filters
	f.SearchQuery = r.URL.Query().Get("q")
	f.JobFilter, _ = strconv.ParseInt(r.URL.Query().Get("job"), 10, 64)
	f.DateFilter = r.URL.Query().Get("filter")
	f.DateFrom = r.URL.Query().Get("from")
	f.DateTo = r.URL.Query().Get("to")
	
	// Parse date range
	if f.DateFrom != "" || f.DateTo != "" {
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
	} else {
		switch f.DateFilter {
		case "day":
			f.SinceTime = time.Now().AddDate(0, 0, -1)
		case "week":
			f.SinceTime = time.Now().AddDate(0, 0, -7)
		case "month":
			f.SinceTime = time.Now().AddDate(0, -1, 0)
		default:
			f.DateFilter = ""
		}
	}
	
	return f
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
	
	q := dbgen.New(s.DB)
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
	
	q := dbgen.New(s.DB)
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
	
	q := dbgen.New(s.DB)
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
	
	q := dbgen.New(s.DB)
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

// queryArticles executes the appropriate query based on filters
func (s *Server) queryArticles(r *http.Request, userID int64, f articlesFilter) ([]dbgen.Article, int64) {
	q := dbgen.New(s.DB)
	var articles []dbgen.Article
	var count int64
	
	// Priority: search > job > date filters
	switch {
	case f.SearchQuery != "":
		articles, count = s.searchArticles(r, userID, f)
	case f.JobFilter > 0:
		articles, _ = q.ListArticlesByJobPaginated(r.Context(), dbgen.ListArticlesByJobPaginatedParams{
			JobID:  f.JobFilter,
			UserID: userID,
			Limit:  f.Limit,
			Offset: f.Offset,
		})
		count, _ = q.CountArticlesByJob(r.Context(), dbgen.CountArticlesByJobParams{
			JobID:  f.JobFilter,
			UserID: userID,
		})
	case f.UseCustomRange:
		articles, _ = q.ListArticlesByUserDateRange(r.Context(), dbgen.ListArticlesByUserDateRangeParams{
			UserID:        userID,
			RetrievedAt:   f.SinceTime,
			RetrievedAt_2: f.UntilTime,
			Limit:         f.Limit,
			Offset:        f.Offset,
		})
		count, _ = q.CountArticlesByUserDateRange(r.Context(), dbgen.CountArticlesByUserDateRangeParams{
			UserID:        userID,
			RetrievedAt:   f.SinceTime,
			RetrievedAt_2: f.UntilTime,
		})
	case f.DateFilter != "":
		articles, _ = q.ListArticlesByUserSince(r.Context(), dbgen.ListArticlesByUserSinceParams{
			UserID:      userID,
			RetrievedAt: f.SinceTime,
			Limit:       f.Limit,
			Offset:      f.Offset,
		})
		count, _ = q.CountArticlesByUserSince(r.Context(), dbgen.CountArticlesByUserSinceParams{
			UserID:      userID,
			RetrievedAt: f.SinceTime,
		})
	default:
		articles, _ = q.ListArticlesByUser(r.Context(), dbgen.ListArticlesByUserParams{
			UserID: userID,
			Limit:  f.Limit,
			Offset: f.Offset,
		})
		count, _ = q.CountArticlesByUser(r.Context(), userID)
	}
	
	return articles, count
}

// searchArticles performs a full-text search across article titles and summaries
func (s *Server) searchArticles(r *http.Request, userID int64, f articlesFilter) ([]dbgen.Article, int64) {
	var articles []dbgen.Article
	var count int64
	
	terms := parseSearchTerms(f.SearchQuery)
	if len(terms) == 0 {
		return articles, count
	}
	
	// Build dynamic query with AND conditions for each term
	var conditions []string
	var args []interface{}
	args = append(args, userID)
	
	for _, term := range terms {
		pattern := "%" + term + "%"
		conditions = append(conditions, "(title LIKE ? OR summary LIKE ?)")
		args = append(args, pattern, pattern)
	}
	
	whereClause := strings.Join(conditions, " AND ")
	
	// Count query
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM articles WHERE user_id = ? AND %s", whereClause)
	s.DB.QueryRowContext(r.Context(), countQuery, args...).Scan(&count)
	
	// Articles query with pagination
	articlesQuery := fmt.Sprintf(
		"SELECT id, job_id, user_id, title, url, summary, content_path, retrieved_at "+
			"FROM articles WHERE user_id = ? AND %s ORDER BY retrieved_at DESC LIMIT ? OFFSET ?",
		whereClause,
	)
	args = append(args, f.Limit, f.Offset)
	rows, err := s.DB.QueryContext(r.Context(), articlesQuery, args...)
	if err != nil {
		return articles, count
	}
	defer rows.Close()
	
	for rows.Next() {
		var a dbgen.Article
		rows.Scan(&a.ID, &a.JobID, &a.UserID, &a.Title, &a.Url, &a.Summary, &a.ContentPath, &a.RetrievedAt)
		articles = append(articles, a)
	}
	
	return articles, count
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
	q := dbgen.New(s.DB)
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
	
	q := dbgen.New(s.DB)
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
	
	q := dbgen.New(s.DB)
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
	
	q := dbgen.New(s.DB)
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
