# Troubleshooting Guide

This guide covers common issues and their solutions.

## Quick Diagnostics

```bash
# Check if services are running
systemctl status news-app
systemctl list-timers 'news-*'

# View recent logs
journalctl -u news-app --since "1 hour ago"
journalctl -u 'news-job-*' --since today

# Run automated troubleshooting
./news-app troubleshoot --dry-run
```

---

## Service Issues

### news-app service won't start

**Symptoms:** Service fails to start, returns error on `systemctl start news-app`

**Diagnostics:**
```bash
sudo systemctl status news-app
journalctl -u news-app -n 50
```

**Common causes:**

1. **Binary not found or not executable**
   ```bash
   ls -la /home/exedev/news-app/news-app
   # If missing, rebuild:
   cd /home/exedev/news-app && make build
   ```

2. **Port already in use**
   ```bash
   ss -tlnp | grep 8000
   # Kill the conflicting process or use a different port
   ```

3. **Database locked**
   ```bash
   # Check for other processes using the database
   fuser db.sqlite3
   # Remove stale lock files
   rm -f db.sqlite3-shm db.sqlite3-wal
   ```

4. **Permission issues**
   ```bash
   # Ensure correct ownership
   sudo chown -R exedev:exedev /home/exedev/news-app
   ```

---

### Jobs not running on schedule

**Symptoms:** Timer is active but jobs don't execute

**Diagnostics:**
```bash
systemctl list-timers 'news-job-*'
systemctl status news-job-{id}.timer
journalctl -u news-job-{id}.service
```

**Common causes:**

1. **Timer not enabled**
   ```bash
   sudo systemctl enable news-job-{id}.timer
   sudo systemctl start news-job-{id}.timer
   ```

2. **Service file missing or invalid**
   ```bash
   cat /etc/systemd/system/news-job-{id}.service
   # Recreate by toggling job active status in UI
   ```

3. **Sudoers not configured**
   ```bash
   sudo cat /etc/sudoers.d/news-app
   # Re-run setup if missing:
   sudo ./deploy/setup-systemd.sh
   ```

---

## Job Execution Issues

### Job times out

**Symptoms:** Job runs for 25+ minutes then fails with timeout

**Diagnostics:**
```bash
journalctl -u news-job-{id} --since "30 minutes ago"
./news-app troubleshoot --dry-run
```

**Common causes:**

1. **Shelley API unresponsive**
   ```bash
   curl http://localhost:9999/health
   # If not responding, check Shelley service
   ```

2. **Complex prompt causing long processing**
   - Simplify the job prompt
   - Reduce the number of requested articles

3. **Network issues preventing article fetches**
   ```bash
   # Test connectivity
   curl -I https://example.com
   ```

---

### No articles saved

**Symptoms:** Job completes successfully but no articles appear

**Diagnostics:**
```bash
# Check job run log
cat logs/runs/run_{id}_*.log | tail -50

# Check for JSON extraction issues
grep -i "json\|extract" logs/runs/run_{id}_*.log
```

**Common causes:**

1. **Agent didn't return valid JSON**
   - Check the run log for the raw response
   - The agent may have returned text instead of JSON array
   - Try adjusting the prompt to be more explicit about JSON format

2. **All articles were duplicates**
   - Check `duplicates_skipped` in the job run
   - This is normal if the job runs frequently

3. **JSON parsing failed**
   - Check logs for "malformed JSON" messages
   - The jobrunner attempts to fix common issues but may fail on severely malformed responses

---

### Article content is empty

**Symptoms:** Articles saved but content is empty or minimal

**Diagnostics:**
```bash
# Check article file
cat articles/job_{id}/article_{id}_*.txt

# Check for fetch errors in run log
grep -i "fetch\|error\|failed" logs/runs/run_{id}_*.log
```

**Common causes:**

1. **Paywalled content**
   - The URL requires a subscription
   - Nothing can be done for these sources

2. **Bot protection**
   - Site blocks automated requests
   - Try adding the source to job's excluded sources

3. **JavaScript-rendered content**
   - Content loaded dynamically via JS
   - go-readability can't execute JavaScript

4. **Unusual HTML structure**
   - go-readability may not recognize the content
   - These are usually edge cases

---

## Authentication Issues

### 401 Unauthorized errors

**Symptoms:** All pages return 401 or redirect to login

**Diagnostics:**
```bash
# Check if headers are present
curl -v https://your-vm.exe.xyz:8000/ 2>&1 | grep -i x-exedev
```

**Common causes:**

1. **Accessing directly instead of via exe.dev proxy**
   - Use `https://your-vm.exe.xyz:8000/` not `http://localhost:8000/`
   - The exe.dev proxy adds authentication headers

2. **Not logged in to exe.dev**
   - Log in at exe.dev first

3. **For local development**
   ```bash
   # Use mitmdump to inject headers
   mitmdump -p 3000 --mode reverse:http://localhost:8000 \
     --modify-headers '/~q/X-ExeDev-UserID/1' \
     --modify-headers '/~q/X-ExeDev-Email/test@example.com'
   ```

---

## Database Issues

### Database locked errors

**Symptoms:** Operations fail with "database is locked"

**Diagnostics:**
```bash
# Check what's using the database
fuser db.sqlite3
lsof db.sqlite3
```

**Solutions:**

1. **Wait for other operations to complete**
   - SQLite has a 5-second busy timeout
   - High-frequency jobs may cause contention

2. **Kill stuck processes**
   ```bash
   # Find and kill stuck job processes
   ps aux | grep 'news-app run-job'
   kill {pid}
   ```

3. **Clear WAL files (last resort)**
   ```bash
   sudo systemctl stop news-app
   sqlite3 db.sqlite3 "PRAGMA wal_checkpoint(TRUNCATE);"
   sudo systemctl start news-app
   ```

---

### Database corruption

**Symptoms:** Strange errors, missing data, integrity check fails

**Diagnostics:**
```bash
sqlite3 db.sqlite3 "PRAGMA integrity_check;"
```

**Solutions:**

1. **Restore from backup** (if available)

2. **Attempt recovery**
   ```bash
   sqlite3 db.sqlite3 ".recover" | sqlite3 db_recovered.sqlite3
   mv db.sqlite3 db.sqlite3.corrupt
   mv db_recovered.sqlite3 db.sqlite3
   ```

---

## Storage Issues

### Shelley database filling up storage

**Symptoms:** VM storage is full or nearly full, `~/.config/shelley/shelley.db` is very large

> ⚠️ **This is a known bug.** When the Shelley service processes LLM requests, the raw request/response data is stored in the `llm_requests` table in the Shelley database and is **not automatically cleaned up**. The `news-app cleanup` command only removes the parsed conversation records from the `conversations` and `messages` tables, **not the underlying raw LLM data in `llm_requests`**. This means storage will continue to grow over time even with cleanup running.

**Diagnostics:**
```bash
# Check Shelley database size
ls -lh ~/.config/shelley/shelley.db

# Check disk usage
df -h /home

# Check database table sizes
sqlite3 ~/.config/shelley/shelley.db ".tables"
sqlite3 ~/.config/shelley/shelley.db "SELECT name, SUM(pgsize) as size FROM dbstat GROUP BY name ORDER BY size DESC;"

# Check llm_requests table size specifically
sqlite3 ~/.config/shelley/shelley.db "SELECT COUNT(*) as rows, printf('%.2f MB', CAST(SUM(LENGTH(COALESCE(request_body, '')) + LENGTH(COALESCE(response_body, ''))) AS REAL) / 1024.0 / 1024.0) as data_size FROM llm_requests;"
```

**Mitigation (partial):**

The `news-app cleanup` command helps reduce growth by removing conversation records, but does not fully solve the problem:

```bash
# Run cleanup to remove old conversation records
./news-app cleanup

# Vacuum to reclaim some space
sqlite3 ~/.config/shelley/shelley.db "VACUUM;"
```

**Workarounds:**

1. **Monitor disk usage regularly**
   ```bash
   df -h /home
   ls -lh ~/.config/shelley/shelley.db
   ```

2. **Reduce job frequency** to slow the growth rate

3. **Manual database cleanup** (use with caution)
   ```bash
   # Stop all services first
   sudo systemctl stop news-app
   sudo systemctl stop shelley  # if applicable
   
   # Back up the database
   cp ~/.config/shelley/shelley.db ~/.config/shelley/shelley.db.backup
   
   # Vacuum to reclaim space
   sqlite3 ~/.config/shelley/shelley.db "VACUUM;"
   
   # Restart services
   sudo systemctl start news-app
   ```

4. **Reset Shelley database** (last resort - loses all conversation history)
   ```bash
   sudo systemctl stop news-app
   sudo systemctl stop shelley  # if applicable
   
   mv ~/.config/shelley/shelley.db ~/.config/shelley/shelley.db.old
   
   sudo systemctl start shelley
   sudo systemctl start news-app
   ```

**Long-term:**
- This is a bug in the Shelley service that needs to be fixed upstream
- Monitor for updates to Shelley that address this issue

---

## Shelley API Issues

### Cannot connect to Shelley API

**Symptoms:** Jobs fail immediately with connection errors

**Diagnostics:**
```bash
curl http://localhost:9999/health
systemctl status shelley  # if applicable
```

**Solutions:**

1. **Check Shelley is running**
   - On exe.dev VMs, Shelley should be running by default

2. **Check the configured URL**
   ```bash
   echo $NEWS_APP_SHELLEY_API
   # Default: http://localhost:9999
   ```

---

### Shelley conversations not cleaning up

**Symptoms:** Shelley database grows very large

**Diagnostics:**
```bash
ls -la ~/.config/shelley/shelley.db
./news-app cleanup --dry-run
```

**Solutions:**

1. **Run cleanup manually**
   ```bash
   ./news-app cleanup
   ```

2. **Check cleanup timer is running**
   ```bash
   systemctl status news-cleanup.timer
   sudo systemctl start news-cleanup.timer
   ```

---

## Getting Help

### Collect diagnostic information

When reporting issues, include:

```bash
# System info
uname -a
go version

# Service status
systemctl status news-app
systemctl list-timers 'news-*'

# Recent logs
journalctl -u news-app --since "1 hour ago" > app.log
journalctl -u 'news-job-*' --since today > jobs.log

# Run troubleshooter
./news-app troubleshoot --dry-run
```

### Automated troubleshooting

The app includes an automated troubleshooter that runs daily:

```bash
# See what issues would be reported
./news-app troubleshoot --dry-run

# Create a troubleshooting conversation with Shelley
./news-app troubleshoot

# Reports are saved to:
ls logs/troubleshoot/
```
