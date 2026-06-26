#!/bin/bash
# One-time server setup for Ubuntu 22.04 (AMD64 or ARM64)
# Works on: Hetzner, Bizfly Cloud, Viettel Cloud, DigitalOcean, Contabo, Azure
#
# Usage: sudo bash setup-server.sh
set -euo pipefail

echo "======================================"
echo "  THG Scraper — Server Setup"
echo "======================================"

# ── 0. Swap file (CRITICAL for servers with 1-2GB RAM) ───────────────────
# Chrome peaks at 400-600MB. Without swap, it OOM-kills on 1GB servers.
setup_swap() {
    local SWAP_SIZE="2G"
    if swapon --show | grep -q "/swapfile"; then
        echo "ℹ️  Swap already exists: $(free -h | grep Swap)"
        return
    fi
    echo "Setting up ${SWAP_SIZE} swap file..."
    fallocate -l "$SWAP_SIZE" /swapfile
    chmod 600 /swapfile
    mkswap /swapfile
    swapon /swapfile
    # Persist across reboots
    echo '/swapfile none swap sw 0 0' >> /etc/fstab
    # Tune: keep swap usage low unless RAM is actually full
    echo 'vm.swappiness=10' >> /etc/sysctl.conf
    sysctl -p
    echo "✅ Swap: $(free -h | grep Swap)"
}
setup_swap

# ── 1. System update ──────────────────────────────────────────────────────
apt-get update && apt-get upgrade -y
apt-get install -y --no-install-recommends \
    nginx \
    chromium-browser \
    fonts-liberation \
    curl \
    ufw \
    fail2ban \
    logrotate \
    ca-certificates \
    openssl \
    sqlite3 \
    python3 \
    python3-venv \
    python3-pip

echo "✅ Packages installed"

# ── 2. Dedicated service user (no shell, no login) ────────────────────────
if ! id thg-scraper &>/dev/null; then
    useradd --system --no-create-home --shell /usr/sbin/nologin thg-scraper
    echo "✅ Created user: thg-scraper"
else
    echo "ℹ️  User thg-scraper already exists"
fi

# ── 3. Directory structure ────────────────────────────────────────────────
mkdir -p /opt/thg-scraper/data/{logs,profiles,backups}
mkdir -p /etc/thg-scraper
mkdir -p /etc/ssl/cloudflare

# App binary dir: root owns it, service user can execute
chown root:thg-scraper /opt/thg-scraper
chmod 750 /opt/thg-scraper

# Data dir: service user owns it
chown -R thg-scraper:thg-scraper /opt/thg-scraper/data
chmod 750 /opt/thg-scraper/data

# SSL dir: root only
chmod 700 /etc/ssl/cloudflare

# Env file: root writes, service user reads, nobody else
touch /etc/thg-scraper/env
chown root:thg-scraper /etc/thg-scraper/env
chmod 640 /etc/thg-scraper/env

echo "✅ Directories and permissions set"

# ── 4. Install systemd service ────────────────────────────────────────────
if [[ -f /tmp/thg-scraper.service ]]; then
    cp /tmp/thg-scraper.service /etc/systemd/system/
    systemctl daemon-reload
    systemctl enable thg-scraper
    echo "✅ Systemd service installed and enabled"
else
    echo "⚠️  /tmp/thg-scraper.service not found — copy it manually later"
fi

# ── 5. Nginx setup ────────────────────────────────────────────────────────
# Add rate limit zones and Cloudflare real_ip restore
cat > /etc/nginx/conf.d/cloudflare.conf << 'EOF'
# Rate limit zones
limit_req_zone $binary_remote_addr zone=api:10m          rate=10r/s;
limit_req_zone $binary_remote_addr zone=auth_login:10m   rate=10r/m;
limit_req_zone $binary_remote_addr zone=auth_refresh:10m rate=60r/m;

# Restore real client IP from Cloudflare
set_real_ip_from 173.245.48.0/20;
set_real_ip_from 103.21.244.0/22;
set_real_ip_from 103.22.200.0/22;
set_real_ip_from 103.31.4.0/22;
set_real_ip_from 141.101.64.0/18;
set_real_ip_from 108.162.192.0/18;
set_real_ip_from 190.93.240.0/20;
set_real_ip_from 188.114.96.0/20;
set_real_ip_from 197.234.240.0/22;
set_real_ip_from 198.41.128.0/17;
set_real_ip_from 162.158.0.0/15;
set_real_ip_from 104.16.0.0/13;
set_real_ip_from 104.24.0.0/14;
set_real_ip_from 172.64.0.0/13;
set_real_ip_from 131.0.72.0/22;
set_real_ip_from 2400:cb00::/32;
set_real_ip_from 2606:4700::/32;
set_real_ip_from 2803:f800::/32;
set_real_ip_from 2405:b500::/32;
set_real_ip_from 2405:8100::/32;
set_real_ip_from 2a06:98c0::/29;
set_real_ip_from 2c0f:f248::/32;
real_ip_header CF-Connecting-IP;
EOF

if [[ -f /tmp/nginx.conf ]]; then
    cp /tmp/nginx.conf /etc/nginx/sites-available/thg-scraper
    ln -sf /etc/nginx/sites-available/thg-scraper /etc/nginx/sites-enabled/thg-scraper
    rm -f /etc/nginx/sites-enabled/default
    echo "✅ Nginx site config installed"
else
    echo "⚠️  /tmp/nginx.conf not found — copy it manually later"
fi

# ── 6. Firewall (UFW) ────────────────────────────────────────────────────
ufw --force reset
ufw default deny incoming
ufw default allow outgoing
ufw allow 22/tcp    comment 'SSH'
ufw allow 80/tcp    comment 'HTTP → Nginx (redirects to HTTPS)'
ufw allow 443/tcp   comment 'HTTPS → Nginx → App'
# Port 8080 stays internal — NOT exposed to internet
ufw --force enable
echo "✅ Firewall configured (port 8080 internal only)"

# ── 7. Fail2ban ───────────────────────────────────────────────────────────
cat > /etc/fail2ban/jail.local << 'EOF'
[DEFAULT]
bantime  = 3600
findtime = 600
maxretry = 5

[sshd]
enabled = true
port    = ssh
logpath = %(sshd_log)s
backend = %(syslog_backend)s

[nginx-limit-req]
enabled  = true
port     = http,https
logpath  = /var/log/nginx/*.error.log
maxretry = 10
EOF

systemctl enable fail2ban
systemctl restart fail2ban
echo "✅ Fail2ban configured"

# ── 8. Log rotation ───────────────────────────────────────────────────────
cat > /etc/logrotate.d/thg-scraper << 'EOF'
/opt/thg-scraper/data/logs/*.log {
    daily
    rotate 14
    compress
    missingok
    notifempty
}
EOF
echo "✅ Log rotation configured"

# ── 9. SSH hardening ──────────────────────────────────────────────────────
sed -i 's/^#*PasswordAuthentication.*/PasswordAuthentication no/' /etc/ssh/sshd_config
systemctl restart sshd
echo "✅ SSH: password login disabled (key-only)"

echo ""
echo "======================================"
echo "  Setup complete!"
echo "======================================"
echo ""
echo "NEXT STEPS:"
echo ""
echo "  1. Paste Cloudflare Origin Certificate:"
echo "     sudo nano /etc/ssl/cloudflare/origin.pem"
echo "     sudo nano /etc/ssl/cloudflare/origin.key"
echo "     sudo chmod 600 /etc/ssl/cloudflare/origin.key"
echo ""
echo "  2. Fill in your secrets:"
echo "     sudo nano /etc/thg-scraper/env"
echo ""
echo "  3. Generate JWT and encryption keys:"
echo "     openssl rand -hex 32   # run twice — one for JWT_SECRET, one for ENCRYPTION_KEY"
echo ""
echo "  4. Test nginx config:"
echo "     sudo nginx -t && sudo systemctl reload nginx"
echo ""
echo "  5. Deploy the app binary (via GitHub Actions or manually)"
echo ""
