# Agent Instructions

This is the News Agent app for exe.dev.

## Overview

A multi-user web app that retrieves news articles using the Shelley AI agent. See README.md for user-facing docs and ARCHITECTURE.md for system design.

## Key Files

- `srv/server.go` - HTTP router and server setup
- `srv/handlers.go` - Page handlers (dashboard, jobs, job edit, articles, preferences)
- `srv/api.go` - API handlers (CRUD, job control, article content)
- `srv/systemd.go` - Creates/removes systemd timers for job scheduling
- `run-job.sh` - Job runner script (calls Shelley API, fetches articles)
- `db/migrations/` - Database schema
- `db/queries/` - sqlc query definitions

## Common Tasks

### Adding a new page

1. Add handler in `srv/handlers.go`
2. Add route in `srv/server.go`
3. Create template in `srv/templates/`

### Adding a new API endpoint

1. Add handler in `srv/api.go`
2. Add route in `srv/server.go`

### Modifying database schema

1. Create new migration file in `db/migrations/`
2. Update queries in `db/queries/`
3. Run `sqlc generate` in `db/`

### Modifying job behavior

Edit `run-job.sh`. Key sections:
- Prompt building (lines 34-75)
- Shelley API interaction (lines 80-145)
- JSON extraction (lines 147-165)
- Article fetching (lines 167-265)

## Build & Deploy

```bash
make build
sudo systemctl restart news-app
```

## Testing

```bash
# Run a job manually
./run-job.sh {job_id}

# Check logs
journalctl -u news-app -f
journalctl -u news-job-{id} -f
```
