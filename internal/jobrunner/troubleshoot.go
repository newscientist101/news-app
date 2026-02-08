package jobrunner

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TroubleshootConfig holds configuration for troubleshooting.
type TroubleshootConfig struct {
	DBPath     string
	ShelleyAPI string
	Lookback   time.Duration
	LogDir     string
	DryRun     bool
}

// DefaultTroubleshootConfig returns default troubleshoot configuration.
func DefaultTroubleshootConfig() TroubleshootConfig {
	return TroubleshootConfig{
		DBPath:     "db.sqlite3",
		ShelleyAPI: "http://localhost:9999",
		Lookback:   24 * time.Hour,
		LogDir:     "logs/troubleshoot",
		DryRun:     false,
	}
}

// ProblemRun represents a problematic job run.
type ProblemRun struct {
	RunID        int64
	JobID        int64
	JobName      string
	Status       string
	ErrorMessage string
	StartedAt    string
	CompletedAt  string
	ArticleCount int
}

// TroubleshootResult holds the results of a troubleshoot run.
type TroubleshootResult struct {
	ProblemsFound  int
	ConversationID string
}

// Troubleshoot identifies problematic job runs and creates a Shelley conversation.
func Troubleshoot(ctx context.Context, cfg TroubleshootConfig) (*TroubleshootResult, error) {
	logger := slog.Default()
	result := &TroubleshootResult{}

	// Ensure log directory exists
	if err := os.MkdirAll(cfg.LogDir, 0755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	// Get absolute path for the prompt
	absLogDir, err := filepath.Abs(cfg.LogDir)
	if err != nil {
		absLogDir = cfg.LogDir
	}

	// Open database
	db, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	// Find problematic runs
	problems, err := findProblemRuns(ctx, db, cfg.Lookback)
	if err != nil {
		return nil, fmt.Errorf("find problem runs: %w", err)
	}

	result.ProblemsFound = len(problems)

	if len(problems) == 0 {
		logger.Info("no problematic runs found", "lookback", cfg.Lookback)
		return result, nil
	}

	logger.Info("found problematic runs", "count", len(problems))
	for _, p := range problems {
		logger.Info("problem run",
			"run_id", p.RunID,
			"job_id", p.JobID,
			"job_name", p.JobName,
			"status", p.Status,
			"articles", p.ArticleCount,
			"error", p.ErrorMessage)
	}

	if cfg.DryRun {
		logger.Info("dry run - not creating conversation")
		return result, nil
	}

	// Build prompt and create conversation
	prompt := buildTroubleshootPrompt(problems, absLogDir)
	client := NewShelleyClient(cfg.ShelleyAPI)

	convID, err := client.CreateConversationAs(ctx, "news-app-troubleshoot", prompt)
	if err != nil {
		return nil, fmt.Errorf("create conversation: %w", err)
	}

	result.ConversationID = convID
	logger.Info("created troubleshooting conversation", "conversation_id", convID)

	return result, nil
}

func findProblemRuns(ctx context.Context, db *sql.DB, lookback time.Duration) ([]ProblemRun, error) {
	cutoff := time.Now().Add(-lookback).UTC().Format("2006-01-02 15:04:05")

	query := `
		SELECT 
			jr.id,
			jr.job_id,
			j.name,
			jr.status,
			COALESCE(jr.error_message, ''),
			jr.started_at,
			COALESCE(jr.completed_at, ''),
			(SELECT COUNT(*) FROM articles a 
			 WHERE a.job_id = jr.job_id 
			 AND a.retrieved_at >= jr.started_at 
			 AND (jr.completed_at IS NULL OR a.retrieved_at <= jr.completed_at)) as article_count
		FROM job_runs jr
		JOIN jobs j ON jr.job_id = j.id
		WHERE jr.started_at >= ?
		  AND j.is_active = 1
		  AND (
			jr.status = 'failed'
			OR (jr.status = 'cancelled' AND jr.error_message NOT LIKE '%new run started%' AND jr.error_message NOT LIKE '%Debug%')
			OR (jr.status = 'completed' AND (SELECT COUNT(*) FROM articles a 
				WHERE a.job_id = jr.job_id 
				AND a.retrieved_at >= jr.started_at 
				AND (jr.completed_at IS NULL OR a.retrieved_at <= jr.completed_at)) = 0)
		  )
		  AND jr.status != 'completed_no_new'
		ORDER BY jr.started_at DESC
	`

	rows, err := db.QueryContext(ctx, query, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var problems []ProblemRun
	for rows.Next() {
		var p ProblemRun
		if err := rows.Scan(&p.RunID, &p.JobID, &p.JobName, &p.Status,
			&p.ErrorMessage, &p.StartedAt, &p.CompletedAt, &p.ArticleCount); err != nil {
			return nil, err
		}
		problems = append(problems, p)
	}

	return problems, rows.Err()
}

func buildTroubleshootPrompt(problems []ProblemRun, logDir string) string {
	var sb strings.Builder
	timestamp := time.Now().Format("2006-01-02")

	sb.WriteString("I need you to troubleshoot issues with the news-app job runs. ")
	sb.WriteString("Here are the problematic runs from the last 24 hours:\n\n")

	for _, p := range problems {
		fmt.Fprintf(&sb, "- Run ID %d (Job '%s', ID %d):\n", p.RunID, p.JobName, p.JobID)
		fmt.Fprintf(&sb, "  Status: %s\n", p.Status)
		fmt.Fprintf(&sb, "  Started: %s\n", p.StartedAt)
		if p.CompletedAt != "" {
			fmt.Fprintf(&sb, "  Completed: %s\n", p.CompletedAt)
		}
		fmt.Fprintf(&sb, "  Articles retrieved: %d\n", p.ArticleCount)
		if p.ErrorMessage != "" {
			fmt.Fprintf(&sb, "  Error: %s\n", p.ErrorMessage)
		}
		sb.WriteString("\n")
	}

	fmt.Fprintf(&sb, `
Please investigate:
1. Check the systemd journal logs for these job runs: journalctl -u news-job-{job_id}.service --since '{started_at}'
2. Check if the conversations were created and completed properly
3. Look for patterns in failures (timeouts, JSON parsing, network issues)
4. Suggest fixes if you find systematic issues

Key files:
- /home/exedev/news-app/internal/jobrunner/ - The Go job runner implementation
- /home/exedev/news-app/db.sqlite3 - The database
- Shelley API at http://localhost:9999

When you've completed your investigation, write a troubleshooting report to:
  %s/report-%s.md

The report should include:
- Summary of issues found
- Root cause analysis
- Recommended fixes
- Any actions you took

Start by examining the logs for the most recent failed run.`, logDir, timestamp)

	return sb.String()
}
