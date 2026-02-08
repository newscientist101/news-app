# News Agent

A multi-user web app that retrieves news articles using the Shelley AI agent. Users can create scheduled jobs to search for news on specific topics, and the app fetches full article content for offline reading.

## Quick Start

```bash
# Build
make build

# Run locally
./news-app -listen :8000

# Or restart the systemd service
sudo systemctl restart news-app
```

Access at: https://anchor-asteroid.exe.xyz:8000/

## Features

- **Job Scheduling**: Create one-time or recurring jobs (hourly, 6 hours, daily, weekly)
- **Job Editing**: Modify job settings, prompts, and schedules at any time
- **Search Filters**: Filter by keywords, sources, geographic region
- **Full Content Fetching**: Automatically fetches and stores complete article text
- **User Preferences**: Custom system prompts, Discord webhook notifications
- **Multi-user**: Each user has their own jobs and articles

## Architecture

See [ARCHITECTURE.md](docs/ARCHITECTURE.md) for detailed system design.

## Building

See [BUILD.md](docs/BUILD.md) for build instructions and development setup.

## Agents

See [AGENTS.MD](docs/AGENTS.md) for agent instructions.

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
├── db/
│   ├── db.go         # Database setup
│   ├── migrations/   # SQL migrations
│   ├── queries/      # sqlc queries
│   └── dbgen/        # Generated code
├── articles/         # Stored article content
│   └── job_{id}/     # Per-job folders
├── run-job.sh        # Job runner script
└── db.sqlite3        # SQLite database
```

## License

MIT
