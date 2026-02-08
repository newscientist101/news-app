#!/bin/bash
#
# News App - Systemd Setup Script
#
# This script installs and configures all systemd services for the news-app.
# Run with sudo or as root.
#
# Usage:
#   ./scripts/setup-systemd.sh [options]
#
# Options:
#   --user USER       User to run services as (default: current user)
#   --app-dir DIR     Application directory (default: script's parent dir)
#   --port PORT       Web server port (default: 8000)
#   --shelley-api URL Shelley API URL (default: http://localhost:9999)
#   --uninstall       Remove all services
#   --help            Show this help
#

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# Defaults
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_DIR="$(dirname "$SCRIPT_DIR")"
APP_USER="${SUDO_USER:-$(whoami)}"
APP_PORT="8000"
SHELLEY_API="http://localhost:9999"
UNINSTALL=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --user)
            APP_USER="$2"
            shift 2
            ;;
        --app-dir)
            APP_DIR="$2"
            shift 2
            ;;
        --port)
            APP_PORT="$2"
            shift 2
            ;;
        --shelley-api)
            SHELLEY_API="$2"
            shift 2
            ;;
        --uninstall)
            UNINSTALL=true
            shift
            ;;
        --help|-h)
            head -25 "$0" | tail -20
            exit 0
            ;;
        *)
            error "Unknown option: $1"
            ;;
    esac
done

# Check if running as root
if [[ $EUID -ne 0 ]]; then
    error "This script must be run as root (use sudo)"
fi

# Verify app directory
if [[ ! -f "$APP_DIR/news-app" ]]; then
    error "Binary not found: $APP_DIR/news-app\nBuild it first with: go build -o news-app ./cmd/srv/"
fi

# Uninstall if requested
if $UNINSTALL; then
    info "Uninstalling news-app services..."
    
    systemctl stop news-app 2>/dev/null || true
    systemctl disable news-app 2>/dev/null || true
    systemctl stop news-cleanup.timer 2>/dev/null || true
    systemctl disable news-cleanup.timer 2>/dev/null || true
    systemctl stop news-troubleshoot.timer 2>/dev/null || true
    systemctl disable news-troubleshoot.timer 2>/dev/null || true
    
    # Stop and remove job services
    for svc in /etc/systemd/system/news-job-*.service; do
        [[ -f "$svc" ]] || continue
        name=$(basename "$svc")
        systemctl stop "$name" 2>/dev/null || true
    done
    for timer in /etc/systemd/system/news-job-*.timer; do
        [[ -f "$timer" ]] || continue
        name=$(basename "$timer")
        systemctl stop "$name" 2>/dev/null || true
        systemctl disable "$name" 2>/dev/null || true
    done
    
    rm -f /etc/systemd/system/news-app.service
    rm -f /etc/systemd/system/news-cleanup.service
    rm -f /etc/systemd/system/news-cleanup.timer
    rm -f /etc/systemd/system/news-troubleshoot.service
    rm -f /etc/systemd/system/news-troubleshoot.timer
    rm -f /etc/systemd/system/news-job-*
    rm -f /etc/sudoers.d/news-app
    
    systemctl daemon-reload
    
    info "Uninstall complete"
    exit 0
fi

info "Setting up news-app services..."
info "  App directory: $APP_DIR"
info "  User: $APP_USER"
info "  Port: $APP_PORT"
info "  Shelley API: $SHELLEY_API"
echo

# Verify user exists
if ! id "$APP_USER" &>/dev/null; then
    error "User does not exist: $APP_USER"
fi

APP_HOME=$(eval echo ~$APP_USER)

# Create required directories
info "Creating directories..."
mkdir -p "$APP_DIR/logs/runs"
mkdir -p "$APP_DIR/articles"
chown -R "$APP_USER:$APP_USER" "$APP_DIR/logs" "$APP_DIR/articles"

# Install main service
info "Installing news-app.service..."
cat > /etc/systemd/system/news-app.service << EOF
[Unit]
Description=News Agent Web App
After=network.target

[Service]
Type=simple
User=$APP_USER
WorkingDirectory=$APP_DIR
ExecStart=$APP_DIR/news-app -listen :$APP_PORT
Restart=on-failure
RestartSec=5
Environment=PATH=/usr/local/bin:/usr/bin:/bin
Environment=NEWS_APP_SHELLEY_API=$SHELLEY_API
Environment=NEWS_APP_DB_PATH=$APP_DIR/db.sqlite3
Environment=NEWS_APP_ARTICLES_DIR=$APP_DIR/articles
Environment=NEWS_APP_LOGS_DIR=$APP_DIR/logs/runs

[Install]
WantedBy=multi-user.target
EOF

# Install cleanup service and timer
info "Installing news-cleanup.service..."
cat > /etc/systemd/system/news-cleanup.service << EOF
[Unit]
Description=Cleanup old news-app conversations
After=network.target

[Service]
Type=oneshot
ExecStart=$APP_DIR/news-app cleanup
User=$APP_USER
WorkingDirectory=$APP_DIR

[Install]
WantedBy=multi-user.target
EOF

cat > /etc/systemd/system/news-cleanup.timer << EOF
[Unit]
Description=Run news-app conversation cleanup every 48 hours

[Timer]
OnBootSec=1h
OnUnitActiveSec=48h
Persistent=true

[Install]
WantedBy=timers.target
EOF

# Install troubleshoot service and timer
info "Installing news-troubleshoot.service..."
cat > /etc/systemd/system/news-troubleshoot.service << EOF
[Unit]
Description=Troubleshoot news-app job runs
After=network.target

[Service]
Type=oneshot
ExecStart=$APP_DIR/news-app troubleshoot
User=$APP_USER
WorkingDirectory=$APP_DIR

[Install]
WantedBy=multi-user.target
EOF

cat > /etc/systemd/system/news-troubleshoot.timer << EOF
[Unit]
Description=Run news-app troubleshoot check daily

[Timer]
OnCalendar=*-*-* 07:00:00
Persistent=true
RandomizedDelaySec=300

[Install]
WantedBy=timers.target
EOF

# Configure sudoers for job management
info "Configuring sudoers..."
cat > /etc/sudoers.d/news-app << EOF
# Allow news-app user to manage job services without password
$APP_USER ALL=(ALL) NOPASSWD: /bin/systemctl start news-job-*
$APP_USER ALL=(ALL) NOPASSWD: /bin/systemctl stop news-job-*
$APP_USER ALL=(ALL) NOPASSWD: /bin/systemctl enable news-job-*
$APP_USER ALL=(ALL) NOPASSWD: /bin/systemctl disable news-job-*
$APP_USER ALL=(ALL) NOPASSWD: /bin/systemctl daemon-reload
$APP_USER ALL=(ALL) NOPASSWD: /bin/cp /tmp/systemd-*.tmp /etc/systemd/system/*
EOF
chmod 440 /etc/sudoers.d/news-app

# Validate sudoers file
if ! visudo -c -f /etc/sudoers.d/news-app &>/dev/null; then
    rm /etc/sudoers.d/news-app
    error "Invalid sudoers file generated"
fi

# Reload systemd
info "Reloading systemd..."
systemctl daemon-reload

# Enable and start services
info "Enabling services..."
systemctl enable news-app.service
systemctl enable news-cleanup.timer
systemctl enable news-troubleshoot.timer

info "Starting services..."
systemctl start news-app.service
systemctl start news-cleanup.timer
systemctl start news-troubleshoot.timer

echo
info "Setup complete!"
echo
echo "Services status:"
systemctl is-active news-app.service && echo "  news-app.service: running" || echo "  news-app.service: not running"
systemctl is-active news-cleanup.timer && echo "  news-cleanup.timer: active" || echo "  news-cleanup.timer: inactive"
systemctl is-active news-troubleshoot.timer && echo "  news-troubleshoot.timer: active" || echo "  news-troubleshoot.timer: inactive"

echo
echo "Useful commands:"
echo "  sudo systemctl status news-app        # Check main app status"
echo "  journalctl -u news-app -f             # Follow app logs"
echo "  systemctl list-timers 'news-*'        # List all timers"
echo
echo "Web app available at: http://localhost:$APP_PORT/"
