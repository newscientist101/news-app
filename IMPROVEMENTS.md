# News App - Potential Improvements

This document lists potential improvements identified during a code review.

## High Priority (COMPLETED)

> All high priority issues have been addressed in commit 83b0dcb.

### 1. Security: SQL Injection in run-job.sh
**Risk: High**

The bash script interpolates variables directly into SQL queries:
```bash
db "SELECT ... FROM jobs WHERE id = $JOB_ID;"
db "UPDATE job_runs SET ... WHERE id = $ORPHAN_ID;"
```

While `JOB_ID` comes from systemd (trusted), other values like `TITLE_ESC` use basic quote escaping that could be bypassed.

**Recommendation:** Use parameterized queries or a safer escaping mechanism. Consider rewriting the job runner in Go.

### 2. Error Handling: Silent Failures
**Risk: Medium**

Many places ignore errors with `_`:
```go
jobs, _ := q.ListJobsByUser(r.Context(), user.ID)
count, _ := q.CountArticlesByUser(r.Context(), user.ID)
```

**Recommendation:** Log errors even if not surfacing to user. At minimum:
```go
if err != nil {
    slog.Warn("failed to list jobs", "error", err)
}
```

### 3. Input Validation: ParseInt Errors Ignored
**Risk: Medium**

All ID parsing ignores errors:
```go
id, _ := strconv.ParseInt(idStr, 10, 64)
```

If `idStr` is malformed, `id` will be 0, potentially causing unexpected behavior.

**Recommendation:** Return 400 Bad Request on parse errors.

### 4. Tests Are Broken/Incomplete
**Risk: Medium**

`server_test.go` references `HandleRoot` which panics with "unimplemented". The tests don't reflect the actual application.

**Recommendation:** Remove or fix the tests. Add proper integration tests for critical paths.

---

## Medium Priority (COMPLETED)

> All medium priority issues have been addressed in commit 49307f0.

### 5. Template Parsing on Every Request
**Issue: Performance**

Templates are parsed fresh on every request:
```go
tmpl, err := template.New("").Funcs(funcMap).ParseFiles(layoutPath, path)
```

**Recommendation:** Parse templates once at startup and cache them.

### 6. Hardcoded Paths
**Issue: Maintainability**

Paths are hardcoded throughout:
- `run-job.sh`: `/home/exedev/news-app/db.sqlite3`
- `systemd.go`: `/home/exedev/news-app/run-job.sh`
- `server.go`: `/home/exedev/news-app/articles`

**Recommendation:** Use environment variables or config file.

### 7. No Rate Limiting
**Issue: Security/Abuse**

No rate limiting on API endpoints. Users could spam job runs.

**Recommendation:** Add rate limiting, especially for `POST /api/jobs/{id}/run`.

### 8. Job Runner Timeout
**Issue: Resource Management**

The script waits up to 25 minutes (`MAX_WAIT=1500`). This is very long.

**Recommendation:** Consider making timeout configurable per job, or reducing default.

### 9. No Pagination on Job Detail Articles
**Issue: Performance**

`handleJobDetail` loads all articles for a job without pagination:
```go
articles, _ := q.ListArticlesByJob(r.Context(), job.ID)
```

**Recommendation:** Add pagination like the articles list page.

### 10. Missing Database Indexes
**Issue: Performance**

No index on `articles.retrieved_at` which is used in date filtering.

**Recommendation:** Add index:
```sql
CREATE INDEX idx_articles_retrieved_at ON articles(retrieved_at);
```

---

## Low Priority

### 11. Duplicate JavaScript Functions ✅ COMPLETED
**Issue: Maintainability**

`runJob`, `stopJob`, `deleteJob` are defined in `app.js` but also used inline in templates via `onclick`. Some pages have duplicate script blocks.

**Recommendation:** Consolidate all JavaScript in `app.js`.

**Resolution:** Consolidated all JS functions into app.js, added helpers for form submission, auto-initialization on DOMContentLoaded.

### 12. No CSRF Protection ✅ COMPLETED
**Issue: Security**

API endpoints have no CSRF tokens. While exe.dev auth headers provide some protection, explicit CSRF tokens are better practice.

**Recommendation:** Add CSRF tokens to forms, or use SameSite cookies.

**Resolution:** Added CSRFStore with per-user tokens (24hr TTL), csrfProtect middleware, X-CSRF-Token header validation on all mutating endpoints.

### 13. Article Content Extraction is Basic ✅ COMPLETED
**Issue: Functionality**

The Python content extractor only pulls `<p>` tags, missing content in `<article>`, `<div>`, etc.

**Recommendation:** Use a proper library like `readability-lxml` or `newspaper3k`.

**Resolution:** Switched to readability-lxml (Mozilla Readability algorithm) in a virtual environment at .venv.

### 14. No Job Run Log/History Viewer ✅ COMPLETED
**Issue: UX**

Users can't see detailed logs of what happened during a job run.

**Recommendation:** Store and display job execution logs.

**Resolution:** Added `log_path` column to job_runs table. run-job.sh now captures all output to timestamped log files in `logs/runs/`. Added `/api/runs/{id}/log` endpoint and "View Log" button with modal viewer on runs page. Supports auto-refresh for watching live job progress.

### 15. Discord Notifications Have No Retry ✅ COMPLETED
**Issue: Reliability**

Discord webhook calls can fail silently.

**Recommendation:** Add retry logic or queue failed notifications.

**Resolution:** Added send_discord_notification() function with 3 retries and exponential backoff. Handles rate limiting (429) with longer delays.

### 16. Conversation Cleanup Could Be Better ❌ SKIPPED
**Issue: Resource Management**

Conversations are archived but subagent detection relies on API call that could fail.

**Recommendation:** More robust cleanup, perhaps as a separate cron job.

**Status:** Skipped - Shelley API likely handles this internally.

### 17. No Health Check Endpoint ✅ COMPLETED
**Issue: Operations**

No `/health` endpoint for monitoring.

**Recommendation:** Add basic health check that verifies DB connectivity.

**Resolution:** Added GET /health endpoint that checks database connectivity and returns JSON status. Returns 200 when healthy, 503 when degraded.

### 18. Missing Form Validation Feedback ✅ COMPLETED
**Issue: UX**

Job creation form does client-side validation but error messages are basic `alert()` boxes.

**Recommendation:** Show inline validation errors.

**Resolution:** Replaced all alert() calls with a toast notification system. Toasts slide in from the right, auto-dismiss, and show success/error/info variants with appropriate styling.

### 19. Static File Caching Headers ✅ COMPLETED
**Issue: Performance**

No cache headers on static files.

**Recommendation:** Add `Cache-Control` headers for CSS/JS files.

**Resolution:** Added cacheControl middleware that sets `Cache-Control: public, max-age=86400` (1 day) on all static files.

### 20. Runs Page Could Show More Details ✅ COMPLETED
**Issue: UX**

Runs page shows limited info. Could link to conversation or show more stats.

**Recommendation:** Add link to view raw agent output, show total articles attempted vs saved.

**Resolution:** Improved results display to show "X saved / Y dupes" format with tooltips. Added green highlighting for saved count and better styling for both in-progress and recent runs tables.

---

## Code Quality

### 21. Inconsistent Error Messages
Some errors return "Failed to X" while others return "X not found". Standardize.

### 22. Magic Numbers
Values like `50` (page limit), `1500` (timeout), `60` (random delay) should be constants.

### 23. Long Functions
`handleArticlesList` is ~100 lines. Consider breaking into smaller functions.

### 24. No Logging in Handlers
Most handlers don't log anything. Add request logging for debugging.

---

## Quick Wins

1. Add index on `articles.retrieved_at`
2. Add health check endpoint
3. Fix/remove broken tests
4. Add static file cache headers
5. Log errors instead of ignoring them

---

## Summary

| Category | Count | Status |
|----------|-------|--------|
| High Priority | 4 | ✅ COMPLETED |
| Medium Priority | 6 | ✅ COMPLETED |
| Low Priority | 10 | 4 done, 6 pending |
| Code Quality | 4 | Pending |

~~Most critical: SQL injection risk in bash script and silent error handling throughout.~~

High and medium priority issues have been resolved.
