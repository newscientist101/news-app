# Changelog

All notable changes to this project are documented in this file.

## [Unreleased]

### Added
- Auto-troubleshooting service that diagnoses failed job runs daily
- Troubleshooting reports saved to `logs/troubleshoot/`
- Token usage warning in README
- Storage warning about Shelley database cleanup requirement
- Troubleshooting section for storage issues

### Changed
- Reorganized directory structure following Go conventions:
  - `cmd/srv/` → `cmd/news-app/`
  - `srv/` → `internal/web/`
  - `jobrunner/` → `internal/jobrunner/`
  - `db/` → `internal/db/`
  - Service files moved to `deploy/`
- Renamed `srv.service` to `news-app.service`
- Updated all documentation for new structure

### Removed
- `run-job.sh` - Replaced by Go `jobrunner` package
- `cleanup-conversations.sh` - Replaced by `news-app cleanup` subcommand
- `troubleshoot-runs.sh` - Replaced by `news-app troubleshoot` subcommand
- `scripts/backfill_duplicates.sh` - One-time migration script
- `.venv/` Python virtual environment - No longer needed
- `welcome.html` and `script.js` - Unused template files
- `IMPROVEMENTS.md` - Issues have been addressed
- `RUN_JOB_GO_DESIGN.md` - Implementation complete

## [2026-02-08] - Go Jobrunner Implementation

### Added
- Full Go implementation of job runner (`internal/jobrunner/`)
- `news-app run-job <id>` subcommand
- `news-app cleanup` subcommand with `--max-age` and `--dry-run` flags
- `news-app troubleshoot` subcommand with `--lookback` and `--dry-run` flags
- Shelley API client in Go (`internal/jobrunner/shelley.go`)
- Article content extraction using go-readability
- JSON extraction with malformed JSON recovery
- Discord webhook notifications

### Changed
- Job services now run `news-app run-job {id}` instead of shell script
- Cleanup service runs `news-app cleanup` instead of shell script
- Troubleshoot service runs `news-app troubleshoot` instead of shell script

## [2026-02-05] - Code Quality Improvements

### Added
- `internal/util/` package for shared utilities
- Job status constants (`StatusRunning`, `StatusCompleted`, etc.)
- Frequency constants (`FreqHourly`, `FreqDaily`, etc.)
- Style guide documentation (`docs/STYLE.md`)

### Changed
- Simplified HTTP handlers with helper functions
- Centralized environment variable handling
- Improved error handling throughout

### Fixed
- SQL injection vulnerabilities in bash script (now replaced with Go)
- Silent error handling in various places

## [2026-02-04] - Job Run Tracking

### Added
- `duplicates_skipped` column in `job_runs` table
- `log_path` column in `job_runs` table
- Per-run log files in `logs/runs/`
- Article deduplication by URL

### Changed
- Improved article indexing for faster queries

## [2026-01-27] - Initial Release

### Added
- Multi-user web application for news retrieval
- Job scheduling with systemd timers
- Shelley AI integration for news search
- Full article content fetching
- User preferences (system prompt, Discord notifications)
- SQLite database with migrations
- exe.dev authentication integration
