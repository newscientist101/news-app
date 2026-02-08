# Code Style Guide

Patterns and conventions for the news-app codebase.

## General Principles

1. **Don't wrap stdlib** - Avoid thin wrappers around standard library functions. Use `max()`, `strconv.Atoi()`, etc. directly.
2. **Helpers should earn their place** - Create helpers when a pattern appears 3+ times, or when it encapsulates meaningful logic.
3. **Keep helpers local when possible** - If a helper is only used in one file, define it there. Move to `internal/util` only when shared across packages.

## Package Organization

### `internal/util`

Shared utilities used across multiple packages. Contains:

- **Constants** - `FreqHourly`, `StatusRunning`, etc.
- **Environment helpers** - `GetEnv(key, default)`
- **Business logic** - `CalculateNextRun()`, `FrequencyToCalendar()`

```go
// Good: shared across web and jobrunner
util.GetEnv("NEWS_APP_DB_PATH", "/default/path")
util.CalculateNextRun(frequency, isOneTime)

// Bad: only used in one place, keep local
util.ParseInt(s)  // just use strconv.Atoi
```

### Server struct

Store shared resources on the Server struct to avoid repeated initialization:

```go
// Good: initialized once
q := s.Queries

// Bad: creates new instance per request
q := dbgen.New(s.DB)
```

## HTTP Handlers

### Authentication

Use `redirectToLogin` for page handlers, `jsonUnauthorized` for API handlers:

```go
// Page handler
user, err := s.getOrCreateUser(r)
if err != nil {
    redirectToLogin(w, r)
    return
}

// API handler
user, err := s.getOrCreateUser(r)
if err != nil {
    s.jsonUnauthorized(w)
    return
}
```

### Path Parameters

Use `parsePathID` for ID extraction:

```go
// Good
id, ok := parsePathID(w, r, "Invalid job ID")
if !ok {
    return
}

// Bad: repetitive
idStr := r.PathValue("id")
id, err := strconv.ParseInt(idStr, 10, 64)
if err != nil {
    http.Error(w, "Invalid job ID", http.StatusBadRequest)
    return
}
```

### JSON Responses

```go
// Status-only responses
s.jsonStatus(w, "ok")
s.jsonStatus(w, "started")

// Data responses
s.jsonOK(w, job)

// Error responses
s.jsonError(w, "Job not found", 404)
s.jsonUnauthorized(w)
```

### Template Rendering

`renderTemplate` handles Content-Type and errors internally:

```go
// Good: clean call
s.renderTemplate(w, "dashboard.html", data)

// Bad: unnecessary error handling
if err := s.renderTemplate(w, "dashboard.html", data); err != nil {
    http.Error(w, err.Error(), 500)
}
```

## Constants

### Job Frequencies

```go
util.FreqHourly   // "hourly"
util.Freq6Hours   // "6hours"
util.FreqDaily    // "daily"
util.FreqWeekly   // "weekly"
```

### Job/Run Status

```go
util.StatusPending    // "pending"
util.StatusRunning    // "running"
util.StatusCompleted  // "completed"
util.StatusFailed     // "failed"
util.StatusStopped    // "stopped"
util.StatusCancelled  // "cancelled"
```

Use constants in Go code. SQL queries must use string literals.

## Systemd Integration

Use `jobServiceName` for consistent service naming:

```go
// Good
serviceName := jobServiceName(job.ID)

// Bad: duplicated format string
serviceName := fmt.Sprintf("news-job-%d", job.ID)
```

## Environment Variables

Use `util.GetEnv` with sensible defaults:

```go
util.GetEnv("NEWS_APP_ARTICLES_DIR", "/home/exedev/news-app/articles")
```

For integers, use a local helper if only needed in one file:

```go
func getEnvInt(key string, defaultVal int) int {
    if v := os.Getenv(key); v != "" {
        if i, err := strconv.Atoi(v); err == nil {
            return i
        }
    }
    return defaultVal
}
```

## What NOT to Abstract

- Simple one-liners that are clear inline
- Patterns that appear only once or twice
- Error messages (keep them specific to context)
- SQL queries (can't use Go constants anyway)
