// Package jobrunner implements the news job execution logic.
// It replaces the run-job.sh bash script with a proper Go implementation.
package jobrunner

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/exedev/news-app/internal/db/dbgen"
	"github.com/exedev/news-app/internal/util"
)

// Config holds configuration for the job runner.
type Config struct {
	DBPath       string
	ArticlesDir  string
	LogsDir      string
	ShelleyAPI   string
	JobTimeout   time.Duration
	PollInterval time.Duration
	StartDelay   time.Duration // Max random delay to stagger job starts
	MaxParallel  int           // Max concurrent article fetches
}

func getEnvInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultVal
}

// DefaultConfig returns configuration with sensible defaults.
func DefaultConfig() Config {
	return Config{
		DBPath:       util.GetEnv("NEWS_APP_DB_PATH", "/home/exedev/news-app/db.sqlite3"),
		ArticlesDir:  util.GetEnv("NEWS_APP_ARTICLES_DIR", "/home/exedev/news-app/articles"),
		LogsDir:      util.GetEnv("NEWS_APP_LOGS_DIR", "/home/exedev/news-app/logs/runs"),
		ShelleyAPI:   util.GetEnv("NEWS_APP_SHELLEY_API", "http://localhost:9999"),
		JobTimeout:   time.Duration(getEnvInt("NEWS_JOB_TIMEOUT_SECS", 25*60)) * time.Second,
		PollInterval: time.Duration(getEnvInt("NEWS_JOB_POLL_INTERVAL_SECS", 10)) * time.Second,
		StartDelay:   time.Duration(getEnvInt("NEWS_JOB_START_DELAY_SECS", 60)) * time.Second,
		MaxParallel:  getEnvInt("NEWS_JOB_MAX_PARALLEL", 5),
	}
}

// Runner executes news retrieval jobs.
type Runner struct {
	config  Config
	db      *sql.DB
	queries *dbgen.Queries
	shelley *ShelleyClient
	logger  *slog.Logger
	logFile *os.File
}

// NewRunner creates a new job runner.
func NewRunner(db *sql.DB, config Config) *Runner {
	return &Runner{
		config:  config,
		db:      db,
		queries: dbgen.New(db),
		shelley: NewShelleyClient(config.ShelleyAPI),
		logger:  slog.Default(),
	}
}

// Run executes a job by ID. This is the main entry point.
// Resume continues an existing job run that was interrupted.
func (r *Runner) Resume(ctx context.Context, runID int64) error {
	// Load the existing run
	var run struct {
		ID      int64
		JobID   int64
		Status  string
		LogPath string
	}

	err := r.db.QueryRowContext(ctx, `
		SELECT id, job_id, status, log_path FROM job_runs WHERE id=?
	`, runID).Scan(&run.ID, &run.JobID, &run.Status, &run.LogPath)
	if err != nil {
		return fmt.Errorf("run not found: %w", err)
	}

	if run.Status != "running" {
		return fmt.Errorf("run %d is not in running state (status: %s)", runID, run.Status)
	}

	// Load job and preferences
	job, err := r.queries.GetJobByID(ctx, run.JobID)
	if err != nil {
		return fmt.Errorf("job not found: %w", err)
	}

	prefs, err := r.queries.GetPreferences(ctx, job.UserID)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("get preferences: %w", err)
	}

	// Set up logging to file (append mode)
	if err := r.setupLoggingAppend(run.LogPath); err != nil {
		r.logger.Warn("setup logging", "error", err)
	}
	defer r.closeLogging()

	r.logger.Info("resuming job run",
		"job_id", run.JobID,
		"run_id", run.ID,
		"job_name", job.Name,
	)

	// Execute the job (will check for existing conversation)
	result := r.executeJob(ctx, job, prefs)

	// If the context was cancelled (e.g. SIGTERM during restart), leave the run
	// in "running" state so it can be resumed on next startup. Don't finalize
	// or send notifications — that should only happen at the real end of a run.
	if ctx.Err() != nil {
		r.logger.Info("context cancelled, leaving run in running state for resume",
			"run_id", run.ID, "reason", ctx.Err())
		return ctx.Err()
	}

	// Finalize the run
	r.finalizeRun(ctx, job, run.ID, result, prefs)

	return result.Error
}

func (r *Runner) Run(ctx context.Context, jobID int64) error {
	// Random delay to stagger concurrent job starts
	if r.config.StartDelay > 0 {
		delay := time.Duration(rand.Int63n(int64(r.config.StartDelay)))
		r.logger.Info("delaying job start", "delay", delay)
		time.Sleep(delay)
	}

	// Load job and preferences
	job, err := r.queries.GetJobByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("job not found: %w", err)
	}

	prefs, err := r.queries.GetPreferences(ctx, job.UserID)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("get preferences: %w", err)
	}

	// Cancel orphaned runs
	if err := r.cancelOrphanedRuns(ctx, jobID); err != nil {
		r.logger.Warn("cancel orphaned runs", "error", err)
	}

	// Create job run record
	run, err := r.queries.CreateJobRun(ctx, jobID)
	if err != nil {
		return fmt.Errorf("create job run: %w", err)
	}

	// Update job status
	r.queries.UpdateJobStatus(ctx, dbgen.UpdateJobStatusParams{
		ID:        jobID,
		Status:    util.StatusRunning,
		NextRunAt: job.NextRunAt,
	})

	// Set up logging to file
	if err := r.setupLogging(run.ID); err != nil {
		r.logger.Warn("setup logging", "error", err)
	}
	defer r.closeLogging()

	r.logger.Info("job run started",
		"job_id", jobID,
		"run_id", run.ID,
		"job_name", job.Name,
	)

	// Execute the job
	result := r.executeJob(ctx, job, prefs)

	// If the context was cancelled (e.g. SIGTERM during restart), leave the run
	// in "running" state so it can be resumed on next startup. Don't finalize
	// or send notifications — that should only happen at the real end of a run.
	if ctx.Err() != nil {
		r.logger.Info("context cancelled, leaving run in running state for resume",
			"run_id", run.ID, "reason", ctx.Err())
		return ctx.Err()
	}

	// Update final status
	r.finalizeRun(ctx, job, run.ID, result, prefs)

	if result.Error != nil {
		return result.Error
	}
	return nil
}

// JobResult holds the outcome of a job execution.
type JobResult struct {
	ArticlesSaved     int
	DuplicatesSkipped int
	ConversationID    string
	Error             error
}

func (r *Runner) executeJob(ctx context.Context, job dbgen.Job, prefs dbgen.Preference) JobResult {
	result := JobResult{}

	// Build prompt
	prompt := r.buildPrompt(job, prefs)

	// Create articles directory
	jobArticlesDir := filepath.Join(r.config.ArticlesDir, fmt.Sprintf("job_%d", job.ID))
	if err := os.MkdirAll(jobArticlesDir, 0755); err != nil {
		result.Error = fmt.Errorf("create articles dir: %w", err)
		return result
	}

	// Check for existing conversation
	convID, shouldCreate := r.checkExistingConversation(ctx, job)

	// Create new conversation if needed
	if shouldCreate {
		var err error
		convID, err = r.shelley.CreateConversation(ctx, job.ID, prompt)
		if err != nil {
			result.Error = fmt.Errorf("create conversation: %w", err)
			return result
		}
		r.logger.Info("created conversation", "conversation_id", convID)
	}
	result.ConversationID = convID

	// Store conversation ID
	r.queries.UpdateJobConversation(ctx, dbgen.UpdateJobConversationParams{
		ID:                    job.ID,
		CurrentConversationID: &convID,
	})

	// Poll for completion
	conv, err := r.pollForCompletion(ctx, job.ID, convID)
	if err != nil {
		result.Error = err
		return result
	}

	// Extract articles from response
	responseText := conv.GetLastAgentText()
	articles, err := ExtractArticlesJSON(responseText)
	if err != nil {
		r.logger.Error("extract articles JSON", "error", err)
		result.Error = fmt.Errorf("failed to extract articles: %w", err)
		return result
	}

	// Fetch content and save articles
	if len(articles) > 0 {
		saved, dups := r.processArticles(ctx, job, articles, jobArticlesDir)
		result.ArticlesSaved = saved
		result.DuplicatesSkipped = dups
	}

	// Archive conversation
	r.archiveConversation(ctx, job.ID, convID)

	return result
}

func (r *Runner) buildPrompt(job dbgen.Job, prefs dbgen.Preference) string {
	var b strings.Builder

	b.WriteString(`You are a news retrieval agent. Your task is to search the web for news articles based on the user's request.

`)

	if prefs.SystemPrompt != "" {
		b.WriteString(prefs.SystemPrompt)
		b.WriteString("\n\n")
	}

	b.WriteString("USER REQUEST: ")
	b.WriteString(job.Prompt)
	b.WriteString("\n\n")

	if job.Keywords != "" {
		b.WriteString("KEYWORDS TO FOCUS ON: ")
		b.WriteString(job.Keywords)
		b.WriteString("\n\n")
	}

	if job.Sources != "" {
		b.WriteString("PREFERRED SOURCES: ")
		b.WriteString(job.Sources)
		b.WriteString("\n\n")
	}

	if job.Region != "" {
		b.WriteString("GEOGRAPHIC FOCUS: ")
		b.WriteString(job.Region)
		b.WriteString("\n\n")
	}

	b.WriteString(`Please search the web for relevant news articles. For each article found, provide:
1. Title
2. URL  
3. Brief summary (2-3 sentences)

Format your response as a JSON array ONLY (no other text):
[{"title": "...", "url": "...", "summary": "..."}]

**IMPORTANT**: When using a subagent to search the web, always wait for it to fully complete its work before returning. Do not return until the subagent has finished and provided its full results.

Search the web now and return the results.`)

	return b.String()
}

func (r *Runner) checkExistingConversation(ctx context.Context, job dbgen.Job) (string, bool) {
	if job.CurrentConversationID == nil || *job.CurrentConversationID == "" {
		return "", true
	}

	convID := *job.CurrentConversationID
	r.logger.Info("checking existing conversation", "conversation_id", convID)

	conv, err := r.shelley.GetConversation(ctx, job.ID, convID)
	if err != nil {
		r.logger.Info("existing conversation not found, creating new")
		return "", true
	}

	if conv.IsComplete() {
		r.logger.Info("existing conversation already complete, creating new")
		return "", true
	}

	r.logger.Info("resuming existing conversation")
	return convID, false
}

func (r *Runner) pollForCompletion(ctx context.Context, jobID int64, convID string) (*Conversation, error) {
	timeout := time.After(r.config.JobTimeout)
	ticker := time.NewTicker(r.config.PollInterval)
	defer ticker.Stop()

	waited := time.Duration(0)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case <-timeout:
			// Try to cancel stuck conversation
			r.shelley.DeleteConversation(ctx, jobID, convID)
			return nil, fmt.Errorf("job timed out after %v", r.config.JobTimeout)

		case <-ticker.C:
			waited += r.config.PollInterval

			conv, err := r.shelley.GetConversation(ctx, jobID, convID)
			if err != nil {
				r.logger.Warn("poll conversation", "error", err, "waited", waited)
				continue
			}

			if conv.IsComplete() {
				r.logger.Info("agent finished", "waited", waited)
				return conv, nil
			}

			r.logger.Debug("waiting for agent", "waited", waited)
		}
	}
}

// ProcessArticles processes and saves articles for a job (public wrapper).
func (r *Runner) ProcessArticles(ctx context.Context, jobID int64, articles []ArticleInfo) (saved, dups int, err error) {
	job, err := r.queries.GetJobByID(ctx, jobID)
	if err != nil {
		return 0, 0, fmt.Errorf("get job: %w", err)
	}

	// Create articles directory for this user
	articlesDir := filepath.Join(r.config.ArticlesDir, fmt.Sprintf("user_%d", job.UserID))
	if err := os.MkdirAll(articlesDir, 0755); err != nil {
		return 0, 0, fmt.Errorf("create articles dir: %w", err)
	}

	saved, dups = r.processArticles(ctx, job, articles, articlesDir)
	return saved, dups, nil
}

func (r *Runner) processArticles(ctx context.Context, job dbgen.Job, articles []ArticleInfo, articlesDir string) (saved, dups int) {
	timestamp := time.Now().Format("20060102_150405")

	// Fetch content in parallel
	contents := r.fetchArticleContents(ctx, articles)

	for i, info := range articles {
		content := contents[i]

		// Create article file
		articleFile := filepath.Join(articlesDir, fmt.Sprintf("article_%d_%s.txt", i+1, timestamp))
		if err := r.writeArticleFile(articleFile, info, content); err != nil {
			r.logger.Warn("write article file", "error", err)
			continue
		}

		// Insert into database
		inserted, err := r.insertArticle(ctx, job, info, articleFile)
		if err != nil {
			r.logger.Warn("insert article", "error", err)
			continue
		}

		if inserted {
			saved++
			r.logger.Info("saved article", "title", info.Title, "file", articleFile)
		} else {
			dups++
			r.logger.Info("skipped duplicate", "title", info.Title)
		}
	}

	return saved, dups
}

func (r *Runner) fetchArticleContents(ctx context.Context, articles []ArticleInfo) []string {
	contents := make([]string, len(articles))
	var wg sync.WaitGroup
	sem := make(chan struct{}, r.config.MaxParallel)

	for i, info := range articles {
		if info.URL == "" {
			contents[i] = "(No URL provided)"
			continue
		}

		wg.Add(1)
		go func(idx int, url string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			r.logger.Info("fetching content", "url", url)
			content, err := FetchArticleContent(ctx, url)
			if err != nil {
				contents[idx] = fmt.Sprintf("[Error fetching article: %v]", err)
			} else {
				contents[idx] = content
			}
		}(i, info.URL)
	}

	wg.Wait()
	return contents
}

func (r *Runner) writeArticleFile(path string, info ArticleInfo, content string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "Title: %s\n", info.Title)
	fmt.Fprintf(f, "URL: %s\n", info.URL)
	fmt.Fprintf(f, "Retrieved: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintln(f)
	fmt.Fprintln(f, "--- Summary ---")
	fmt.Fprintln(f, info.Summary)
	fmt.Fprintln(f)
	fmt.Fprintln(f, "--- Full Content ---")
	fmt.Fprintln(f, content)

	return nil
}

func (r *Runner) insertArticle(ctx context.Context, job dbgen.Job, info ArticleInfo, contentPath string) (bool, error) {
	// Check if article already exists (by URL)
	exists, err := r.queries.ArticleExistsByURL(ctx, dbgen.ArticleExistsByURLParams{
		UserID: job.UserID,
		Url:    info.URL,
	})
	if err != nil {
		return false, err
	}
	if exists > 0 {
		return false, nil // duplicate
	}

	_, err = r.queries.CreateArticle(ctx, dbgen.CreateArticleParams{
		JobID:       job.ID,
		UserID:      job.UserID,
		Title:       info.Title,
		Url:         info.URL,
		Summary:     info.Summary,
		ContentPath: contentPath,
	})
	if err != nil {
		return false, err
	}

	return true, nil
}

func (r *Runner) archiveConversation(ctx context.Context, jobID int64, convID string) {
	if convID == "" {
		return
	}

	r.logger.Info("archiving conversation", "conversation_id", convID)
	if err := r.shelley.ArchiveConversation(ctx, jobID, convID); err != nil {
		r.logger.Warn("archive conversation", "error", err)
	}

	// Archive subagents
	subagents, err := r.shelley.ListSubagents(ctx, jobID, convID)
	if err != nil {
		r.logger.Warn("list subagents", "error", err)
		return
	}

	for _, subID := range subagents {
		r.logger.Info("archiving subagent", "conversation_id", subID)
		r.shelley.ArchiveConversation(ctx, jobID, subID)
	}
}

func (r *Runner) cancelOrphanedRuns(ctx context.Context, jobID int64) error {
	return r.queries.CancelOrphanedRuns(ctx, jobID)
}

func (r *Runner) finalizeRun(ctx context.Context, job dbgen.Job, runID int64, result JobResult, prefs dbgen.Preference) {
	// Check if run is still in running state (prevent double finalization)
	var currentStatus string
	err := r.db.QueryRowContext(ctx, "SELECT status FROM job_runs WHERE id = ?", runID).Scan(&currentStatus)
	if err != nil || currentStatus != "running" {
		r.logger.Info("run already finalized, skipping", "run_id", runID, "current_status", currentStatus)
		return
	}

	now := time.Now()

	// Determine run status
	var runStatus string
	var errorMsg string
	if result.Error != nil {
		runStatus = util.StatusFailed
		errorMsg = result.Error.Error()
	} else if result.ArticlesSaved == 0 {
		runStatus = "completed_no_new"
	} else {
		runStatus = util.StatusCompleted
	}

	// Update job run
	articlesSaved := int64(result.ArticlesSaved)
	duplicatesSkipped := int64(result.DuplicatesSkipped)
	r.queries.UpdateJobRunComplete(ctx, dbgen.UpdateJobRunCompleteParams{
		ID:                runID,
		Status:            runStatus,
		ErrorMessage:      &errorMsg,
		ArticlesSaved:     &articlesSaved,
		DuplicatesSkipped: &duplicatesSkipped,
	})

	// Calculate next run time
	var nextRunAt *time.Time
	if job.IsOneTime == 0 && result.Error == nil {
		next := util.CalculateNextRun(job.Frequency, false)
		nextRunAt = &next
	}

	// Update job status
	jobStatus := util.StatusCompleted
	if result.Error != nil {
		jobStatus = util.StatusFailed
	}

	if job.IsOneTime == 1 {
		// Deactivate one-time jobs
		r.queries.DeactivateJob(ctx, job.ID)
	}

	r.queries.UpdateJobStatus(ctx, dbgen.UpdateJobStatusParams{
		ID:        job.ID,
		Status:    jobStatus,
		LastRunAt: &now,
		NextRunAt: nextRunAt,
	})

	// Clear conversation ID
	emptyConvID := ""
	r.queries.UpdateJobConversation(ctx, dbgen.UpdateJobConversationParams{
		ID:                    job.ID,
		CurrentConversationID: &emptyConvID,
	})

	// Send notifications
	r.sendNotification(prefs, job.Name, result)

	r.logger.Info("job run completed",
		"status", runStatus,
		"articles_saved", result.ArticlesSaved,
		"duplicates_skipped", result.DuplicatesSkipped,
	)
}


func (r *Runner) sendNotification(prefs dbgen.Preference, jobName string, result JobResult) {
	if prefs.DiscordWebhook == "" {
		return
	}

	var msg string
	if result.Error != nil {
		if prefs.NotifyFailure == 0 {
			return
		}
		msg = fmt.Sprintf("❌ News job '%s' failed: %v", jobName, result.Error)
	} else {
		if prefs.NotifySuccess == 0 {
			return
		}
		if result.ArticlesSaved == 0 {
			msg = fmt.Sprintf("ℹ️ News job '%s' completed - no new articles found", jobName)
		} else {
			msg = fmt.Sprintf("✅ News job '%s' completed! (%d new articles)", jobName, result.ArticlesSaved)
		}
	}

	if err := SendDiscordNotification(prefs.DiscordWebhook, msg); err != nil {
		r.logger.Warn("send discord notification", "error", err)
	}
}

func (r *Runner) setupLogging(runID int64) error {
	if err := os.MkdirAll(r.config.LogsDir, 0755); err != nil {
		return err
	}

	logPath := filepath.Join(r.config.LogsDir, fmt.Sprintf("run_%d_%s.log", runID, time.Now().Format("20060102_150405")))

	// Update run with log path
	r.queries.UpdateJobRunLogPath(context.Background(), dbgen.UpdateJobRunLogPathParams{
		ID:      runID,
		LogPath: logPath,
	})

	var err error
	r.logFile, err = os.Create(logPath)
	if err != nil {
		return err
	}

	// Create multi-writer for stdout + file
	multiWriter := io.MultiWriter(os.Stdout, r.logFile)
	r.logger = slog.New(slog.NewTextHandler(multiWriter, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	return nil
}

func (r *Runner) setupLoggingAppend(logPath string) error {
	// Open existing log file in append mode
	var err error
	r.logFile, err = os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	// Create multi-writer: log to both file and original logger
	multi := io.MultiWriter(r.logFile, os.Stderr)

	// Replace logger handler to write to file
	handler := slog.NewTextHandler(multi, &slog.HandlerOptions{})
	r.logger = slog.New(handler)

	return nil
}

func (r *Runner) closeLogging() {
	if r.logFile != nil {
		r.logFile.Close()
	}
}

// ArticleInfo holds metadata about an article from the agent's response.
type ArticleInfo struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Summary string `json:"summary"`
}

// ExtractArticlesJSON extracts and parses the JSON array from agent response.
func ExtractArticlesJSON(text string) ([]ArticleInfo, error) {
	jsonStr, err := extractJSONArray(text)
	if err != nil {
		return nil, err
	}

	var articles []ArticleInfo
	if err := json.Unmarshal([]byte(jsonStr), &articles); err != nil {
		// Try fixing malformed JSON
		fixed := fixMalformedJSON(jsonStr)
		if err := json.Unmarshal([]byte(fixed), &articles); err != nil {
			return nil, fmt.Errorf("parse JSON: %w", err)
		}
	}

	return articles, nil
}
