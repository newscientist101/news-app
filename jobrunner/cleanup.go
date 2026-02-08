package jobrunner

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "modernc.org/sqlite"
)

// CleanupConfig holds configuration for conversation cleanup.
type CleanupConfig struct {
	ShelleyDBPath string
	ShelleyAPI    string
	MaxAgeHours   int
	DryRun        bool
}

// DefaultCleanupConfig returns default cleanup configuration.
func DefaultCleanupConfig() CleanupConfig {
	return CleanupConfig{
		ShelleyDBPath: "/home/exedev/.config/shelley/shelley.db",
		ShelleyAPI:    "http://localhost:9999",
		MaxAgeHours:   48,
		DryRun:        false,
	}
}

// CleanupResult holds the results of a cleanup run.
type CleanupResult struct {
	Found   int
	Deleted int
	Failed  int
}

// Cleanup removes old conversations from the Shelley API.
func Cleanup(ctx context.Context, cfg CleanupConfig) (*CleanupResult, error) {
	logger := slog.Default()
	result := &CleanupResult{}

	// Open Shelley database (read-only)
	db, err := sql.Open("sqlite", cfg.ShelleyDBPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open shelley db: %w", err)
	}
	defer db.Close()

	// Find old parent conversations (cwd IS NULL = API-created, not interactive)
	cutoff := time.Now().Add(-time.Duration(cfg.MaxAgeHours) * time.Hour)
	rows, err := db.QueryContext(ctx, `
		SELECT conversation_id 
		FROM conversations 
		WHERE cwd IS NULL 
		AND parent_conversation_id IS NULL
		AND created_at < ?
		ORDER BY created_at ASC
	`, cutoff.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return nil, fmt.Errorf("query old conversations: %w", err)
	}
	defer rows.Close()

	var parentIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan conversation id: %w", err)
		}
		parentIDs = append(parentIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conversations: %w", err)
	}

	result.Found = len(parentIDs)
	logger.Info("found old conversations", "count", result.Found, "max_age_hours", cfg.MaxAgeHours)

	if cfg.DryRun {
		logger.Info("dry run - not deleting")
		return result, nil
	}

	// Create Shelley client
	client := NewShelleyClient(cfg.ShelleyAPI)

	// Delete each parent and its children
	for _, parentID := range parentIDs {
		deleted, failed := deleteConversationTree(ctx, db, client, parentID, logger)
		result.Deleted += deleted
		result.Failed += failed
	}

	logger.Info("cleanup complete", 
		"found", result.Found, 
		"deleted", result.Deleted, 
		"failed", result.Failed)

	return result, nil
}

// deleteConversationTree deletes a conversation and all its children.
func deleteConversationTree(ctx context.Context, db *sql.DB, client *ShelleyClient, convID string, logger *slog.Logger) (deleted, failed int) {
	// Find children first
	rows, err := db.QueryContext(ctx, `
		SELECT conversation_id FROM conversations 
		WHERE parent_conversation_id = ?
	`, convID)
	if err != nil {
		logger.Warn("query children", "conversation_id", convID, "error", err)
	} else {
		var childIDs []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err == nil {
				childIDs = append(childIDs, id)
			}
		}
		rows.Close()

		// Recursively delete children
		for _, childID := range childIDs {
			logger.Debug("deleting child conversation", "child_id", childID, "parent_id", convID)
			d, f := deleteConversationTree(ctx, db, client, childID, logger)
			deleted += d
			failed += f
		}
	}

	// Delete this conversation
	logger.Info("deleting conversation", "conversation_id", convID)
	if err := client.DeleteConversationAsCleanup(ctx, convID); err != nil {
		logger.Warn("delete conversation", "conversation_id", convID, "error", err)
		failed++
	} else {
		deleted++
	}

	return deleted, failed
}
