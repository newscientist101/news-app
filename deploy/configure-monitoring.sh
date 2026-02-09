#!/bin/bash
# Configure database monitoring email alerts

set -e

if [[ $EUID -ne 0 ]]; then
    echo "This script must be run with sudo"
    exit 1
fi

echo "Database Monitoring Configuration"
echo "=================================="
echo

if [[ -z "$1" ]]; then
    echo "Usage: sudo $0 <your-email@example.com> [threshold_gb]"
    echo
    echo "Example:"
    echo "  sudo $0 user@example.com 5"
    echo
    echo "This will configure the monitoring service to send alerts to"
    echo "the specified email when the Shelley database exceeds the threshold."
    echo
    exit 1
fi

EMAIL="$1"
THRESHOLD="${2:-5}"

# Validate email format (basic)
if [[ ! "$EMAIL" =~ ^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$ ]]; then
    echo "Error: Invalid email format: $EMAIL"
    exit 1
fi

echo "Configuring monitoring with:"
echo "  Email: $EMAIL"
echo "  Threshold: ${THRESHOLD} GB"
echo

# Update the service file
SERVICE_FILE="/etc/systemd/system/news-db-monitor.service"

if [[ ! -f "$SERVICE_FILE" ]]; then
    echo "Error: Service file not found: $SERVICE_FILE"
    echo "Run ./deploy/setup-systemd.sh first to install the service."
    exit 1
fi

# Update email
sed -i "s|^Environment=\"ALERT_EMAIL=.*|Environment=\"ALERT_EMAIL=$EMAIL\"|" "$SERVICE_FILE"

# Update threshold
sed -i "s|^Environment=\"DB_THRESHOLD_GB=.*|Environment=\"DB_THRESHOLD_GB=$THRESHOLD\"|" "$SERVICE_FILE"

echo "✓ Updated $SERVICE_FILE"

# Reload systemd
systemctl daemon-reload
echo "✓ Reloaded systemd"

# Enable and start the timer if not already running
if ! systemctl is-enabled news-db-monitor.timer &>/dev/null; then
    systemctl enable news-db-monitor.timer
    echo "✓ Enabled news-db-monitor.timer"
fi

if ! systemctl is-active news-db-monitor.timer &>/dev/null; then
    systemctl start news-db-monitor.timer
    echo "✓ Started news-db-monitor.timer"
else
    systemctl restart news-db-monitor.timer
    echo "✓ Restarted news-db-monitor.timer"
fi

echo
echo "Configuration complete!"
echo
echo "The monitoring service will:"
echo "  - Check database size every 6 hours"
echo "  - Send email to $EMAIL if size exceeds ${THRESHOLD} GB"
echo "  - Only send one alert per 24 hours"
echo
echo "Next check scheduled for:"
systemctl list-timers news-db-monitor.timer --no-pager | tail -1
echo
echo "View logs with: journalctl -u news-db-monitor.service -f"
echo "Test now with:  sudo systemctl start news-db-monitor.service"
