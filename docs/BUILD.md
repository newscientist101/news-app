# Build Notes

## Prerequisites

- Go 1.21+
- SQLite3
- sqlc (for regenerating database code)
- exe.dev VM (for authentication and Shelley API)

## Building

```bash
# Build the binary
make build

# Or manually:
go build -o news-app ./cmd/news-app
chmod +x news-app
```

## Running Locally

```bash
# Start the server
./news-app -listen :8000

# For local dev with fake auth, use mitmdump proxy:
mitmdump -p 3000 --mode reverse:http://localhost:8000 \
  --modify-headers '/~q/X-ExeDev-UserID/1' \
  --modify-headers '/~q/X-ExeDev-Email/testuser@example.com'

# Then access at http://localhost:3000
```

## Running as systemd Service

```bash
# Run setup script
sudo ./deploy/setup-systemd.sh

# Check status
systemctl status news-app

# View logs
journalctl -u news-app -f

# Restart after changes
make build && sudo systemctl restart news-app
```

## Database

### Migrations

Migrations are in `internal/db/migrations/` and run automatically on startup.

```bash
# View current schema
sqlite3 db.sqlite3 ".schema"

# Query data
sqlite3 db.sqlite3 "SELECT * FROM jobs;"
```

### Regenerating sqlc Code

If you modify queries in `internal/db/queries/`:

```bash
cd internal/db
sqlc generate
```

## Subcommands

The binary supports multiple subcommands:

```bash
# Run web server (default)
./news-app -listen :8000

# Run a specific job
./news-app run-job {job_id}

# Cleanup old Shelley conversations
./news-app cleanup [--max-age 48] [--dry-run]

# Diagnose failed job runs
./news-app troubleshoot [--lookback 24] [--dry-run]

# Show help
./news-app help
```

## Systemd Timers

Jobs are scheduled via systemd timers:

```bash
# List all news job timers
systemctl list-timers 'news-job-*'

# Check specific timer
systemctl status news-job-6.timer

# Manually trigger a job
sudo systemctl start news-job-6.service
```

## Cleaning Up

```bash
# Remove old article files
rm -rf articles/job_*/

# Remove orphaned timers
sudo systemctl disable news-job-{id}.timer
sudo rm /etc/systemd/system/news-job-{id}.*
sudo systemctl daemon-reload

# Clean old Shelley conversations
./news-app cleanup
```

## Running Tests

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run specific package tests
go test ./internal/jobrunner/
```

## Troubleshooting

### Job times out

The job runner waits up to 25 minutes for the agent. If timing out:
- Check Shelley API is running: `curl http://localhost:9999/health`
- Check job logs: `journalctl -u news-job-{id}`

### No articles saved

If job completes but no articles:
- Check logs for "No JSON array found"
- The agent may not have returned valid JSON
- Run `./news-app troubleshoot --dry-run` to see recent failures

### Article content empty

If articles have no full content:
- The URL may be paywalled or block bots
- Check the `.txt` file for error messages
- The go-readability extractor may not handle the site's HTML

### Auth issues

If getting 401 errors or redirects:
- Ensure accessing via exe.dev proxy (not localhost directly)
- Check `X-ExeDev-UserID` header is present
- For local dev, use mitmdump proxy
