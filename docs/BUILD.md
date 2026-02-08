# Build Notes

## Prerequisites

- Go 1.21+
- SQLite3
- Python 3 (for article content extraction)
- sqlc (for regenerating database code)

## Building

```bash
# Build the binary
make build

# Or manually:
go build -o news-app ./cmd/srv
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
# Install service file (already done)
sudo cp news-app.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable news-app
sudo systemctl start news-app

# Check status
systemctl status news-app

# View logs
journalctl -u news-app -f

# Restart after changes
make build && sudo systemctl restart news-app
```

## Database

### Migrations

Migrations are in `db/migrations/` and run automatically on startup.

```bash
# View current schema
sqlite3 db.sqlite3 ".schema"

# Query data
sqlite3 db.sqlite3 "SELECT * FROM jobs;"
```

### Regenerating sqlc Code

If you modify queries in `db/queries/`:

```bash
cd db
sqlc generate
```

## Job Runner

The job runner script can be tested manually:

```bash
# Run a specific job
./run-job.sh {job_id}

# Check job logs
journalctl -u news-job-{id} -f
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
```

## Troubleshooting

### Job times out

The job runner waits up to 5 minutes for the agent. If timing out:
- Check Shelley API is running: `curl http://localhost:9999/health`
- Check job logs: `journalctl -u news-job-{id}`

### No articles saved

If job completes but no articles:
- Check logs for "No JSON array found"
- The agent may not have returned valid JSON
- View raw response in job logs

### Article content empty

If articles have no full content:
- The URL may be paywalled or block bots
- Check the `.txt` file for error messages
- The Python extractor only gets `<p>` tag content

### Auth issues

If getting 401 errors or redirects:
- Ensure accessing via exe.dev proxy (not localhost directly)
- Check `X-ExeDev-UserID` header is present
- For local dev, use mitmdump proxy
