# Configuration Reference

This document lists all configuration options for the news-app.

## Environment Variables

### Application Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `NEWS_APP_DB_PATH` | `/home/exedev/news-app/db.sqlite3` | Path to SQLite database |
| `NEWS_APP_ARTICLES_DIR` | `/home/exedev/news-app/articles` | Directory for article text files |
| `NEWS_APP_LOGS_DIR` | `/home/exedev/news-app/logs/runs` | Directory for job run logs |
| `NEWS_APP_SHELLEY_API` | `http://localhost:9999` | Shelley API base URL |

### Systemd Integration

| Variable | Default | Description |
|----------|---------|-------------|
| `NEWS_APP_SYSTEMD_DIR` | `/etc/systemd/system` | Directory for systemd unit files |
| `NEWS_APP_JOB_RUNNER` | `/home/exedev/news-app/news-app` | Path to job runner binary |
| `NEWS_APP_JOB_RUNNER_ARGS` | `run-job` | Arguments/subcommand for job runner |
| `NEWS_APP_WORKING_DIR` | `/home/exedev/news-app` | Working directory for job services |
| `NEWS_APP_RUN_USER` | `exedev` | User to run job services as |

### Job Runner Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `NEWS_JOB_TIMEOUT` | `25m` | Maximum time to wait for Shelley response |
| `NEWS_JOB_POLL_INTERVAL` | `10s` | Interval between Shelley API polls |
| `NEWS_JOB_START_DELAY` | `60s` | Maximum random delay before job starts |
| `NEWS_JOB_MAX_PARALLEL` | `5` | Maximum concurrent article fetches |

## Command Line Flags

### Server (`news-app`)

```bash
./news-app [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `-listen` | `:8000` | Address to listen on |

### Cleanup (`news-app cleanup`)

```bash
./news-app cleanup [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--max-age` | `48` | Max age in hours for conversations to keep |
| `--dry-run` | `false` | Show what would be deleted without deleting |

### Troubleshoot (`news-app troubleshoot`)

```bash
./news-app troubleshoot [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--lookback` | `24` | Hours to look back for problems |
| `--dry-run` | `false` | Show problems without creating conversation |

### Run Job (`news-app run-job`)

```bash
./news-app run-job <job_id>
```

No additional flags. Job ID is required.

## Systemd Service Configuration

### Overriding Defaults

Create a drop-in file to override service settings:

```bash
sudo mkdir -p /etc/systemd/system/news-app.service.d
sudo cat > /etc/systemd/system/news-app.service.d/override.conf << 'EOF'
[Service]
Environment=NEWS_APP_SHELLEY_API=http://other-host:9999
Environment=NEWS_APP_DB_PATH=/var/lib/news-app/db.sqlite3
EOF
sudo systemctl daemon-reload
sudo systemctl restart news-app
```

### Job Service Timeouts

Job services have a 30-minute timeout (`RuntimeMaxSec=1800`) to prevent stuck jobs. This can be overridden per-job:

```bash
sudo mkdir -p /etc/systemd/system/news-job-123.service.d
echo -e '[Service]\nRuntimeMaxSec=3600' | sudo tee /etc/systemd/system/news-job-123.service.d/timeout.conf
sudo systemctl daemon-reload
```

## Database Configuration

The SQLite database is configured with the following pragmas (set in `internal/db/db.go`):

| Pragma | Value | Purpose |
|--------|-------|--------|
| `journal_mode` | `WAL` | Write-ahead logging for better concurrency |
| `busy_timeout` | `5000` | Wait 5 seconds on lock contention |
| `synchronous` | `NORMAL` | Balance between safety and performance |
| `foreign_keys` | `ON` | Enforce foreign key constraints |

## Job Frequencies

Available job frequencies and their systemd OnCalendar equivalents:

| Frequency | Value | OnCalendar | Description |
|-----------|-------|------------|-------------|
| Hourly | `hourly` | `*-*-* *:00:00` | Every hour at :00 |
| Every 6 hours | `6hours` | `*-*-* 00/6:00:00` | At 00:00, 06:00, 12:00, 18:00 |
| Daily | `daily` | `*-*-* 06:00:00` | Every day at 06:00 |
| Weekly | `weekly` | `Mon *-*-* 06:00:00` | Every Monday at 06:00 |

## File Paths

### Default Directory Structure

```
/home/exedev/news-app/
├── news-app              # Binary
├── db.sqlite3            # Database
├── articles/             # Article content
│   └── job_{id}/
│       └── article_{id}_{timestamp}.txt
└── logs/
    ├── runs/             # Job run logs
    │   └── run_{id}_{timestamp}.log
    └── troubleshoot/     # Troubleshooting reports
        └── report-{date}.md
```

### Systemd Unit Files

```
/etc/systemd/system/
├── news-app.service
├── news-cleanup.service
├── news-cleanup.timer
├── news-troubleshoot.service
├── news-troubleshoot.timer
├── news-job-{id}.service    # Created dynamically
└── news-job-{id}.timer      # Created dynamically
```

## Shelley API Configuration

The Shelley API URL can be configured via `NEWS_APP_SHELLEY_API`. The default assumes Shelley is running locally on the exe.dev VM.

The job runner uses the following Shelley settings:
- Model: `claude-sonnet-4.5`
- User ID format: `news-job-{job_id}`
- Cleanup user ID: `cleanup`
- Troubleshoot user ID: `news-app-troubleshoot`
