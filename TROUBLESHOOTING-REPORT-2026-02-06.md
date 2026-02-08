# News-App Job Troubleshooting Report
**Generated:** 2026-02-06 07:07 UTC

## Summary

✅ **Overall Status: HEALTHY**

All job runs in the last 24 hours completed successfully with no critical failures.

## Job Run Statistics (Last 24 hours)

| Metric | Count |
|--------|-------|
| Total Runs | 10 |
| Completed | 8 |
| Cancelled | 1 |
| Failed | 0 |
| Running | 0 |
| **Success Rate** | **100%** (all non-cancelled runs succeeded) |

**Average Completion Time:** 5.4 minutes (321 seconds)

## Recent Job Runs (Last 24h)

| Run ID | Job Name | Status | Duration | Articles | Notes |
|--------|----------|--------|----------|----------|-------|
| 147 | Test Redirect | ✅ completed | 114s | 8 | |
| 146 | curl test | ✅ completed | 91s | 2 | |
| 145 | Test Job | ✅ completed | 85s | 10 | |
| 144 | Browser Test Job | ✅ completed | 285s | 10 | |
| 143 | Test Job | ✅ completed | 425s | 5 | |
| 142 | Chinese LLMs | ✅ completed | 459s | 7 | |
| 141 | Metaverse | ✅ completed | 525s | 7 | |
| 140 | Metaverse | ✅ completed | 364s | 1 | |
| 139 | Metaverse | ⚠️ cancelled | 542s | 0 | New run started (expected behavior) |

## Issues Found

### 1. ⚠️ Unarchived Subagent Conversations (Minor Issue)

**Issue:** 6 subagent conversations from completed jobs were not archived.

**Details:**
- These are child conversations (web-search, news-search subagents)
- Parent conversations were archived correctly
- All conversations completed successfully (end_of_turn: true)
- This doesn't affect functionality but clutters the conversation list

**Affected Conversations:**
```
cZIT43N - news-search-6 (parent: cTZJCIM)
cFGYBMD - web-search-12 (parent: cBOQ5OU)
c4PRI6O - tech-news-search-10 (parent: c7CBFKZ)
cLGLZL7 - web-search-13 (parent: cHPV7CT)
cZWFT3R - news-search-5 (parent: c4YVOCU)
cQZRROE - web-search-11 (parent: c6Z77B2)
```

**Root Cause:** The archiving logic in run-job.sh (line 308) only archives the main conversation, not its subagent children.

**Impact:** Low - causes clutter in conversation list but doesn't affect job functionality

### 2. ℹ️ Orphaned Systemd Service Units (Informational)

**Issue:** Several systemd service units show "failed" status from old runs (2+ weeks ago):
- news-job-17, 18, 19, 21, 22, 23, 24

**Details:**
- These jobs no longer exist in the database
- Failed due to SIGTERM (likely systemd timeout)
- Last activity: January 21, 2026

**Impact:** None - these are stale unit files from deleted jobs

### 3. ✅ No Critical Issues Found

- ✅ No timeouts in the last 24 hours
- ✅ No JSON parsing errors
- ✅ No network failures
- ✅ No database lock issues
- ✅ All conversations created successfully
- ✅ All jobs cleared their conversation IDs properly
- ✅ Duplicate detection working correctly

## Performance Observations

### Completion Times by Job Type
- Fast jobs (< 2 min): 85-114s - Test jobs, simple searches
- Medium jobs (2-5 min): 234-285s - Browser tests, multi-source searches
- Slow jobs (5-9 min): 364-599s - Complex searches (Chinese LLMs, Metaverse)

**No outliers or timeout concerns** - all completed well within the 1500s (25 min) timeout.

## Recommendations

### 1. Archive Subagent Conversations (Priority: Low)

**Fix:** Modify run-job.sh to recursively archive subagent conversations.

Add after line 308 in run-job.sh:

```bash
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
        echo "Archiving subagent conversation: $SUB_ID"
        curl -s -X POST "$SHELLEY_API/api/conversation/$SUB_ID/archive" \
            -H "X-Exedev-Userid: news-job-$JOB_ID" \
            -H "X-Shelley-Request: 1" > /dev/null 2>&1 || true
    done
fi
```

### 2. Clean Up Orphaned Systemd Units (Priority: Low)

Reset failed systemd units that no longer have corresponding jobs:

```bash
sudo systemctl reset-failed news-job-17.service
sudo systemctl reset-failed news-job-18.service
sudo systemctl reset-failed news-job-19.service
sudo systemctl reset-failed news-job-21.service
sudo systemctl reset-failed news-job-22.service
sudo systemctl reset-failed news-job-23.service
sudo systemctl reset-failed news-job-24.service
```

Or clean them all at once:
```bash
for id in 17 18 19 21 22 23 24; do
    sudo systemctl reset-failed news-job-$id.service 2>/dev/null || true
done
```

### 3. Add Monitoring Dashboard (Priority: Medium)

Consider adding a job health dashboard to the web UI showing:
- Recent run success rate
- Average completion times
- Failed/stuck runs
- Unarchived conversations count

## Conclusion

**The news-app job system is operating normally.** All recent runs completed successfully with reasonable performance. The only issues found are minor housekeeping items (unarchived subagent conversations and stale systemd units) that don't affect functionality.

No systematic failures, timeouts, or critical bugs detected.
