#!/bin/bash
# Cleanup old news-app conversations (older than 48 hours)
# Only deletes conversations created by run-job.sh (cwd IS NULL)

set -e

SHELLEY_DB="/home/exedev/.config/shelley/shelley.db"
SHELLEY_API="http://localhost:9999"
MAX_AGE_HOURS=48

if [ ! -f "$SHELLEY_DB" ]; then
    echo "Shelley database not found: $SHELLEY_DB"
    exit 1
fi

echo "Cleaning up news-app conversations older than ${MAX_AGE_HOURS} hours..."

# Find old conversations that were created by run-job.sh:
# - cwd IS NULL (API-created, not interactive sessions)
# - older than MAX_AGE_HOURS
# Get parent conversations first
OLD_PARENTS=$(sqlite3 "$SHELLEY_DB" "
    SELECT conversation_id 
    FROM conversations 
    WHERE cwd IS NULL 
    AND parent_conversation_id IS NULL
    AND created_at < datetime('now', '-${MAX_AGE_HOURS} hours')
    ORDER BY created_at ASC;
")

DELETED=0
FAILED=0

delete_conversation() {
    local CONV_ID="$1"
    local RESULT
    
    # First, find and delete any child conversations (subagents)
    local CHILDREN=$(sqlite3 "$SHELLEY_DB" "
        SELECT conversation_id FROM conversations 
        WHERE parent_conversation_id = '$CONV_ID';
    ")
    
    for CHILD_ID in $CHILDREN; do
        if [ -n "$CHILD_ID" ]; then
            echo "  Deleting child conversation $CHILD_ID..."
            delete_conversation "$CHILD_ID"
        fi
    done
    
    # Now delete this conversation
    RESULT=$(curl -s -X POST \
        -H "X-Exedev-Userid: cleanup" \
        -H "X-Shelley-Request: 1" \
        "$SHELLEY_API/api/conversation/$CONV_ID/delete" 2>/dev/null || echo "error")
    
    if echo "$RESULT" | grep -q '"deleted"'; then
        DELETED=$((DELETED + 1))
        return 0
    else
        echo "  Warning: Delete failed for $CONV_ID: $RESULT"
        FAILED=$((FAILED + 1))
        return 1
    fi
}

for CONV_ID in $OLD_PARENTS; do
    if [ -n "$CONV_ID" ]; then
        echo "Deleting conversation $CONV_ID..."
        delete_conversation "$CONV_ID" || true
    fi
done

TOTAL=$(echo "$OLD_PARENTS" | grep -c . 2>/dev/null || echo 0)
echo "Done. Found $TOTAL old parent conversations, deleted $DELETED total (including children), failed $FAILED."
