#!/bin/bash
set -e

JOB_ID="$1"
DB_PATH="/home/exedev/news-app/db.sqlite3"
ARTICLES_DIR="/home/exedev/news-app/articles"
LOGS_DIR="/home/exedev/news-app/logs/runs"
SHELLEY_API="http://localhost:9999"

# Create logs directory
mkdir -p "$LOGS_DIR"

# SQLite with busy timeout (30 seconds) and WAL mode for better concurrency
db() {
    sqlite3 -cmd ".timeout 30000" "$DB_PATH" "$@"
}

# Send Discord notification with retry logic
# Args: $1 = message content
send_discord_notification() {
    local message="$1"
    local max_retries=3
    local retry_delay=2
    
    if [ -z "$DISCORD_WEBHOOK" ]; then
        return 0
    fi
    
    for attempt in $(seq 1 $max_retries); do
        local http_code
        http_code=$(curl -s -o /dev/null -w "%{http_code}" \
            -H "Content-Type: application/json" \
            -d "{\"content\": \"$message\"}" \
            "$DISCORD_WEBHOOK")
        
        if [ "$http_code" = "200" ] || [ "$http_code" = "204" ]; then
            echo "Discord notification sent successfully"
            return 0
        fi
        
        # Rate limited - wait longer
        if [ "$http_code" = "429" ]; then
            echo "Discord rate limited, waiting ${retry_delay}s before retry $attempt/$max_retries"
            sleep $retry_delay
            retry_delay=$((retry_delay * 2))
            continue
        fi
        
        # Other error - retry with backoff
        if [ $attempt -lt $max_retries ]; then
            echo "Discord notification failed (HTTP $http_code), retrying in ${retry_delay}s ($attempt/$max_retries)"
            sleep $retry_delay
            retry_delay=$((retry_delay * 2))
        else
            echo "Discord notification failed after $max_retries attempts (HTTP $http_code)"
            return 1
        fi
    done
}

# Enable WAL mode once at startup
db "PRAGMA journal_mode=WAL;" > /dev/null 2>&1 || true

if [ -z "$JOB_ID" ]; then
    echo "Usage: $0 <job_id>"
    exit 1
fi

# Add random delay (0-60 seconds) to stagger concurrent job starts
# This prevents overwhelming the Shelley API when multiple jobs start simultaneously
sleep $(( RANDOM % 60 ))

# Get job details
JOB_DATA=$(db "SELECT user_id, name, prompt, keywords, sources, region, frequency, is_one_time FROM jobs WHERE id = $JOB_ID;")
if [ -z "$JOB_DATA" ]; then
    echo "Job not found: $JOB_ID"
    exit 1
fi

IFS='|' read -r USER_ID JOB_NAME PROMPT KEYWORDS SOURCES REGION FREQUENCY IS_ONE_TIME <<< "$JOB_DATA"

# Get user preferences (system prompt)
SYSTEM_PROMPT=$(db "SELECT system_prompt FROM preferences WHERE user_id = $USER_ID;" 2>/dev/null || echo "")
DISCORD_WEBHOOK=$(db "SELECT discord_webhook FROM preferences WHERE user_id = $USER_ID;" 2>/dev/null || echo "")
NOTIFY_SUCCESS=$(db "SELECT notify_success FROM preferences WHERE user_id = $USER_ID;" 2>/dev/null || echo "0")
NOTIFY_FAILURE=$(db "SELECT notify_failure FROM preferences WHERE user_id = $USER_ID;" 2>/dev/null || echo "0")

# Check for existing conversation that might still be running
EXISTING_CONV=$(db "SELECT current_conversation_id FROM jobs WHERE id = $JOB_ID;" 2>/dev/null || echo "")

# Cancel any orphaned 'running' job_runs for this job (from crashed previous runs)
ORPHANED_RUNS=$(db "SELECT id FROM job_runs WHERE job_id = $JOB_ID AND status = 'running';")
for ORPHAN_ID in $ORPHANED_RUNS; do
    echo "Cancelling orphaned run $ORPHAN_ID"
    db "UPDATE job_runs SET status = 'cancelled', error_message = 'Cancelled: new run started', completed_at = datetime('now') WHERE id = $ORPHAN_ID;"
done

# Create job run record and update job status to running
RUN_ID=$(db "INSERT INTO job_runs (job_id, status, started_at) VALUES ($JOB_ID, 'running', datetime('now')); SELECT last_insert_rowid();")
db "UPDATE jobs SET status = 'running', updated_at = datetime('now') WHERE id = $JOB_ID;"

# Set up log file for this run
LOG_FILE="$LOGS_DIR/run_${RUN_ID}_$(date +%Y%m%d_%H%M%S).log"
db "UPDATE job_runs SET log_path = '$LOG_FILE' WHERE id = $RUN_ID;"

# Redirect all output to log file while also displaying to stdout
exec > >(tee -a "$LOG_FILE") 2>&1

echo "=== Job Run Started ==="
echo "Job ID: $JOB_ID"
echo "Run ID: $RUN_ID"
echo "Job Name: $JOB_NAME"
echo "Started: $(date -Iseconds)"
echo ""

# Build the full prompt
FULL_PROMPT="You are a news retrieval agent. Your task is to search the web for news articles based on the user's request.

"
if [ -n "$SYSTEM_PROMPT" ]; then
    FULL_PROMPT+="$SYSTEM_PROMPT

"
fi

FULL_PROMPT+="USER REQUEST: $PROMPT

"

if [ -n "$KEYWORDS" ]; then
    FULL_PROMPT+="KEYWORDS TO FOCUS ON: $KEYWORDS

"
fi

if [ -n "$SOURCES" ]; then
    FULL_PROMPT+="PREFERRED SOURCES: $SOURCES

"
fi

if [ -n "$REGION" ]; then
    FULL_PROMPT+="GEOGRAPHIC FOCUS: $REGION

"
fi

FULL_PROMPT+='Please search the web for relevant news articles. For each article found, provide:
1. Title
2. URL  
3. Brief summary (2-3 sentences)

Format your response as a JSON array ONLY (no other text):
[{"title": "...", "url": "...", "summary": "..."}]

**IMPORTANT**: When using a subagent to search the web, always wait for it to fully complete its work before returning. Do not return until the subagent has finished and provided its full results.

Search the web now and return the results.'

# Run via Shelley API
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
JOB_ARTICLES_DIR="$ARTICLES_DIR/job_${JOB_ID}"
mkdir -p "$JOB_ARTICLES_DIR"

echo "Running job $JOB_ID via Shelley API..."

# Write prompt to temp file to avoid escaping issues
PROMPT_FILE=$(mktemp)
echo "$FULL_PROMPT" > "$PROMPT_FILE"

# Create JSON payload using jq with file input
# Specify model explicitly (claude-sonnet-4.5 is fast and capable for news retrieval)
PAYLOAD=$(jq -n --rawfile msg "$PROMPT_FILE" '{message: $msg, model: "claude-sonnet-4.5"}')
rm "$PROMPT_FILE"

# Check if there's an existing conversation we should resume
CONV_ID=""
if [ -n "$EXISTING_CONV" ]; then
    echo "Found existing conversation: $EXISTING_CONV, checking status..."
    EXISTING_DATA=$(curl -s -H "X-Exedev-Userid: news-job-$JOB_ID" "$SHELLEY_API/api/conversation/$EXISTING_CONV" 2>/dev/null || echo "")
    EXISTING_END=$(echo "$EXISTING_DATA" | jq -r '.messages | map(select(.type == "agent")) | last | .end_of_turn // false' 2>/dev/null || echo "false")
    
    if [ "$EXISTING_END" = "true" ]; then
        echo "Existing conversation already completed, will create new one"
    elif [ -n "$EXISTING_DATA" ] && echo "$EXISTING_DATA" | jq -e '.messages' >/dev/null 2>&1; then
        echo "Resuming existing conversation (agent still working)"
        CONV_ID="$EXISTING_CONV"
    else
        echo "Existing conversation not found or invalid, will create new one"
    fi
fi

# Create a new conversation if needed
if [ -z "$CONV_ID" ]; then
    CONV_RESPONSE=$(curl -s -X POST "$SHELLEY_API/api/conversations/new" \
        -H "Content-Type: application/json" \
        -H "X-Exedev-Userid: news-job-$JOB_ID" \
        -H "X-Shelley-Request: 1" \
        -d "$PAYLOAD")

    CONV_ID=$(echo "$CONV_RESPONSE" | jq -r '.conversation_id // empty')

    if [ -z "$CONV_ID" ]; then
        ERROR_MSG="Failed to create conversation: $CONV_RESPONSE"
        db "UPDATE job_runs SET status = 'failed', error_message = '${ERROR_MSG//\'/\'\'}', articles_saved = 0, completed_at = datetime('now') WHERE id = $RUN_ID;"
        db "UPDATE jobs SET status = 'failed', last_run_at = datetime('now') WHERE id = $JOB_ID;"
        
        if [ "$NOTIFY_FAILURE" = "1" ]; then
            send_discord_notification "❌ News job '$JOB_NAME' failed: $ERROR_MSG"
        fi
        exit 1
    fi

    echo "Created new conversation: $CONV_ID"
fi

# Store the conversation ID in the database
db "UPDATE jobs SET current_conversation_id = '$CONV_ID' WHERE id = $JOB_ID;"

# Poll for completion (wait for agent to finish with end_of_turn: true)
# Timeout can be set via NEWS_JOB_TIMEOUT environment variable (in seconds)
MAX_WAIT=${NEWS_JOB_TIMEOUT:-900}  # Default 15 minutes
WAITED=0
POLL_INTERVAL=10

while [ $WAITED -lt $MAX_WAIT ]; do
    sleep $POLL_INTERVAL
    WAITED=$((WAITED + POLL_INTERVAL))
    
    # Check if the last agent message has end_of_turn: true
    CONV_DATA=$(curl -s -H "X-Exedev-Userid: news-job-$JOB_ID" "$SHELLEY_API/api/conversation/$CONV_ID")
    END_OF_TURN=$(echo "$CONV_DATA" | jq -r '.messages | map(select(.type == "agent")) | last | .end_of_turn // false')
    
    if [ "$END_OF_TURN" = "true" ]; then
        echo "Agent finished after ${WAITED}s"
        break
    fi
    echo "Waiting... (${WAITED}s)"
done

if [ $WAITED -ge $MAX_WAIT ]; then
    ERROR_MSG="Job timed out after ${MAX_WAIT}s"
    echo "$ERROR_MSG"
    
    # Try to cleanup the stuck conversation
    if [ -n "$CONV_ID" ]; then
        echo "Attempting to cancel stuck conversation: $CONV_ID"
        curl -s -X DELETE "$SHELLEY_API/api/conversation/$CONV_ID" \
            -H "X-Exedev-Userid: news-job-$JOB_ID" > /dev/null 2>&1 || true
    fi
    
    # Clear conversation ID and mark as failed
    db "UPDATE job_runs SET status = 'failed', error_message = '${ERROR_MSG}', articles_saved = 0, completed_at = datetime('now') WHERE id = $RUN_ID;"
    db "UPDATE jobs SET status = 'failed', last_run_at = datetime('now'), current_conversation_id = '' WHERE id = $JOB_ID;"
    
    if [ "$NOTIFY_FAILURE" = "1" ]; then
        send_discord_notification "❌ News job '$JOB_NAME' timed out after ${MAX_WAIT}s"
    fi
    exit 1
fi

# Get the conversation messages
MESSAGES=$(curl -s -H "X-Exedev-Userid: news-job-$JOB_ID" "$SHELLEY_API/api/conversation/$CONV_ID")

# Extract the last agent message text
RESULT=$(echo "$MESSAGES" | jq -r '.messages | map(select(.type == "agent")) | last | .llm_data' | jq -r '.Content[] | select(.Type == 2) | .Text' 2>/dev/null || echo "")

# Try to extract JSON from the result and save articles
# Use Python to robustly extract and fix JSON (handles malformed quotes from LLM)
# Write result to temp file to avoid shell escaping issues with large text
RESULT_FILE=$(mktemp)
echo "$RESULT" > "$RESULT_FILE"

JSON_ARRAY=$(python3 - "$RESULT_FILE" << 'EXTRACT_JSON'
import sys
import json
import re

# Read from file to avoid shell argument length/escaping issues
with open(sys.argv[1], 'r') as f:
    text = f.read()

# Remove markdown code blocks
text = re.sub(r'^```json\s*', '', text)
text = re.sub(r'\s*```$', '', text)
text = text.strip()

# Find JSON array
match = re.search(r'\[.*\]', text, re.DOTALL)
if not match:
    sys.exit(1)

json_str = match.group(0)

# Try to parse as-is first
try:
    data = json.loads(json_str)
    print(json.dumps(data))
    sys.exit(0)
except json.JSONDecodeError:
    pass

# Fix common LLM JSON issues:
# 1. Unescaped quotes inside strings - replace straight quotes inside values with curly quotes
# This is a heuristic fix for Chinese text with embedded quotes
def fix_json_string(s):
    result = []
    in_string = False
    escape_next = False
    i = 0
    while i < len(s):
        c = s[i]
        if escape_next:
            result.append(c)
            escape_next = False
        elif c == '\\':
            result.append(c)
            escape_next = True
        elif c == '"':
            if not in_string:
                in_string = True
                result.append(c)
            else:
                # Check if this looks like end of string
                rest = s[i+1:i+20].lstrip()
                if rest.startswith((',', '}', ']', ':')) or rest == '':
                    in_string = False
                    result.append(c)
                else:
                    # Embedded quote - escape it
                    result.append('\\"')
        else:
            result.append(c)
        i += 1
    return ''.join(result)

try:
    fixed = fix_json_string(json_str)
    data = json.loads(fixed)
    print(json.dumps(data))
except Exception as e:
    print(f"Error: {e}", file=sys.stderr)
    sys.exit(1)
EXTRACT_JSON
)

# Clean up temp file
rm -f "$RESULT_FILE"

if [ -z "$JSON_ARRAY" ]; then
    echo "Warning: Could not extract valid JSON from agent response"
    JSON_ARRAY=""
fi

SAVED_COUNT=0
DUP_COUNT=0
if [ -n "$JSON_ARRAY" ]; then
    # Parse and save articles with full content
    while read -r article; do
        TITLE=$(echo "$article" | jq -r '.title // "Untitled"')
        URL=$(echo "$article" | jq -r '.url // ""')
        SUMMARY=$(echo "$article" | jq -r '.summary // ""')
        
        # Fetch full article content if URL is provided
        ARTICLE_CONTENT=""
        if [ -n "$URL" ] && [ "$URL" != "null" ]; then
            echo "Fetching content from: $URL"
            
            # Use readability-lxml to extract article text (Mozilla Readability algorithm)
            ARTICLE_CONTENT=$(/home/exedev/news-app/.venv/bin/python3 - "$URL" 2>/dev/null << 'PYTHON_SCRIPT'
import sys
import urllib.request
import re

try:
    from readability import Document
    from lxml.html import tostring, fromstring
except ImportError:
    # Fallback if readability not available
    print("[readability-lxml not installed]")
    sys.exit(0)

def extract_text(element):
    """Extract plain text from HTML element, preserving paragraph breaks."""
    texts = []
    for item in element.iter():
        if item.tag in ('p', 'div', 'article', 'section', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'li'):
            text = item.text_content().strip()
            if text and len(text) > 20:
                texts.append(text)
    return '\n\n'.join(texts)

try:
    url = sys.argv[1]
    req = urllib.request.Request(url, headers={
        'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36'
    })
    with urllib.request.urlopen(req, timeout=20) as response:
        html = response.read().decode('utf-8', errors='ignore')
    
    # Use readability to extract main content
    doc = Document(html)
    content_html = doc.summary()
    
    # Convert to plain text
    content_element = fromstring(content_html)
    content = extract_text(content_element)
    
    # Clean up whitespace
    content = re.sub(r'\n{3,}', '\n\n', content)
    content = re.sub(r' {2,}', ' ', content)
    content = content.strip()
    
    if content:
        print(content)
    else:
        print("[Content could not be extracted from this page]")
except Exception as e:
    print(f"[Error fetching article: {e}]")
PYTHON_SCRIPT
)
        fi
        
        # Get next article ID and create file
        ARTICLE_ID=$(db "SELECT COALESCE(MAX(id), 0) + 1 FROM articles;")
        ARTICLE_FILE="$JOB_ARTICLES_DIR/article_${ARTICLE_ID}_${TIMESTAMP}.txt"
        
        # Write article file with metadata header
        {
            echo "Title: $TITLE"
            echo "URL: ${URL:-none}"
            echo "Retrieved: $(date -Iseconds)"
            echo ""
            echo "--- Summary ---"
            echo "$SUMMARY"
            echo ""
            echo "--- Full Content ---"
            if [ -n "$ARTICLE_CONTENT" ]; then
                echo "$ARTICLE_CONTENT"
            else
                echo "(No URL provided or content not available)"
            fi
        } > "$ARTICLE_FILE"
        echo "Saved article to: $ARTICLE_FILE"
        
        # Use Python for safe parameterized SQL insertion
        INSERT_RESULT=$(python3 - "$DB_PATH" "$JOB_ID" "$USER_ID" "$TITLE" "$URL" "$SUMMARY" "$ARTICLE_FILE" << 'INSERT_ARTICLE'
import sys
import sqlite3

db_path, job_id, user_id, title, url, summary, content_path = sys.argv[1:8]

try:
    conn = sqlite3.connect(db_path, timeout=30)
    conn.execute("PRAGMA journal_mode=WAL")
    cursor = conn.cursor()
    
    # Check count before
    cursor.execute("SELECT COUNT(*) FROM articles WHERE user_id = ?", (user_id,))
    before = cursor.fetchone()[0]
    
    # Insert with parameterized query (safe from SQL injection)
    cursor.execute(
        "INSERT OR IGNORE INTO articles (job_id, user_id, title, url, summary, content_path, retrieved_at) VALUES (?, ?, ?, ?, ?, ?, datetime('now'))",
        (job_id, user_id, title, url, summary, content_path)
    )
    conn.commit()
    
    # Check count after
    cursor.execute("SELECT COUNT(*) FROM articles WHERE user_id = ?", (user_id,))
    after = cursor.fetchone()[0]
    
    conn.close()
    
    if after > before:
        print("inserted")
    else:
        print("duplicate")
except Exception as e:
    print(f"error:{e}", file=sys.stderr)
    sys.exit(1)
INSERT_ARTICLE
)

        if [ "$INSERT_RESULT" = "inserted" ]; then
            SAVED_COUNT=$((SAVED_COUNT + 1))
        elif [ "$INSERT_RESULT" = "duplicate" ]; then
            echo "Skipped duplicate: $TITLE"
            DUP_COUNT=$((DUP_COUNT + 1))
        fi
    done < <(echo "$JSON_ARRAY" | jq -c '.[]' 2>/dev/null)
    echo "$SAVED_COUNT new articles saved to database"
else
    echo "Warning: No JSON array found in response"
fi

# Update job status and clear conversation ID
NEXT_RUN=""
if [ "$IS_ONE_TIME" = "0" ]; then
    case "$FREQUENCY" in
        hourly)  NEXT_RUN="datetime('now', '+1 hour')" ;;
        6hours)  NEXT_RUN="datetime('now', '+6 hours')" ;;
        daily)   NEXT_RUN="datetime('now', '+1 day')" ;;
        weekly)  NEXT_RUN="datetime('now', '+7 days')" ;;
        *)       NEXT_RUN="datetime('now', '+1 day')" ;;
    esac
    db "UPDATE jobs SET status = 'completed', last_run_at = datetime('now'), next_run_at = $NEXT_RUN, current_conversation_id = '' WHERE id = $JOB_ID;"
else
    db "UPDATE jobs SET status = 'completed', last_run_at = datetime('now'), is_active = 0, current_conversation_id = '' WHERE id = $JOB_ID;"
fi

# Set run status based on whether new articles were saved
if [ "$SAVED_COUNT" -eq 0 ]; then
    RUN_STATUS="completed_no_new"
else
    RUN_STATUS="completed"
fi
db "UPDATE job_runs SET status = '$RUN_STATUS', articles_saved = $SAVED_COUNT, duplicates_skipped = $DUP_COUNT, completed_at = datetime('now') WHERE id = $RUN_ID;"

# Archive the conversation and its subagents to keep the list clean
if [ -n "$CONV_ID" ]; then
    echo "Archiving conversation: $CONV_ID"
    curl -s -X POST "$SHELLEY_API/api/conversation/$CONV_ID/archive" \
        -H "X-Exedev-Userid: news-job-$JOB_ID" \
        -H "X-Shelley-Request: 1" > /dev/null 2>&1 || true
    
    # Also archive any subagent conversations
    SUBAGENTS=$(curl -s -H "X-Exedev-Userid: news-job-$JOB_ID" \
        "$SHELLEY_API/api/conversations" | \
        jq -r ".conversations[] | select(.parent_conversation_id == \"$CONV_ID\") | .conversation_id" 2>/dev/null || echo "")
    
    for SUB_ID in $SUBAGENTS; do
        if [ -n "$SUB_ID" ]; then
            echo "Archiving subagent conversation: $SUB_ID"
            curl -s -X POST "$SHELLEY_API/api/conversation/$SUB_ID/archive" \
                -H "X-Exedev-Userid: news-job-$JOB_ID" \
                -H "X-Shelley-Request: 1" > /dev/null 2>&1 || true
        fi
    done
fi

# Send success notification
if [ "$NOTIFY_SUCCESS" = "1" ]; then
    if [ "$SAVED_COUNT" -eq 0 ]; then
        send_discord_notification "ℹ️ News job '$JOB_NAME' completed - no new articles found (all duplicates)"
    else
        send_discord_notification "✅ News job '$JOB_NAME' completed successfully! ($SAVED_COUNT new articles saved)"
    fi
fi

if [ "$SAVED_COUNT" -eq 0 ]; then
    echo "Job $JOB_ID completed - no new articles found (all duplicates)"
else
    echo "Job $JOB_ID completed successfully ($SAVED_COUNT new articles saved)"
fi
