package srv

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"srv.exe.dev/db/dbgen"
	"srv.exe.dev/internal/util"
)

type CreateJobRequest struct {
	Name      string `json:"name"`
	Prompt    string `json:"prompt"`
	Keywords  string `json:"keywords"`
	Sources   string `json:"sources"`
	Region    string `json:"region"`
	Frequency string `json:"frequency"`
	IsOneTime bool   `json:"is_one_time"`
}

type UpdateJobRequest struct {
	Name      string `json:"name"`
	Prompt    string `json:"prompt"`
	Keywords  string `json:"keywords"`
	Sources   string `json:"sources"`
	Region    string `json:"region"`
	Frequency string `json:"frequency"`
	IsActive  bool   `json:"is_active"`
}

type UpdatePreferencesRequest struct {
	SystemPrompt  string `json:"system_prompt"`
	DiscordWebhook string `json:"discord_webhook"`
	NotifySuccess bool   `json:"notify_success"`
	NotifyFailure bool   `json:"notify_failure"`
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	user, err := s.getOrCreateUser(r)
	if err != nil {
		s.jsonError(w, "Unauthorized", 401)
		return
	}
	
	// Rate limit job creation per user
	rateLimitKey := fmt.Sprintf("create-job:%d", user.ID)
	if !s.rateLimiter.Allow(rateLimitKey) {
		s.jsonError(w, "Rate limit exceeded: please wait before creating another job", http.StatusTooManyRequests)
		return
	}
	
	var req CreateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	
	if req.Name == "" || req.Prompt == "" {
		s.jsonError(w, "Invalid request: name and prompt are required", http.StatusBadRequest)
		return
	}
	
	nextRun := util.CalculateNextRun(req.Frequency, req.IsOneTime)
	
	q := s.Queries
	job, err := q.CreateJob(r.Context(), dbgen.CreateJobParams{
		UserID:    user.ID,
		Name:      req.Name,
		Prompt:    req.Prompt,
		Keywords:  req.Keywords,
		Sources:   req.Sources,
		Region:    req.Region,
		Frequency: req.Frequency,
		IsOneTime: boolToInt64(req.IsOneTime),
		NextRunAt: &nextRun,
	})
	if err != nil {
		s.jsonError(w, "Failed to create job", http.StatusInternalServerError)
		return
	}
	
	// Create systemd timer
	if err := createSystemdTimer(job); err != nil {
		slog.Warn("failed to create systemd timer", "job_id", job.ID, "error", err)
	}
	
	slog.Info("job created", "job_id", job.ID, "user_id", user.ID, "name", job.Name)
	s.jsonOK(w, job)
}

func (s *Server) handleUpdateJob(w http.ResponseWriter, r *http.Request) {
	user, err := s.getOrCreateUser(r)
	if err != nil {
		s.jsonError(w, "Unauthorized", 401)
		return
	}
	
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.jsonError(w, "Invalid job ID", http.StatusBadRequest)
		return
	}
	
	var req UpdateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	
	q := s.Queries
	err = q.UpdateJob(r.Context(), dbgen.UpdateJobParams{
		Name:      req.Name,
		Prompt:    req.Prompt,
		Keywords:  req.Keywords,
		Sources:   req.Sources,
		Region:    req.Region,
		Frequency: req.Frequency,
		IsActive:  boolToInt64(req.IsActive),
		ID:        id,
		UserID:    user.ID,
	})
	if err != nil {
		slog.Error("failed to update job", "job_id", id, "user_id", user.ID, "error", err)
		s.jsonError(w, "Failed to update job", http.StatusInternalServerError)
		return
	}
	
	// Update systemd timer
	job, _ := q.GetJob(r.Context(), dbgen.GetJobParams{ID: id, UserID: user.ID})
	updateSystemdTimer(job)
	
	slog.Info("job updated", "job_id", id, "user_id", user.ID)
	s.jsonOK(w, map[string]string{"status": "ok"})
}

func (s *Server) handleDeleteJob(w http.ResponseWriter, r *http.Request) {
	user, err := s.getOrCreateUser(r)
	if err != nil {
		s.jsonError(w, "Unauthorized", 401)
		return
	}
	
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.jsonError(w, "Invalid job ID", http.StatusBadRequest)
		return
	}
	
	// Remove systemd timer first
	removeSystemdTimer(id)
	
	q := s.Queries
	err = q.DeleteJob(r.Context(), dbgen.DeleteJobParams{ID: id, UserID: user.ID})
	if err != nil {
		slog.Error("failed to delete job", "job_id", id, "user_id", user.ID, "error", err)
		s.jsonError(w, "Failed to delete job", http.StatusInternalServerError)
		return
	}
	
	slog.Info("job deleted", "job_id", id, "user_id", user.ID)
	s.jsonOK(w, map[string]string{"status": "ok"})
}

func (s *Server) handleRunJob(w http.ResponseWriter, r *http.Request) {
	user, err := s.getOrCreateUser(r)
	if err != nil {
		s.jsonError(w, "Unauthorized", 401)
		return
	}
	
	// Rate limit job runs per user
	rateLimitKey := fmt.Sprintf("run-job:%d", user.ID)
	if !s.rateLimiter.Allow(rateLimitKey) {
		s.jsonError(w, "Rate limit exceeded: please wait before running another job", http.StatusTooManyRequests)
		return
	}
	
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.jsonError(w, "Invalid job ID", http.StatusBadRequest)
		return
	}
	
	q := s.Queries
	job, err := q.GetJob(r.Context(), dbgen.GetJobParams{ID: id, UserID: user.ID})
	if err != nil {
		s.jsonError(w, "Job not found", 404)
		return
	}
	
	if job.Status == "running" {
		s.jsonError(w, "Job is already running", http.StatusBadRequest)
		return
	}
	
	// Run immediately via systemd
	serviceName := fmt.Sprintf("news-job-%d", job.ID)
	cmd := exec.Command("sudo", "systemctl", "start", serviceName+".service")
	if err := cmd.Run(); err != nil {
		slog.Warn("systemd start failed, running directly", "job_id", job.ID, "error", err)
		go runJobDirectly(s.DB, job.ID)
	}
	
	slog.Info("job started", "job_id", job.ID, "user_id", user.ID, "name", job.Name)
	s.jsonOK(w, map[string]string{"status": "started"})
}

func (s *Server) handleStopJob(w http.ResponseWriter, r *http.Request) {
	user, err := s.getOrCreateUser(r)
	if err != nil {
		s.jsonError(w, "Unauthorized", 401)
		return
	}
	
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.jsonError(w, "Invalid job ID", http.StatusBadRequest)
		return
	}
	
	q := s.Queries
	job, err := q.GetJob(r.Context(), dbgen.GetJobParams{ID: id, UserID: user.ID})
	if err != nil {
		s.jsonError(w, "Job not found", 404)
		return
	}
	
	if job.Status != "running" {
		s.jsonError(w, "Job is not running", http.StatusBadRequest)
		return
	}
	
	// Stop via systemd
	serviceName := fmt.Sprintf("news-job-%d", job.ID)
	cmd := exec.Command("sudo", "systemctl", "stop", serviceName+".service")
	cmd.Run()
	
	// Update job status to stopped/failed, preserving next_run_at
	now := time.Now()
	q.UpdateJobStatus(r.Context(), dbgen.UpdateJobStatusParams{
		Status:    "stopped",
		LastRunAt: &now,
		NextRunAt: job.NextRunAt,
		ID:        job.ID,
	})
	
	slog.Info("job stopped", "job_id", job.ID, "user_id", user.ID)
	s.jsonOK(w, map[string]string{"status": "stopped"})
}

func (s *Server) handleCancelRun(w http.ResponseWriter, r *http.Request) {
	user, err := s.getOrCreateUser(r)
	if err != nil {
		s.jsonError(w, "Unauthorized", 401)
		return
	}
	
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.jsonError(w, "Invalid run ID", http.StatusBadRequest)
		return
	}
	
	q := s.Queries
	
	// Verify the run belongs to this user
	run, err := q.GetJobRun(r.Context(), dbgen.GetJobRunParams{ID: id, UserID: user.ID})
	if err != nil {
		s.jsonError(w, "Run not found", 404)
		return
	}
	
	if run.Status != "running" {
		s.jsonError(w, "Run is not running", http.StatusBadRequest)
		return
	}
	
	// Try to stop the systemd service if it's still running
	serviceName := fmt.Sprintf("news-job-%d", run.JobID)
	cmd := exec.Command("sudo", "systemctl", "stop", serviceName+".service")
	cmd.Run() // Ignore errors - service may not be running
	
	// Mark the run as cancelled
	if err := q.CancelJobRun(r.Context(), id); err != nil {
		slog.Error("failed to cancel run", "run_id", id, "user_id", user.ID, "error", err)
		s.jsonError(w, "Failed to cancel run", http.StatusInternalServerError)
		return
	}
	
	// Also update job status if it's still marked as running, preserving next_run_at
	job, _ := q.GetJobByID(r.Context(), run.JobID)
	if job.Status == "running" {
		now := time.Now()
		q.UpdateJobStatus(r.Context(), dbgen.UpdateJobStatusParams{
			Status:    "cancelled",
			LastRunAt: &now,
			NextRunAt: job.NextRunAt,
			ID:        run.JobID,
		})
	}
	
	slog.Info("run cancelled", "run_id", id, "job_id", run.JobID, "user_id", user.ID)
	s.jsonOK(w, map[string]string{"status": "cancelled"})
}

func (s *Server) handleUpdatePreferences(w http.ResponseWriter, r *http.Request) {
	user, err := s.getOrCreateUser(r)
	if err != nil {
		s.jsonError(w, "Unauthorized", 401)
		return
	}
	
	var req UpdatePreferencesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	
	q := s.Queries
	
	// Ensure preferences exist
	_, err = q.GetPreferences(r.Context(), user.ID)
	if err == sql.ErrNoRows {
		q.CreatePreferences(r.Context(), user.ID)
	}
	
	err = q.UpdatePreferences(r.Context(), dbgen.UpdatePreferencesParams{
		SystemPrompt:   req.SystemPrompt,
		DiscordWebhook: req.DiscordWebhook,
		NotifySuccess:  boolToInt64(req.NotifySuccess),
		NotifyFailure:  boolToInt64(req.NotifyFailure),
		UserID:         user.ID,
	})
	if err != nil {
		s.jsonError(w, "Failed to update preferences", http.StatusInternalServerError)
		return
	}
	
	s.jsonOK(w, map[string]string{"status": "ok"})
}

func (s *Server) handleArticleContent(w http.ResponseWriter, r *http.Request) {
	user, err := s.getOrCreateUser(r)
	if err != nil {
		s.jsonError(w, "Unauthorized", 401)
		return
	}
	
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.jsonError(w, "Invalid article ID", http.StatusBadRequest)
		return
	}
	
	q := s.Queries
	article, err := q.GetArticle(r.Context(), dbgen.GetArticleParams{ID: id, UserID: user.ID})
	if err != nil {
		http.Error(w, "Article not found", 404)
		return
	}
	
	if article.ContentPath == "" {
		http.Error(w, "No content file available", 404)
		return
	}
	
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	http.ServeFile(w, r, article.ContentPath)
}

func (s *Server) handleRunLog(w http.ResponseWriter, r *http.Request) {
	user, err := s.getOrCreateUser(r)
	if err != nil {
		s.jsonError(w, "Unauthorized", 401)
		return
	}
	
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.jsonError(w, "Invalid run ID", http.StatusBadRequest)
		return
	}
	
	q := s.Queries
	logPath, err := q.GetJobRunLogPath(r.Context(), dbgen.GetJobRunLogPathParams{ID: id, UserID: user.ID})
	if err != nil {
		http.Error(w, "Run not found", 404)
		return
	}
	
	if logPath == "" {
		http.Error(w, "No log available for this run", 404)
		return
	}
	
	// Check if file exists
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		http.Error(w, "Log file not found", 404)
		return
	}
	
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	http.ServeFile(w, r, logPath)
}


func (s *Server) handleDeleteArticles(w http.ResponseWriter, r *http.Request) {
	user, err := s.getOrCreateUser(r)
	if err != nil {
		s.jsonError(w, "Unauthorized", 401)
		return
	}

	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.IDs) == 0 {
		s.jsonError(w, "Invalid request: no articles specified", http.StatusBadRequest)
		return
	}

	deleted, err := s.deleteArticlesWithFiles(r.Context(), user.ID, req.IDs)
	if err != nil {
		slog.Error("failed to delete articles", "error", err)
		s.jsonError(w, "Failed to delete articles", http.StatusInternalServerError)
		return
	}

	s.jsonOK(w, map[string]interface{}{"deleted": deleted})
}

// deleteArticlesWithFiles deletes articles and their content files.
func (s *Server) deleteArticlesWithFiles(ctx context.Context, userID int64, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	placeholders, args := buildINClause(userID, ids)

	// Get content paths before deleting
	query := fmt.Sprintf(
		"SELECT content_path FROM articles WHERE user_id = ? AND id IN (%s) AND content_path != ''",
		placeholders,
	)
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("query content paths: %w", err)
	}

	// Delete files (ignoring errors - files may already be gone)
	for rows.Next() {
		var path string
		if rows.Scan(&path) == nil && path != "" {
			os.Remove(path)
		}
	}
	rows.Close()

	// Delete from database
	deleteQuery := fmt.Sprintf("DELETE FROM articles WHERE user_id = ? AND id IN (%s)", placeholders)
	result, err := s.DB.ExecContext(ctx, deleteQuery, args...)
	if err != nil {
		return 0, fmt.Errorf("delete articles: %w", err)
	}
	return result.RowsAffected()
}

func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// buildINClause builds a placeholder string and args for an IN clause.
// Returns placeholders like "?,?,?" and args as [userID, id1, id2, id3].
func buildINClause(userID int64, ids []int64) (string, []interface{}) {
	placeholders := make([]string, len(ids))
	args := make([]interface{}, 0, len(ids)+1)
	args = append(args, userID)
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	return strings.Join(placeholders, ","), args
}
