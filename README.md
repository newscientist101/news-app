# News Agent

A multi-user web app that retrieves news articles using the Shelley AI agent. Users can create scheduled jobs to search for news on specific topics, and the app fetches full article content for offline reading.

**Designed for [exe.dev](https://exe.dev) VMs** - uses exe.dev authentication headers and the Shelley AI agent.

> ⚠️ **Token Usage Warning**: Each job run uses the Shelley AI agent to search for and retrieve news articles, which consumes API tokens. Recurring jobs (hourly, daily, etc.) will accumulate significant token usage over time. Monitor your usage and adjust job frequency accordingly.

## Quick Start

```bash
# Build
go build -o news-app ./cmd/news-app/

# Install systemd services
sudo ./deploy/setup-systemd.sh

# Or run locally without systemd
./news-app -listen :8000
```

Access at your exe.dev VM URL (e.g., `https://your-vm.exe.xyz:8000/`)

## Features

- **exe.dev Integration**: Authentication via exe.dev proxy headers, Shelley AI for news retrieval
- **Job Scheduling**: Create one-time or recurring jobs (hourly, 6 hours, daily, weekly)
- **Job Editing**: Modify job settings, prompts, and schedules at any time
- **Search Filters**: Filter by keywords, sources, geographic region
- **Full Content Fetching**: Automatically fetches and stores complete article text
- **User Preferences**: Custom system prompts, Discord webhook notifications
- **Multi-user**: Each user has their own jobs and articles (identified by exe.dev user ID)

## Documentation

- [ARCHITECTURE.md](docs/ARCHITECTURE.md) - System design and data flow
- [DEPLOYMENT.md](docs/DEPLOYMENT.md) - Systemd services and deployment guide
- [BUILD.md](docs/BUILD.md) - Build instructions and development setup
- [AGENTS.md](docs/AGENTS.md) - Agent instructions for AI assistants

## Code Layout

```
news-app/
├── cmd/news-app/        # Main entrypoint
├── internal/
│   ├── web/             # HTTP server
│   │   ├── server.go    # Router and server setup
│   │   ├── handlers.go  # Page handlers
│   │   ├── api.go       # API handlers
│   │   ├── systemd.go   # Job scheduling
│   │   ├── templates/   # HTML templates
│   │   └── static/      # CSS, JS
│   ├── jobrunner/       # Job execution
│   │   ├── runner.go    # Main job logic
│   │   ├── shelley.go   # Shelley API client
│   │   ├── content.go   # Article content extraction
│   │   ├── cleanup.go   # Conversation cleanup
│   │   ├── troubleshoot.go # Auto-diagnosis
│   │   └── discord.go   # Discord notifications
│   ├── db/
│   │   ├── db.go        # Database setup
│   │   ├── migrations/  # SQL migrations
│   │   ├── queries/     # sqlc queries
│   │   └── dbgen/       # Generated code
│   └── util/            # Shared utilities
├── deploy/              # Deployment files
│   ├── setup-systemd.sh # Service installation
│   └── *.service/*.timer
├── docs/                # Documentation
├── articles/            # Stored article content
└── logs/
    ├── runs/            # Job run logs
    └── troubleshoot/    # Troubleshooting reports
```

## Systemd Services

The app uses several systemd services for scheduling and maintenance:

| Service | Purpose |
|---------|---------|
| `news-app.service` | Main web server |
| `news-job-{id}.service` | Individual job execution |
| `news-job-{id}.timer` | Job scheduling |
| `news-cleanup.timer` | Clean old Shelley conversations (every 48h) |
| `news-troubleshoot.timer` | Auto-diagnose failed runs (daily) |

See [DEPLOYMENT.md](docs/DEPLOYMENT.md) for details.

## Requirements

- [exe.dev](https://exe.dev) VM (provides authentication and HTTPS proxy)
- Go 1.21+
- SQLite3
- Shelley AI agent (runs on exe.dev VMs at localhost:9999)
- Linux with systemd (for scheduled jobs)

## Authentication

This app relies on exe.dev's authentication proxy. The proxy adds `X-Exedev-Userid` and `X-Exedev-Email` headers to incoming requests, which the app uses to identify users. No additional auth configuration is needed.

## License

MIT
