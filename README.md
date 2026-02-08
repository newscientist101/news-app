# News Agent

A multi-user web app that retrieves news articles using the Shelley AI agent. Users can create scheduled jobs to search for news on specific topics, and the app fetches full article content for offline reading.

## Quick Start

```bash
# Build
go build -o news-app ./cmd/srv/

# Install systemd services
sudo ./scripts/setup-systemd.sh

# Or run locally without systemd
./news-app -listen :8000
```

Access at: http://localhost:8000/

## Features

- **Job Scheduling**: Create one-time or recurring jobs (hourly, 6 hours, daily, weekly)
- **Job Editing**: Modify job settings, prompts, and schedules at any time
- **Search Filters**: Filter by keywords, sources, geographic region
- **Full Content Fetching**: Automatically fetches and stores complete article text
- **User Preferences**: Custom system prompts, Discord webhook notifications
- **Multi-user**: Each user has their own jobs and articles

## Documentation

- [ARCHITECTURE.md](docs/ARCHITECTURE.md) - System design and data flow
- [DEPLOYMENT.md](docs/DEPLOYMENT.md) - Systemd services and deployment guide
- [BUILD.md](docs/BUILD.md) - Build instructions and development setup
- [AGENTS.md](docs/AGENTS.md) - Agent instructions for AI assistants

## Code Layout

```
news-app/
├── cmd/srv/          # Main entrypoint
├── srv/              # HTTP server
│   ├── server.go     # Router and server setup
│   ├── handlers.go   # Page handlers
│   ├── api.go        # API handlers
│   ├── systemd.go    # Job scheduling
│   ├── templates/    # HTML templates
│   └── static/       # CSS, JS
├── jobrunner/        # Job execution (Go implementation)
│   ├── runner.go     # Main job logic
│   ├── shelley.go    # Shelley API client
│   ├── content.go    # Article content extraction
│   └── discord.go    # Discord notifications
├── db/
│   ├── db.go         # Database setup
│   ├── migrations/   # SQL migrations
│   ├── queries/      # sqlc queries
│   └── dbgen/        # Generated code
├── scripts/
│   └── setup-systemd.sh  # Service installation script
├── articles/         # Stored article content
└── logs/             # Job run logs
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

- Go 1.21+
- SQLite3
- Shelley API instance (default: localhost:9999)
- Linux with systemd (for scheduled jobs)

## License

MIT
