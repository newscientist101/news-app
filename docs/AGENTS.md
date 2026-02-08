# Agent Instructions

This is the News Agent app for exe.dev.

## Overview

A multi-user web app that retrieves news articles using the Shelley AI agent. See README.md for user-facing docs and ARCHITECTURE.md for system design.

## Key Files

- `internal/web/server.go` - HTTP router and server setup
- `internal/web/handlers.go` - Page handlers (dashboard, jobs, job edit, articles, preferences)
- `internal/web/api.go` - API handlers (CRUD, job control, article content)
- `internal/web/systemd.go` - Creates/removes systemd timers for job scheduling
- `internal/jobrunner/runner.go` - Job execution logic (calls Shelley API, fetches articles)
- `internal/jobrunner/shelley.go` - Shelley API client
- `internal/jobrunner/cleanup.go` - Conversation cleanup (`news-app cleanup` subcommand)
- `internal/jobrunner/troubleshoot.go` - Auto-diagnosis (`news-app troubleshoot` subcommand)
- `internal/db/migrations/` - Database schema
- `internal/db/queries/` - sqlc query definitions

## Common Tasks

### Adding a new page

1. Add handler in `internal/web/handlers.go`
2. Add route in `internal/web/server.go`
3. Create template in `internal/web/templates/`

### Adding a new API endpoint

1. Add handler in `internal/web/api.go`
2. Add route in `internal/web/server.go`

### Modifying database schema

1. Create new migration file in `internal/db/migrations/`
2. Update queries in `internal/db/queries/`
3. Run `sqlc generate` in `internal/db/`

### Modifying job behavior

Edit `internal/jobrunner/runner.go`. Key methods:
- `Run()` - Main entry point
- `buildPrompt()` - Constructs the Shelley prompt
- `runConversation()` - Shelley API interaction
- `extractArticles()` - JSON extraction from response
- `fetchArticleContent()` - HTTP fetch + content extraction

## Build & Deploy

```bash
make build
sudo systemctl restart news-app
```

## Subcommands

The binary supports multiple subcommands:

```bash
# Run web server (default)
./news-app -listen :8000

# Run a job manually
./news-app run-job {job_id}

# Cleanup old conversations
./news-app cleanup [--max-age 48] [--dry-run]

# Diagnose failed runs
./news-app troubleshoot [--lookback 24] [--dry-run]
```

## Testing

```bash
# Run a job manually
./news-app run-job {job_id}

# Check logs
journalctl -u news-app -f
journalctl -u news-job-{id} -f

# Run tests
go test ./...
```
