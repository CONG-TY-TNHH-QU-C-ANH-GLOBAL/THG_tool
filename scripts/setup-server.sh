#!/bin/bash
# ============================================
# THG Agentic Scraper — One-Command Server Setup
# For Oracle Cloud Always Free (Ubuntu 22.04 ARM)
# ============================================
set -euo pipefail

echo "🕷️  THG Agentic Scraper — Server Setup"
echo "======================================="

# --- 1. System Update ---
echo "📦 Updating system packages..."
sudo apt-get update -qq
sudo apt-get upgrade -y -qq

# --- 2. Install Chromium (for chromedp) ---
echo "🌐 Installing Chromium browser..."
sudo apt-get install -y -qq chromium-browser fonts-liberation libappindicator3-1 libasound2 \
    libatk-bridge2.0-0 libatk1.0-0 libcups2 libdbus-1-3 libgdk-pixbuf2.0-0 \
    libnspr4 libnss3 libx11-xcb1 libxcomposite1 libxdamage1 libxrandr2 \
    xdg-utils wget ca-certificates

# Verify Chrome
CHROME_PATH=$(which chromium-browser || which chromium || echo "")
if [ -z "$CHROME_PATH" ]; then
    echo "❌ Chromium installation failed!"
    exit 1
fi
echo "✅ Chromium installed: $CHROME_PATH"
$CHROME_PATH --version

# --- 3. Create application directory ---
echo "📁 Setting up application directory..."
sudo mkdir -p /opt/thg-scraper/data
sudo chown -R $USER:$USER /opt/thg-scraper

# --- 4. Create .env template if not exists ---
if [ ! -f /opt/thg-scraper/.env ]; then
    cat > /opt/thg-scraper/.env << 'ENVEOF'
# THG Agentic Scraper — Production Config
TELEGRAM_BOT_TOKEN=
TELEGRAM_ADMIN_CHAT_ID=
GROQ_API_KEY=
GEMINI_API_KEY=
CHROME_PATH=/usr/bin/chromium-browser
WEB_PORT=8080
DB_PATH=data/scraper.db
MAX_WORKERS=3
SCAN_INTERVAL_MIN=30
ENVEOF
    echo "⚠️  Created .env template at /opt/thg-scraper/.env"
    echo "   → Edit it with your actual tokens!"
fi

# --- 5. Create systemd service ---
echo "⚙️  Creating systemd service..."
sudo tee /etc/systemd/system/thg-scraper.service > /dev/null << 'SERVICEEOF'
[Unit]
Description=THG Agentic Scraper
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=ubuntu
Group=ubuntu
WorkingDirectory=/opt/thg-scraper
ExecStart=/opt/thg-scraper/scraper
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=thg-scraper

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ReadWritePaths=/opt/thg-scraper
PrivateTmp=true

# Environment
EnvironmentFile=/opt/thg-scraper/.env

# Chrome needs /dev/shm
ReadWritePaths=/dev/shm

[Install]
WantedBy=multi-user.target
SERVICEEOF

sudo systemctl daemon-reload
sudo systemctl enable thg-scraper
echo "✅ Systemd service created and enabled"

# --- 6. Open firewall port 8080 ---
echo "🔥 Opening firewall port 8080..."
sudo iptables -I INPUT -p tcp --dport 8080 -j ACCEPT
# Persist iptables rules
echo iptables-persistent iptables-persistent/autosave_v4 boolean true | sudo debconf-set-selections
echo iptables-persistent iptables-persistent/autosave_v6 boolean true | sudo debconf-set-selections
sudo apt-get install -y -qq iptables-persistent || true
sudo netfilter-persistent save || true

# --- 7. Setup log rotation ---
echo "📋 Setting up log rotation..."
sudo tee /etc/logrotate.d/thg-scraper > /dev/null << 'LOGEOF'
/opt/thg-scraper/data/*.log {
    daily
    rotate 7
    compress
    missingok
    notifempty
}
LOGEOF

# --- 8. Create helper commands ---
echo "🛠️  Creating helper scripts..."
cat > /opt/thg-scraper/logs.sh << 'EOF'
#!/bin/bash
sudo journalctl -u thg-scraper -f --no-pager
EOF
chmod +x /opt/thg-scraper/logs.sh

cat > /opt/thg-scraper/restart.sh << 'EOF'
#!/bin/bash
sudo systemctl restart thg-scraper
echo "✅ Scraper restarted"
sudo systemctl status thg-scraper --no-pager -l
EOF
chmod +x /opt/thg-scraper/restart.sh

cat > /opt/thg-scraper/status.sh << 'EOF'
#!/bin/bash
echo "=== Service Status ==="
sudo systemctl status thg-scraper --no-pager -l
echo ""
echo "=== Web UI Check ==="
curl -s http://localhost:8080/api/stats | python3 -m json.tool 2>/dev/null || echo "Web UI not responding"
echo ""
echo "=== Disk Usage ==="
du -sh /opt/thg-scraper/data/
echo ""
echo "=== Memory ==="
free -h
EOF
chmod +x /opt/thg-scraper/status.sh

echo ""
echo "============================================="
echo "✅ Server setup complete!"
echo "============================================="
echo ""
echo "📋 Next steps:"
echo "  1. Edit .env:    nano /opt/thg-scraper/.env"
echo "  2. Upload binary: scp scraper ubuntu@<IP>:/opt/thg-scraper/"
echo "  3. Start service: sudo systemctl start thg-scraper"
echo "  4. View logs:     /opt/thg-scraper/logs.sh"
echo "  5. Check status:  /opt/thg-scraper/status.sh"
echo "  6. Web UI:        http://$(curl -s ifconfig.me):8080"
echo ""
