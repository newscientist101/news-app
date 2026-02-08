# API Reference

The news-app provides a REST API for managing jobs, articles, and preferences.

## Authentication

All API endpoints require authentication via exe.dev proxy headers:
- `X-ExeDev-UserID` - User identifier
- `X-ExeDev-Email` - User email

POST/PUT/DELETE requests also require the `X-News-App-Request: 1` header for CSRF protection.

## Response Format

All API responses are JSON. Successful responses return the requested data or a status object:

```json
{"status": "ok"}
```

Error responses include an error message:

```json
{"error": "Job not found"}
```

---

## Health Check

### GET /health

Returns server health status. No authentication required.

**Response:**
```json
{"status": "ok"}
```

---

## Jobs

### POST /api/jobs

Create a new job.

**Request Body:**
```json
{
  "name": "AI News",
  "prompt": "Find the latest news about artificial intelligence",
  "keywords": "AI, machine learning, LLM",
  "sources": "techcrunch.com, arstechnica.com",
  "region": "US",
  "frequency": "daily",
  "is_one_time": false
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Job display name |
| `prompt` | string | Yes | Search prompt for the AI agent |
| `keywords` | string | No | Comma-separated keywords to filter results |
| `sources` | string | No | Comma-separated preferred sources |
| `region` | string | No | Geographic region (e.g., "US", "EU") |
| `frequency` | string | Yes | One of: `hourly`, `6hours`, `daily`, `weekly` |
| `is_one_time` | boolean | No | If true, job runs once then deactivates |

**Response:** Created job object

**Errors:**
- `400` - Invalid request body or missing required fields
- `401` - Unauthorized
- `429` - Rate limit exceeded

---

### PUT /api/jobs/{id}

Update an existing job.

**Request Body:**
```json
{
  "name": "AI News Updated",
  "prompt": "Find the latest news about AI and robotics",
  "keywords": "AI, robotics",
  "sources": "",
  "region": "US",
  "frequency": "daily",
  "is_active": true
}
```

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Job display name |
| `prompt` | string | Search prompt |
| `keywords` | string | Filter keywords |
| `sources` | string | Preferred sources |
| `region` | string | Geographic region |
| `frequency` | string | Schedule frequency |
| `is_active` | boolean | Whether job is active |

**Response:**
```json
{"status": "ok"}
```

**Errors:**
- `400` - Invalid request body
- `401` - Unauthorized
- `404` - Job not found

---

### DELETE /api/jobs/{id}

Delete a job and all its articles.

**Response:**
```json
{"status": "ok"}
```

**Errors:**
- `401` - Unauthorized
- `404` - Job not found

---

### POST /api/jobs/{id}/run

Manually trigger a job run.

**Response:**
```json
{"status": "started"}
```

**Errors:**
- `400` - Job is already running
- `401` - Unauthorized
- `404` - Job not found
- `429` - Rate limit exceeded

---

### POST /api/jobs/{id}/stop

Stop a running job.

**Response:**
```json
{"status": "stopped"}
```

**Errors:**
- `401` - Unauthorized
- `404` - Job not found

---

## Job Runs

### POST /api/runs/{id}/cancel

Cancel a pending or running job run.

**Response:**
```json
{"status": "ok"}
```

**Errors:**
- `401` - Unauthorized
- `404` - Run not found

---

### GET /api/runs/{id}/log

Get the log file for a job run.

**Response:** Plain text log content

**Content-Type:** `text/plain`

**Errors:**
- `401` - Unauthorized
- `404` - Run or log not found

---

## Articles

### GET /api/articles/{id}/content

Get the full text content of an article.

**Response:** Plain text article content

**Content-Type:** `text/plain`

**Errors:**
- `401` - Unauthorized
- `404` - Article not found

---

### POST /api/articles/delete

Delete multiple articles.

**Request Body:**
```json
{
  "article_ids": [1, 2, 3]
}
```

**Response:**
```json
{"status": "ok", "deleted": 3}
```

**Errors:**
- `400` - Invalid request body
- `401` - Unauthorized

---

## Preferences

### POST /api/preferences

Update user preferences.

**Request Body:**
```json
{
  "system_prompt": "You are a helpful news assistant...",
  "discord_webhook": "https://discord.com/api/webhooks/...",
  "notify_success": true,
  "notify_failure": true
}
```

| Field | Type | Description |
|-------|------|-------------|
| `system_prompt` | string | Custom system prompt prepended to all job prompts |
| `discord_webhook` | string | Discord webhook URL for notifications |
| `notify_success` | boolean | Send notification on successful job runs |
| `notify_failure` | boolean | Send notification on failed job runs |

**Response:**
```json
{"status": "ok"}
```

**Errors:**
- `400` - Invalid request body
- `401` - Unauthorized

---

## Rate Limiting

The following endpoints are rate-limited per user:

| Endpoint | Limit |
|----------|-------|
| `POST /api/jobs` | 1 request per minute |
| `POST /api/jobs/{id}/run` | 1 request per minute |

Exceeding the rate limit returns `429 Too Many Requests`.

---

## Page Endpoints

These endpoints return HTML pages (not API endpoints):

| Endpoint | Description |
|----------|-------------|
| `GET /` | Dashboard |
| `GET /jobs` | Jobs list |
| `GET /jobs/new` | New job form |
| `GET /jobs/{id}` | Job detail |
| `GET /jobs/{id}/edit` | Edit job form |
| `GET /articles` | Articles list |
| `GET /articles/{id}` | Article detail |
| `GET /preferences` | User preferences |
| `GET /runs` | Job runs history |
