# Rewriting run-job.sh in Go

## Current State (Bash)

The 558-line bash script does:
1. Parse job ID from args
2. Load job details and user preferences from SQLite
3. Cancel orphaned runs, create new job_run record
4. Build prompt from job config + user system prompt
5. Create/resume Shelley API conversation
6. Poll for completion (up to 25 min timeout)
7. Extract JSON array from agent response (with malformed JSON fixing)
8. For each article: fetch full content via HTTP + readability parser
9. Save article files and database records
10. Update job status, calculate next_run_at
11. Archive conversation and subagents
12. Send Discord notifications

## Proposed Go Structure

```
news-app
├── cmd/srv/main.go          # Add "run-job" subcommand
├── jobrunner/
│   ├── runner.go            # Main JobRunner struct and Run() method
│   ├── shelley.go           # Shelley API client
│   ├── content.go           # Article content fetcher (HTML → text)
│   ├── json.go              # JSON extraction and fixing
│   └── discord.go           # Discord webhook notifications
```

## Benefits

| Aspect | Bash | Go |
|--------|------|----|
| **Error handling** | Fragile, easy to miss | Explicit, typed |
| **Testing** | Difficult | Unit testable |
| **Dependencies** | python3, jq, curl, sqlite3 | Single binary |
| **Concurrency** | Sequential article fetching | Parallel with goroutines |
| **JSON parsing** | Inline Python hack | Proper Go with recovery |
| **Logging** | echo + tee | Structured slog |
| **Deployment** | Script + venv + deps | Single binary |

## Key Design Decisions

### 1. Subcommand vs Separate Binary

**Recommendation: Subcommand**

```bash
# Instead of:
./run-job.sh 123

# Use:
./news-app run-job 123
```

Benefits:
- Single binary to deploy
- Shares database code with server
- No need to maintain separate build

### 2. HTML Content Extraction

The bash script uses `readability-lxml` (Python). Options:

1. **go-readability** - Pure Go port of Mozilla Readability
2. **goquery** - jQuery-like HTML parsing (more manual)
3. **colly** - Web scraping framework

**Recommendation: go-readability** (`github.com/go-shiori/go-readability`)
- Direct port of the algorithm already in use
- Well-maintained, used by Shiori bookmarks

### 3. Shelley API Client

Simple HTTP client with types:

```go
type ShelleyClient struct {
    BaseURL    string
    HTTPClient *http.Client
}

type ConversationResponse struct {
    ConversationID string    `json:"conversation_id"`
    Messages       []Message `json:"messages"`
}

type Message struct {
    Type      string          `json:"type"`
    EndOfTurn bool            `json:"end_of_turn"`
    LLMData   json.RawMessage `json:"llm_data"`
}
```

### 4. Parallel Article Fetching

Fetch articles concurrently with bounded parallelism:

```go
func (r *Runner) fetchArticles(articles []ArticleInfo) []Article {
    sem := make(chan struct{}, 5) // max 5 concurrent
    var wg sync.WaitGroup
    results := make(chan Article, len(articles))
    
    for _, info := range articles {
        wg.Add(1)
        go func(info ArticleInfo) {
            defer wg.Done()
            sem <- struct{}{}
            defer func() { <-sem }()
            
            content := r.fetchContent(info.URL)
            results <- Article{Info: info, Content: content}
        }(info)
    }
    
    wg.Wait()
    close(results)
    // collect results...
}
```

### 5. Logging

Use `log/slog` with file + stdout output:

```go
logFile, _ := os.Create(logPath)
multiWriter := io.MultiWriter(os.Stdout, logFile)
logger := slog.New(slog.NewTextHandler(multiWriter, nil))
```

### 6. Configuration

Environment variables (same as bash):

```go
type Config struct {
    DBPath       string        // NEWS_APP_DB_PATH
    ArticlesDir  string        // NEWS_APP_ARTICLES_DIR  
    LogsDir      string        // NEWS_APP_LOGS_DIR
    ShelleyAPI   string        // NEWS_APP_SHELLEY_API
    JobTimeout   time.Duration // NEWS_JOB_TIMEOUT
    PollInterval time.Duration // NEWS_JOB_POLL_INTERVAL
}
```

## Implementation Plan

### Phase 1: Core Structure
- [ ] Add `run-job` subcommand to main.go
- [ ] Create `jobrunner/runner.go` with main flow
- [ ] Create `jobrunner/shelley.go` for API client

### Phase 2: Content Fetching
- [ ] Add go-readability dependency
- [ ] Create `jobrunner/content.go` for HTML extraction
- [ ] Implement parallel fetching

### Phase 3: JSON Handling
- [ ] Create `jobrunner/json.go` for extraction
- [ ] Port the malformed JSON fixing logic

### Phase 4: Notifications & Cleanup
- [ ] Create `jobrunner/discord.go`
- [ ] Implement conversation archiving

### Phase 5: Update systemd
- [ ] Update `srv/systemd.go` to use `./news-app run-job {id}`
- [ ] Test end-to-end

## Estimated Impact

- **Lines of code**: ~400 Go vs 558 bash (cleaner, more maintainable)
- **Dependencies**: +1 (go-readability) vs python3+venv+jq+curl
- **Binary size**: ~+2MB (readability HTML parsing)
- **Performance**: Faster (parallel fetching, no subprocess spawning)
