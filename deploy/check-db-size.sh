#!/bin/bash
# Database size monitoring script
# Checks if Shelley database exceeds threshold and sends alert email
# Usage:
#   check-db-size.sh           # Normal monitoring
#   check-db-size.sh --test    # Send test email with current size

set -euo pipefail

DB_PATH="${SHELLEY_DB:-$HOME/.config/shelley/shelley.db}"
THRESHOLD_GB="${DB_THRESHOLD_GB:-5}"
EMAIL="${ALERT_EMAIL:-}"
STATE_FILE="${STATE_FILE:-$HOME/.config/news-app/db-monitor-state}"
TEST_MODE=false

# Parse arguments
if [[ "${1:-}" == "--test" ]]; then
    TEST_MODE=true
    echo "Running in TEST mode - will send email regardless of threshold"
fi

# Ensure state directory exists
mkdir -p "$(dirname "$STATE_FILE")"

# Check if database exists
if [[ ! -f "$DB_PATH" ]]; then
    echo "Database not found: $DB_PATH"
    exit 0
fi

# Get database size in bytes
DB_SIZE_BYTES=$(stat -c%s "$DB_PATH")
DB_SIZE_GB=$(echo "scale=2; $DB_SIZE_BYTES / 1024 / 1024 / 1024" | bc)
THRESHOLD_BYTES=$(echo "$THRESHOLD_GB * 1024 * 1024 * 1024" | bc | cut -d'.' -f1)

echo "Database: $DB_PATH"
echo "Current size: $DB_SIZE_GB GB ($DB_SIZE_BYTES bytes)"
echo "Threshold: $THRESHOLD_GB GB ($THRESHOLD_BYTES bytes)"

# Function to send email
send_email() {
    local subject="$1"
    local body="$2"
    local is_alert="$3"
    
    if [[ -z "$EMAIL" ]]; then
        echo "No email configured (set ALERT_EMAIL environment variable)"
        return 1
    fi
    
    echo "Sending email to $EMAIL"
    echo "Subject: $subject"
    
    RESPONSE=$(curl -s -X POST http://169.254.169.254/gateway/email/send \
        -H "Content-Type: application/json" \
        -d "{
            \"to\": \"$EMAIL\",
            \"subject\": \"$subject\",
            \"body\": $(echo "$body" | jq -Rs .)
        }" 2>&1)
    
    echo "Email response: $RESPONSE"
    
    if [[ "$is_alert" == "true" ]]; then
        # Record that we sent an alert
        date +%s > "$STATE_FILE"
    fi
    
    return 0
}

# Test mode - send email with current stats
if $TEST_MODE; then
    echo "📧 Sending test email..."
    
    # Get table breakdown
    TABLE_STATS=$(sqlite3 "$DB_PATH" "
        SELECT 
            'llm_requests: ' || COUNT(*) || ' rows, ' || 
            printf('%.2f MB', CAST(SUM(LENGTH(COALESCE(request_body, '')) + LENGTH(COALESCE(response_body, ''))) AS REAL) / 1024.0 / 1024.0)
        FROM llm_requests
    " 2>/dev/null || echo "Unable to query table stats")
    
    PERCENT_OF_THRESHOLD=$(echo "scale=1; ($DB_SIZE_GB / $THRESHOLD_GB) * 100" | bc)
    
    BODY="TEST EMAIL - Shelley Database Size Report

This is a test email from the database monitoring service.

Database: $DB_PATH
Current size: $DB_SIZE_GB GB ($DB_SIZE_BYTES bytes)
Threshold: $THRESHOLD_GB GB
Usage: ${PERCENT_OF_THRESHOLD}% of threshold

Table breakdown:
$TABLE_STATS

Status: $(if [[ $DB_SIZE_BYTES -gt $THRESHOLD_BYTES ]]; then echo "⚠️  OVER THRESHOLD"; else echo "✓ Under threshold"; fi)

VM: $(hostname)
Time: $(date)

This is a test message. Normal monitoring will only send alerts when the database exceeds the threshold."
    
    if send_email "✅ Test: Shelley DB Size ($DB_SIZE_GB GB / $THRESHOLD_GB GB)" "$BODY" "false"; then
        echo "✓ Test email sent successfully"
        exit 0
    else
        echo "✗ Failed to send test email"
        exit 1
    fi
fi

# Check if over threshold
if [[ $DB_SIZE_BYTES -gt $THRESHOLD_BYTES ]]; then
    echo "⚠️  Database size exceeds threshold!"
    
    # Check if we've already sent an alert (to avoid spam)
    if [[ -f "$STATE_FILE" ]]; then
        LAST_ALERT=$(cat "$STATE_FILE")
        CURRENT_TIME=$(date +%s)
        # Only send alert once per 24 hours
        if [[ $((CURRENT_TIME - LAST_ALERT)) -lt 86400 ]]; then
            echo "Alert already sent in last 24 hours, skipping"
            exit 0
        fi
    fi
    
    # Get table breakdown
    TABLE_STATS=$(sqlite3 "$DB_PATH" "
        SELECT 
            'llm_requests: ' || COUNT(*) || ' rows, ' || 
            printf('%.2f MB', CAST(SUM(LENGTH(COALESCE(request_body, '')) + LENGTH(COALESCE(response_body, ''))) AS REAL) / 1024.0 / 1024.0)
        FROM llm_requests
    " 2>/dev/null || echo "Unable to query table stats")
    
    BODY="WARNING: Shelley database has exceeded ${THRESHOLD_GB}GB threshold

Database: $DB_PATH
Current size: $DB_SIZE_GB GB
Threshold: $THRESHOLD_GB GB

$TABLE_STATS

This database stores raw LLM request/response data that is not automatically cleaned up.

Recommended actions:
1. Review database contents and clean up old data
2. Consider reducing news job frequency
3. See troubleshooting docs for mitigation steps

VM: $(hostname)
Time: $(date)"
    
    # Send email alert
    if send_email "⚠️  Shelley Database Over ${THRESHOLD_GB}GB" "$BODY" "true"; then
        # Exit with error code to trigger systemd failure state
        exit 1
    else
        exit 1
    fi
else
    echo "✓ Database size OK"
    # Clear alert state if size is back under threshold
    rm -f "$STATE_FILE"
    exit 0
fi
