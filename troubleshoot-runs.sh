#!/bin/bash
# Troubleshoot failed/problematic job runs
# Runs daily to identify issues and initiate Shelley troubleshooting

set -e

DB_PATH="/home/exedev/news-app/db.sqlite3"
SHELLEY_API="http://localhost:9999"
LOG_FILE="/home/exedev/news-app/logs/troubleshoot.log"

# Ensure log directory exists
mkdir -p "$(dirname "$LOG_FILE")"

log() {
    echo "[$(date -Iseconds)] $1" | tee -a "$LOG_FILE"
}

db() {
    sqlite3 -cmd ".timeout 30000" "$DB_PATH" "$@"
}

log "Starting troubleshoot run check..."

# Find problematic runs from the last 24 hours:
# - Failed runs
# - Cancelled runs (excluding "Cancelled: new run started" which is normal)
# - Completed runs with 0 or 1 articles
PROBLEM_RUNS=$(db "
SELECT 
    jr.id,
    jr.job_id,
    j.name,
    jr.status,
    jr.error_message,
    jr.started_at,
    jr.completed_at,
    (SELECT COUNT(*) FROM articles a 
     WHERE a.job_id = jr.job_id 
     AND a.retrieved_at >= jr.started_at 
     AND (jr.completed_at IS NULL OR a.retrieved_at <= jr.completed_at)) as article_count
FROM job_runs jr
JOIN jobs j ON jr.job_id = j.id
WHERE jr.started_at >= datetime('now', '-24 hours')
  AND j.is_active = 1
  AND (
    jr.status = 'failed'
    OR (jr.status = 'cancelled' AND jr.error_message NOT LIKE '%new run started%' AND jr.error_message NOT LIKE '%Debug%')
    OR (jr.status = 'completed' AND (SELECT COUNT(*) FROM articles a 
        WHERE a.job_id = jr.job_id 
        AND a.retrieved_at >= jr.started_at 
        AND (jr.completed_at IS NULL OR a.retrieved_at <= jr.completed_at)) = 0)
  )
  AND jr.status != 'completed_no_new'
ORDER BY jr.started_at DESC;
")

if [ -z "$PROBLEM_RUNS" ]; then
    log "No problematic runs found in the last 24 hours."
    exit 0
fi

log "Found problematic runs:"
echo "$PROBLEM_RUNS" | while IFS='|' read -r run_id job_id job_name status error_msg started_at completed_at article_count; do
    log "  Run $run_id (Job $job_id '$job_name'): status=$status, articles=$article_count, error='$error_msg'"
done

# Build a troubleshooting prompt
PROMPT="I need you to troubleshoot issues with the news-app job runs. Here are the problematic runs from the last 24 hours:

"

echo "$PROBLEM_RUNS" | while IFS='|' read -r run_id job_id job_name status error_msg started_at completed_at article_count; do
    PROMPT+="- Run ID $run_id (Job '$job_name', ID $job_id):
  Status: $status
  Started: $started_at
  Completed: $completed_at
  Articles retrieved: $article_count
  Error: ${error_msg:-none}

"
done

PROMPT+="
Please investigate:
1. Check the systemd journal logs for these job runs: journalctl -u news-job-{job_id}.service --since '{started_at}'
2. Check if the conversations were created and completed properly
3. Look for patterns in failures (timeouts, JSON parsing, network issues)
4. Suggest fixes if you find systematic issues

Key files:
- /home/exedev/news-app/run-job.sh - The job runner script
- /home/exedev/news-app/db.sqlite3 - The database
- Shelley API at http://localhost:9999

Start by examining the logs for the most recent failed run."

# Create troubleshooting conversation
log "Creating troubleshooting conversation with Shelley..."

PROMPT_FILE=$(mktemp)
echo "$PROMPT" > "$PROMPT_FILE"

PAYLOAD=$(jq -n --rawfile msg "$PROMPT_FILE" '{message: $msg, model: "claude-sonnet-4.5"}')
rm "$PROMPT_FILE"

RESPONSE=$(curl -s -X POST "$SHELLEY_API/api/conversations/new" \
    -H "Content-Type: application/json" \
    -H "X-Exedev-Userid: news-app-troubleshoot" \
    -H "X-Shelley-Request: 1" \
    -d "$PAYLOAD")

CONV_ID=$(echo "$RESPONSE" | jq -r '.conversation_id // empty')

if [ -z "$CONV_ID" ]; then
    log "ERROR: Failed to create conversation: $RESPONSE"
    exit 1
fi

log "Created troubleshooting conversation: $CONV_ID"
log "View at: https://anchor-asteroid.exe.xyz:9999/c/$CONV_ID"

# Store the conversation ID for reference
db "INSERT INTO job_runs (job_id, status, started_at, error_message) 
    VALUES (0, 'troubleshoot', datetime('now'), 'Troubleshoot conversation: $CONV_ID');" 2>/dev/null || true

log "Troubleshoot check completed."
