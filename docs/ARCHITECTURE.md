# Architecture

## Overview

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│     Browser     │─────│   exe.dev Proxy │─────│   Go Server     │
│                 │     │  (adds auth)    │     │   (port 8000)   │
└─────────────────┘     └─────────────────┘     └────────┬────────┘
                                                         │
                                                 ┌───────┴───────┐
                                                 │    SQLite     │
                                                 │  (db.sqlite3) │
                                                 └───────────────┘

┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  systemd Timer  │─────│ news-app run-job│─────│  Shelley API    │
│  (scheduling)   │     │  (Go jobrunner) │     │ (localhost:9999)│
└─────────────────┘     └────────┬────────┘     └─────────────────┘
                                 │
                         ┌───────┴───────┐
                         │   articles/   │
                         │  (text files) │
                         └───────────────┘
```

## Components

### Go Web Server (`internal/web/`)

The main application server handles:
- **Pages**: Dashboard, jobs list, job detail, job edit, articles, preferences
- **API**: CRUD for jobs, run/stop jobs, update preferences, serve article content
- **Auth**: Uses exe.dev proxy headers (`X-ExeDev-UserID`, `X-ExeDev-Email`)

### SQLite Database (`internal/db/`)

Tables:
- `users` - User accounts (created on first visit)
- `preferences` - User settings (system prompt, Discord webhook, notifications)
- `jobs` - News retrieval jobs (prompt, filters, schedule)
- `job_runs` - Execution history
- `articles` - Article metadata (title, URL, summary, content_path)

### Job Runner (`internal/jobrunner/`)

Go implementation that:
1. Reads job config from database
2. Builds prompt with user's system prompt + job filters
3. Creates conversation via Shelley API
4. Polls for completion (checks `end_of_turn: true`)
5. Extracts JSON array from response
6. For each article URL, fetches full content via go-readability
7. Saves articles to `articles/job_{id}/article_{id}_{timestamp}.txt`
8. Updates database with article metadata
9. Sends optional Discord notifications

### systemd Timers (`deploy/`)

Each job gets a systemd timer for scheduling:
- `news-job-{id}.service` - Runs `news-app run-job {id}`
- `news-job-{id}.timer` - Triggers based on frequency

Additional maintenance services:
- `news-cleanup.timer` - Runs `news-app cleanup` every 48h
- `news-troubleshoot.timer` - Runs `news-app troubleshoot` daily at 07:00

## Data Flow

### Creating a Job

1. User submits form → `POST /api/jobs`
2. Server creates job record in database
3. Server creates systemd timer for scheduling
4. Redirects to dashboard with success message

### Editing a Job

1. User clicks "Edit" on job detail page → `GET /jobs/{id}/edit`
2. Server renders form pre-filled with job data
3. User submits changes → `PUT /api/jobs/{id}`
4. Server updates job record and systemd timer
5. Redirects to job detail page

### Running a Job

1. systemd timer triggers (or user clicks "Run")
2. `news-app run-job {id}` executes:
   - Creates conversation with Shelley API
   - Agent searches web, returns JSON array
   - Fetches full content for each URL using go-readability
   - Saves to `articles/job_{id}/`
   - Updates database
3. Optional: Discord notification on success/failure

### Viewing Articles

1. User visits `/articles`
2. Server queries articles for user
3. User clicks article → `/articles/{id}`
4. "View text file" link → `/api/articles/{id}/content`
5. Server serves file from `articles/job_{id}/article_{id}_{timestamp}.txt`

## File Storage

Article content is stored as plain text files:

```
articles/
└── job_6/
    ├── article_15_20260121_045918.txt
    ├── article_16_20260121_045918.txt
    └── ...
```

Each file contains:
```
Title: Article headline
URL: https://source.com/article
Retrieved: 2026-01-21T04:59:18+00:00

--- Summary ---
Brief summary from agent...

--- Full Content ---
Full article text extracted from webpage...
```

## Logs

```
logs/
├── runs/           # Per-job-run logs
│   └── run_{id}_{timestamp}.log
└── troubleshoot/   # Auto-diagnosis reports
    └── report-{date}.md
```

## Authentication

exe.dev proxy adds headers:
- `X-ExeDev-UserID` - Unique user ID
- `X-ExeDev-Email` - User's email

Server creates user record on first visit. All queries filter by `user_id`.

## Shelley API Integration

The job runner uses the local Shelley API (see SHELLEY_API.md for details):

```go
// Create conversation
client := jobrunner.NewShelleyClient("http://localhost:9999")
convID, err := client.CreateConversation(ctx, jobID, prompt)

// Poll for completion
for {
    conv, _ := client.GetConversation(ctx, jobID, convID)
    if conv.IsComplete() {
        break
    }
    time.Sleep(10 * time.Second)
}

// Extract response text
text := conv.GetLastAgentText()
```

The agent spawns a subagent to search the web. The prompt includes an instruction to wait for subagent completion before returning.

## Resilience Features

### Service Restart Recovery

The app automatically resumes interrupted jobs after service restarts:

**Automatic Job Resumption:**
1. **Detection**: On startup, scans for job runs stuck in "running" state
2. **Mark Failed**: Marks the interrupted run as failed with message "interrupted by service restart (auto-restarting)"
3. **Reset Job**: Resets job status to 'idle' to allow new run
4. **Restart**: Immediately starts a new run for the same job
5. **Resume**: New run detects existing Shelley conversation and resumes polling it
6. **Complete**: Job continues from where it left off until conversation completes

**Deduplication**: If multiple runs of the same job were stuck, only one restart is triggered to avoid race conditions.

**Example Recovery:**
```
21:04:50 - Service restarted
21:04:51 - Found run 164 stuck (job 26, conversation cKVMHUC)
21:04:51 - Marked run 164 as failed
21:04:51 - Reset job 26 status to 'idle'
21:04:51 - Started new run 165 for job 26
21:04:52 - Run 165 detected existing conversation cKVMHUC
21:04:52 - Resuming polling of conversation cKVMHUC
21:12:30 - Conversation completes, articles processed
```

### Separate Process Execution

When jobs are triggered manually (not via systemd), they run as independent processes rather than goroutines within the web server. This ensures jobs continue running even if the web server process restarts.

### Signal Handling

Job runners listen for SIGTERM and SIGINT signals and attempt graceful shutdown:
- Cancel ongoing operations via context
- Wait up to 10 seconds for cleanup
- Update database status before exit

### Context Propagation

The job runner respects context cancellation throughout its execution:
- During conversation polling
- During content fetching
- During database operations

### Manual Recovery

If a job was killed but the Shelley conversation completed successfully, you can manually recover the results:

```bash
# Extract conversation results to JSON
sqlite3 ~/.config/shelley/shelley.db "SELECT json_extract(llm_data, '$.Content[0].Text') FROM messages WHERE conversation_id='CONV_ID' AND type='agent' ORDER BY sequence_id DESC LIMIT 1;" > articles.json

# Process the articles
./news-app process-articles <job_id> articles.json
```

### Best Practices

- **Use systemd services**: Prefer scheduled jobs over manual runs for production workloads
- **Monitor disk space**: The Shelley database can grow large; see [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
- **Check logs after restarts**: Review `/var/log/syslog` and `journalctl -u news-app` for stuck job recovery messages
