package srv

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"log/slog"
	"time"

	"srv.exe.dev/db/dbgen"
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
	
	nextRun := calculateNextRun(req.Frequency, req.IsOneTime)
	
	q := dbgen.New(s.DB)
	job, err := q.CreateJob(r.Context(), dbgen.CreateJobParams{
		UserID:    user.ID,
		Name:      req.Name,
		Prompt:    req.Prompt,
		Keywords:  req.Keywords,
		Sources:   req.Sources,
		Region:    req.Region,
		Frequency: req.Frequency,
		IsOneTime: boolToInt(req.IsOneTime),
		NextRunAt: &nextRun,
	})
	if err != nil {
		s.jsonError(w, "Failed to create job", http.StatusInternalServerError)
		return
	}
	
	// Create systemd timer
	if err := createSystemdTimer(job); err != nil {
		// Log but don't fail - job is created
		fmt.Fprintf(os.Stderr, "Failed to create systemd timer: %v\n", err)
	}
	
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
	
	q := dbgen.New(s.DB)
	err = q.UpdateJob(r.Context(), dbgen.UpdateJobParams{
		Name:      req.Name,
		Prompt:    req.Prompt,
		Keywords:  req.Keywords,
		Sources:   req.Sources,
		Region:    req.Region,
		Frequency: req.Frequency,
		IsActive:  boolToInt(req.IsActive),
		ID:        id,
		UserID:    user.ID,
	})
	if err != nil {
		s.jsonError(w, "Failed to update job", http.StatusInternalServerError)
		return
	}
	
	// Update systemd timer
	job, _ := q.GetJob(r.Context(), dbgen.GetJobParams{ID: id, UserID: user.ID})
	updateSystemdTimer(job)
	
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
	
	q := dbgen.New(s.DB)
	err = q.DeleteJob(r.Context(), dbgen.DeleteJobParams{ID: id, UserID: user.ID})
	if err != nil {
		s.jsonError(w, "Failed to delete job", http.StatusInternalServerError)
		return
	}
	
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
	
	q := dbgen.New(s.DB)
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
		// Try running directly if systemd fails
		go runJobDirectly(s.DB, job.ID)
	}
	
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
	
	q := dbgen.New(s.DB)
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
	
	q := dbgen.New(s.DB)
	
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
	
	q := dbgen.New(s.DB)
	
	// Ensure preferences exist
	_, err = q.GetPreferences(r.Context(), user.ID)
	if err == sql.ErrNoRows {
		q.CreatePreferences(r.Context(), user.ID)
	}
	
	err = q.UpdatePreferences(r.Context(), dbgen.UpdatePreferencesParams{
		SystemPrompt:   req.SystemPrompt,
		DiscordWebhook: req.DiscordWebhook,
		NotifySuccess:  boolToInt(req.NotifySuccess),
		NotifyFailure:  boolToInt(req.NotifyFailure),
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
	
	q := dbgen.New(s.DB)
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
	
	q := dbgen.New(s.DB)
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

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func calculateNextRun(frequency string, isOneTime bool) time.Time {
	now := time.Now()
	if isOneTime {
		return now.Add(10 * time.Second) // Run almost immediately
	}
	switch frequency {
	case "hourly":
		return now.Add(1 * time.Hour)
	case "6hours":
		return now.Add(6 * time.Hour)
	case "daily":
		return now.Add(24 * time.Hour)
	case "weekly":
		return now.Add(7 * 24 * time.Hour)
	default:
		return now.Add(24 * time.Hour)
	}
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

	q := dbgen.New(s.DB)
	deleted := 0
	var errors []string

	for _, id := range req.IDs {
		// Get article to find content_path
		article, err := q.GetArticle(r.Context(), dbgen.GetArticleParams{
			ID:     id,
			UserID: user.ID,
		})
		if err != nil {
			errors = append(errors, fmt.Sprintf("Article %d not found", id))
			continue
		}

		// Delete the content file
		if article.ContentPath != "" {
			if err := os.Remove(article.ContentPath); err != nil && !os.IsNotExist(err) {
				// Log but don't fail - file might already be gone
				slog.Warn("Failed to delete article file", "path", article.ContentPath, "error", err)
			}
		}

		// Delete from database
		if err := q.DeleteArticle(r.Context(), dbgen.DeleteArticleParams{
			ID:     id,
			UserID: user.ID,
		}); err != nil {
			errors = append(errors, fmt.Sprintf("Failed to delete article %d", id))
			continue
		}
		deleted++
	}

	s.jsonOK(w, map[string]interface{}{
		"deleted": deleted,
		"errors":  errors,
	})
}
