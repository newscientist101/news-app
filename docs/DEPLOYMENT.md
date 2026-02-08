# Deployment Guide

This guide explains how to deploy the news-app on a Linux server with systemd.

## Prerequisites

- Linux with systemd
- Go 1.21+ (for building)
- SQLite3
- Access to a Shelley API instance (default: localhost:9999)
- (Optional) exe.dev proxy for authentication

## Quick Start

```bash
# Clone and build
git clone <repo-url> /home/youruser/news-app
cd /home/youruser/news-app
go build -o news-app ./cmd/news-app/

# Run setup script
./deploy/setup-systemd.sh
```

## Services Overview

The app uses several systemd services:

| Service | Type | Purpose |
|---------|------|--------|
| `news-app.service` | Long-running | Main web server on port 8000 |
| `news-job-{id}.service` | One-shot | Individual job execution |
| `news-job-{id}.timer` | Timer | Scheduled job triggers |
| `news-cleanup.service` | One-shot | Cleanup old Shelley conversations |
| `news-cleanup.timer` | Timer | Runs cleanup every 48 hours |
| `news-troubleshoot.service` | One-shot | Auto-diagnose failed runs |
| `news-troubleshoot.timer` | Timer | Runs daily at 07:00 |

## Service Details

### 1. Main Web Server (`news-app.service`)

The primary web application server.

```ini
[Unit]
Description=News Agent Web App
After=network.target

[Service]
Type=simple
User=exedev
WorkingDirectory=/home/exedev/news-app
ExecStart=/home/exedev/news-app/news-app -listen :8000
Restart=on-failure
RestartSec=5
Environment=PATH=/usr/local/bin:/usr/bin:/bin

[Install]
WantedBy=multi-user.target
```

**Management:**
```bash
sudo systemctl start news-app
sudo systemctl stop news-app
sudo systemctl restart news-app
sudo systemctl status news-app
journalctl -u news-app -f  # View logs
```

### 2. Job Runner Services (`news-job-{id}.service`)

These are created dynamically when users create jobs. Each job gets its own service file.

```ini
[Unit]
Description=News Job {id}: {name}
After=network.target

[Service]
Type=oneshot
ExecStart=/home/exedev/news-app/news-app run-job {id}
User=exedev
WorkingDirectory=/home/exedev/news-app
RuntimeMaxSec=1800

[Install]
WantedBy=multi-user.target
```

**Key points:**
- `Type=oneshot` - Runs once and exits
- `RuntimeMaxSec=1800` - 30 minute timeout to prevent stuck jobs
- Created/updated by the web server when jobs are created/modified

**Management:**
```bash
# Run a job immediately
sudo systemctl start news-job-123.service

# Check job status
sudo systemctl status news-job-123.service

# View job logs
journalctl -u news-job-123.service --since "1 hour ago"
```

### 3. Job Timers (`news-job-{id}.timer`)

Schedule recurring jobs. Only created for non-one-time jobs.

```ini
[Unit]
Description=Timer for News Job {id}: {name}

[Timer]
OnCalendar={schedule}
Persistent=true

[Install]
WantedBy=timers.target
```

**Schedule formats:**
| Frequency | OnCalendar Value |
|-----------|------------------|
| Hourly | `*-*-* *:00:00` |
| Every 6 hours | `*-*-* 00/6:00:00` |
| Daily | `*-*-* 06:00:00` |
| Weekly | `Mon *-*-* 06:00:00` |

**Management:**
```bash
# List all job timers
systemctl list-timers 'news-job-*'

# Enable/disable a timer
sudo systemctl enable news-job-123.timer
sudo systemctl disable news-job-123.timer
```

### 4. Cleanup Service (`news-cleanup.service`)

Cleans up old Shelley conversations to prevent database bloat.

```ini
[Unit]
Description=Cleanup old news-app conversations
After=network.target

[Service]
Type=oneshot
ExecStart=/home/exedev/news-app/cleanup-conversations.sh
User=exedev
WorkingDirectory=/home/exedev/news-app

[Install]
WantedBy=multi-user.target
```

**What it does:**
- Finds Shelley conversations older than 48 hours
- Only deletes API-created conversations (not interactive sessions)
- Deletes child conversations (subagents) first
- Logs results to stdout

**Timer:**
```ini
[Timer]
OnBootSec=1h           # Run 1 hour after boot
OnUnitActiveSec=48h    # Then every 48 hours
Persistent=true        # Catch up if missed
```

### 5. Troubleshoot Service (`news-troubleshoot.service`)

Automatically diagnoses failed job runs by creating a Shelley conversation.

```ini
[Unit]
Description=Troubleshoot news-app job runs
After=network.target

[Service]
Type=oneshot
ExecStart=/home/exedev/news-app/troubleshoot-runs.sh
User=exedev
WorkingDirectory=/home/exedev/news-app

[Install]
WantedBy=multi-user.target
```

**What it does:**
- Queries database for failed/problematic runs in last 24 hours
- Creates a Shelley conversation with diagnostic prompt
- Logs the conversation URL for review

**Timer:**
```ini
[Timer]
OnCalendar=*-*-* 07:00:00   # Daily at 7 AM
Persistent=true
RandomizedDelaySec=300       # Random delay up to 5 min
```

## Configuration

### Environment Variables

Set these in the service file's `[Service]` section or in a drop-in:

| Variable | Default | Description |
|----------|---------|-------------|
| `NEWS_APP_DB_PATH` | `/home/exedev/news-app/db.sqlite3` | SQLite database path |
| `NEWS_APP_ARTICLES_DIR` | `/home/exedev/news-app/articles` | Article storage directory |
| `NEWS_APP_LOGS_DIR` | `/home/exedev/news-app/logs/runs` | Job run logs directory |
| `NEWS_APP_SHELLEY_API` | `http://localhost:9999` | Shelley API URL |
| `NEWS_JOB_TIMEOUT` | `25m` | Job execution timeout |
| `NEWS_JOB_POLL_INTERVAL` | `10s` | Shelley API poll interval |
| `NEWS_JOB_START_DELAY` | `60s` | Max random delay before job starts |
| `NEWS_JOB_MAX_PARALLEL` | `5` | Max concurrent article fetches |

**Example drop-in:**
```bash
sudo mkdir -p /etc/systemd/system/news-app.service.d
sudo cat > /etc/systemd/system/news-app.service.d/override.conf << 'EOF'
[Service]
Environment=NEWS_APP_SHELLEY_API=http://other-host:9999
EOF
sudo systemctl daemon-reload
```

### Permissions

The setup script configures sudoers to allow the app user to manage job services:

```
exedev ALL=(ALL) NOPASSWD: /bin/systemctl start news-job-*
exedev ALL=(ALL) NOPASSWD: /bin/systemctl stop news-job-*
exedev ALL=(ALL) NOPASSWD: /bin/systemctl enable news-job-*
exedev ALL=(ALL) NOPASSWD: /bin/systemctl disable news-job-*
exedev ALL=(ALL) NOPASSWD: /bin/systemctl daemon-reload
exedev ALL=(ALL) NOPASSWD: /bin/cp /tmp/systemd-*.tmp /etc/systemd/system/*
```

## Logs

### System Logs
```bash
# Main app logs
journalctl -u news-app -f

# All job logs
journalctl -u 'news-job-*' --since today

# Specific job
journalctl -u news-job-123 --since "2 hours ago"
```

### Application Logs
```
logs/
└── runs/
    ├── run_1_20260208_120000.log
    ├── run_2_20260208_130000.log
    └── ...
```

Each job run creates a log file with:
- Job metadata
- Shelley API interactions
- Article fetch results
- Final status

## Troubleshooting

### Service won't start
```bash
# Check status and logs
sudo systemctl status news-app
journalctl -u news-app -n 50

# Verify binary exists and is executable
ls -la /home/exedev/news-app/news-app

# Test manually
/home/exedev/news-app/news-app -listen :8000
```

### Jobs not running
```bash
# Check if timer is active
systemctl list-timers 'news-job-*'

# Check timer status
systemctl status news-job-123.timer

# Run job manually
sudo systemctl start news-job-123.service
```

### Permission errors
```bash
# Verify sudoers config
sudo cat /etc/sudoers.d/news-app

# Test sudo access
sudo -u exedev sudo systemctl daemon-reload
```

## Uninstalling

```bash
# Stop and disable services
sudo systemctl stop news-app
sudo systemctl disable news-app
sudo systemctl stop news-cleanup.timer news-troubleshoot.timer
sudo systemctl disable news-cleanup.timer news-troubleshoot.timer

# Remove service files
sudo rm /etc/systemd/system/news-app.service
sudo rm /etc/systemd/system/news-cleanup.{service,timer}
sudo rm /etc/systemd/system/news-troubleshoot.{service,timer}
sudo rm /etc/systemd/system/news-job-*
sudo rm /etc/sudoers.d/news-app

# Reload systemd
sudo systemctl daemon-reload
```
