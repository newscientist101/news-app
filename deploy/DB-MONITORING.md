# Database Size Monitoring

The news-app includes a database size monitoring service that alerts you when the Shelley database grows too large.

## Overview

The monitoring service:
- Checks the Shelley database size every 6 hours
- Sends an email alert if the database exceeds 5 GB
- Only sends one alert per 24 hours (to avoid spam)
- Includes detailed table statistics in the alert

## Configuration

### Set Your Email Address

> **Note:** The exe.dev email service can only send emails to the VM owner's email address (the address associated with your exe.dev account). You cannot send to arbitrary email addresses.

The service needs your email address to send alerts. Edit the systemd service file:

```bash
sudo nano /etc/systemd/system/news-db-monitor.service
```

Change this line:
```
Environment="ALERT_EMAIL="
```

To:
```
Environment="ALERT_EMAIL=your-email@example.com"
```

Then reload and restart:
```bash
sudo systemctl daemon-reload
sudo systemctl restart news-db-monitor.timer
```

### Customize Threshold

To change the 5 GB threshold, edit the same file and change:
```
Environment="DB_THRESHOLD_GB=5"
```

To your desired value (in gigabytes).

## Manual Testing

Test the monitoring script manually:

```bash
# Check current size (no email)
./deploy/check-db-size.sh

# Send test email with current database size
ALERT_EMAIL=your-email@example.com ./deploy/check-db-size.sh --test

# Test with custom threshold (will trigger alert if over 0.5 GB)
DB_THRESHOLD_GB=0.5 ALERT_EMAIL=your-email@example.com ./deploy/check-db-size.sh
```

### Test Email

The `--test` flag sends an informational email with:
- Current database size
- Threshold setting
- Percentage of threshold used
- Table breakdown (llm_requests stats)
- Current status (over/under threshold)

This is useful for:
- Verifying email configuration works
- Checking current database size
- Testing the monitoring setup
- Doesn't trigger the "alert sent" state, so won't affect normal alerts

## Systemd Service

The monitoring runs as a systemd timer:

```bash
# Check service status
systemctl status news-db-monitor.timer
systemctl status news-db-monitor.service

# View logs
journalctl -u news-db-monitor.service -f

# List next scheduled run
systemctl list-timers news-db-monitor.timer

# Trigger a manual check
sudo systemctl start news-db-monitor.service

# Send a test email (as the exedev user)
ALERT_EMAIL=your-email@example.com /home/exedev/news-app/deploy/check-db-size.sh --test
```

## Alert Email

When the database exceeds the threshold, you'll receive an email with:

- Current database size
- Threshold value
- Table breakdown (llm_requests row count and size)
- Recommended actions
- VM hostname and timestamp

## What to Do When Alerted

1. **Check disk space:**
   ```bash
   df -h /home
   ```

2. **Check database size:**
   ```bash
   ls -lh ~/.config/shelley/shelley.db
   ```

3. **Review table sizes:**
   ```bash
   sqlite3 ~/.config/shelley/shelley.db "
     SELECT COUNT(*) as rows, 
            printf('%.2f MB', CAST(SUM(LENGTH(COALESCE(request_body, '')) + 
                   LENGTH(COALESCE(response_body, ''))) AS REAL) / 1024.0 / 1024.0) as size 
     FROM llm_requests;"
   ```

4. **Review mitigation options** in [TROUBLESHOOTING.md](../docs/TROUBLESHOOTING.md#shelley-database-filling-up-storage)

5. **Consider:**
   - Reducing job frequency
   - Running manual cleanup
   - Vacuuming the database
   - Archiving old data

## Rate Limiting

The exe.dev email service is rate-limited. If you accidentally flood yourself with alerts:
- The service will only send one alert per 24 hours
- If rate-limited by exe.dev, wait a few hours before testing again

## Disabling Monitoring

If you don't want database monitoring:

```bash
sudo systemctl stop news-db-monitor.timer
sudo systemctl disable news-db-monitor.timer
```

## Files

- `deploy/check-db-size.sh` - Monitoring script
- `deploy/news-db-monitor.service` - Systemd service
- `deploy/news-db-monitor.timer` - Systemd timer
- `~/.config/news-app/db-monitor-state` - Tracks last alert time
