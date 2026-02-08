# Agent Instructions

This is the News Agent app for exe.dev.

## Overview

A multi-user web app that retrieves news articles using the Shelley AI agent. See README.md for user-facing docs and ARCHITECTURE.md for system design.

## Key Files

- `internal/web/server.go` - HTTP router and server setup
- `internal/web/handlers.go` - Page handlers (dashboard, jobs, job edit, articles, preferences)
- `internal/web/api.go` - API handlers (CRUD, job control, article content)
- `internal/web/systemd.go` - Creates/removes systemd timers for job scheduling
- `internal/jobrunner/runner.go` - Job execution logic (calls Shelley API, fetches articles)
- `internal/jobrunner/shelley.go` - Shelley API client
- `internal/jobrunner/cleanup.go` - Conversation cleanup (`news-app cleanup` subcommand)
- `internal/jobrunner/troubleshoot.go` - Auto-diagnosis (`news-app troubleshoot` subcommand)
- `internal/db/migrations/` - Database schema
- `internal/db/queries/` - sqlc query definitions

## Common Tasks

### Adding a new page

1. Add handler in `internal/web/handlers.go`
2. Add route in `internal/web/server.go`
3. Create template in `internal/web/templates/`

### Adding a new API endpoint

1. Add handler in `internal/web/api.go`
2. Add route in `internal/web/server.go`

### Modifying database schema

1. Create new migration file in `internal/db/migrations/`
2. Update queries in `internal/db/queries/`
3. Run `sqlc generate` in `internal/db/`

### Modifying job behavior

Edit `internal/jobrunner/runner.go`. Key methods:
- `Run()` - Main entry point
- `buildPrompt()` - Constructs the Shelley prompt
- `runConversation()` - Shelley API interaction
- `extractArticles()` - JSON extraction from response
- `fetchArticleContent()` - HTTP fetch + content extraction

## Build & Deploy

```bash
make build
sudo systemctl restart news-app
```

## Subcommands

The binary supports multiple subcommands:

```bash
# Run web server (default)
./news-app -listen :8000

# Run a job manually
./news-app run-job {job_id}

# Cleanup old conversations
./news-app cleanup [--max-age 48] [--dry-run]

# Diagnose failed runs
./news-app troubleshoot [--lookback 24] [--dry-run]
```

## Testing

```bash
# Run a job manually
./news-app run-job {job_id}

# Check logs
journalctl -u news-app -f
journalctl -u news-job-{id} -f

# Run tests
go test ./...
```

### Browser Testing and Page Viewing

**Authentication Requirements:**

The news-app requires exe.dev authentication headers on ALL requests. Without these headers, requests are redirected to `/__exe.dev/login`.

**Required Headers:**
- `X-ExeDev-UserID` - User identifier (e.g., "test-user-123")
- `X-ExeDev-Email` - User email (e.g., "test@example.com")

**When Testing with curl:**
```bash
curl -H "X-ExeDev-UserID: test-user-123" \
     -H "X-ExeDev-Email: test@example.com" \
     http://localhost:8000/
```

**When Testing with Browser Automation:**

Direct navigation to `http://localhost:8000/` will result in a 404 redirect because the browser cannot inject headers automatically.

**Solutions:**

1. **Fetch and inject (simplest):**
   ```javascript
   const response = await fetch('http://localhost:8000/', {
     headers: {
       'X-ExeDev-UserID': 'test-user-123',
       'X-ExeDev-Email': 'test@example.com'
     }
   });
   const html = await response.text();
   document.open();
   document.write(html);
   document.close();
   ```
   
   Note: This approach loads the initial HTML but may have issues with relative links and static resources, which won't automatically include the headers.

2. **Access via exe.dev proxy (production):**
   Navigate to `https://your-vm.exe.xyz:8000/`
   
   The exe.dev HTTPS proxy automatically injects the authentication headers based on your logged-in user.

3. **Use a local proxy:**
   Set up a reverse proxy (like nginx) that adds the headers to all requests before forwarding to the app.

**Why This is Needed:**

The app's `getOrCreateUser()` function (in `internal/web/server.go`) checks for `X-ExeDev-UserID` header on every request. If missing, it returns an authentication error, causing handlers to call `redirectToLogin()`, which redirects to `/__exe.dev/login?redirect={path}`.

**In Production:**

When accessed through the exe.dev proxy (`https://your-vm.exe.xyz:8000/`), all authentication is handled automatically by the proxy infrastructure. No additional configuration is needed.
