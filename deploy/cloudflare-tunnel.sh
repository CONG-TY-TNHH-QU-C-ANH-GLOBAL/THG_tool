#!/bin/bash
# Cloudflare Tunnel setup — routes sale.thgfulfill.com → localhost:8080
# No server needed. Runs on your own PC/laptop or any cheap VPS.
# Cost: $0 (Cloudflare Tunnel is free forever)
#
# Usage: bash cloudflare-tunnel.sh
set -euo pipefail

echo "======================================"
echo "  Cloudflare Tunnel Setup"
echo "  sale.thgfulfill.com → localhost:8080"
echo "======================================"

# ── 1. Install cloudflared ────────────────────────────────────────────────
install_cloudflared() {
    if command -v cloudflared &>/dev/null; then
        echo "✅ cloudflared already installed: $(cloudflared --version)"
        return
    fi

    echo "Installing cloudflared..."
    ARCH=$(uname -m)
    if [ "$ARCH" = "x86_64" ]; then
        URL="https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64"
    elif [ "$ARCH" = "aarch64" ]; then
        URL="https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-arm64"
    else
        echo "❌ Unsupported architecture: $ARCH"
        exit 1
    fi

    curl -fsSL "$URL" -o /usr/local/bin/cloudflared
    chmod +x /usr/local/bin/cloudflared
    echo "✅ cloudflared installed: $(cloudflared --version)"
}

# ── 2. Authenticate with Cloudflare ──────────────────────────────────────
authenticate() {
    echo ""
    echo "Opening browser to authenticate with Cloudflare..."
    echo "Sign in and select 'thgfulfill.com' when prompted."
    echo ""
    cloudflared tunnel login
    echo "✅ Authenticated"
}

# ── 3. Create the tunnel ──────────────────────────────────────────────────
create_tunnel() {
    TUNNEL_NAME="thg-scraper"

    # Check if tunnel already exists
    if cloudflared tunnel list 2>/dev/null | grep -q "$TUNNEL_NAME"; then
        echo "ℹ️  Tunnel '$TUNNEL_NAME' already exists"
        TUNNEL_ID=$(cloudflared tunnel list | grep "$TUNNEL_NAME" | awk '{print $1}')
    else
        echo "Creating tunnel: $TUNNEL_NAME"
        cloudflared tunnel create "$TUNNEL_NAME"
        TUNNEL_ID=$(cloudflared tunnel list | grep "$TUNNEL_NAME" | awk '{print $1}')
        echo "✅ Tunnel created: $TUNNEL_ID"
    fi

    echo "$TUNNEL_ID" > /tmp/tunnel_id
}

# ── 4. Create tunnel config ───────────────────────────────────────────────
create_config() {
    TUNNEL_ID=$(cat /tmp/tunnel_id)
    CONFIG_DIR="$HOME/.cloudflared"
    mkdir -p "$CONFIG_DIR"

    cat > "$CONFIG_DIR/config.yml" << EOF
tunnel: $TUNNEL_ID
credentials-file: $CONFIG_DIR/${TUNNEL_ID}.json

# Route sale.thgfulfill.com → Go app on localhost:8080
ingress:
  - hostname: sale.thgfulfill.com
    service: http://localhost:8080
    originRequest:
      connectTimeout: 30s
      noTLSVerify: false
  # Catch-all: reject everything else
  - service: http_status:404
EOF

    echo "✅ Config written: $CONFIG_DIR/config.yml"
}

# ── 5. Create Cloudflare DNS route ────────────────────────────────────────
create_dns() {
    TUNNEL_ID=$(cat /tmp/tunnel_id)
    echo "Creating DNS route: sale.thgfulfill.com → tunnel"
    cloudflared tunnel route dns thg-scraper sale.thgfulfill.com || \
        echo "ℹ️  DNS route may already exist — check Cloudflare dashboard"
    echo "✅ DNS route configured"
}

# ── 6. Install as systemd service ─────────────────────────────────────────
install_service() {
    cat > /etc/systemd/system/cloudflared.service << 'EOF'
[Unit]
Description=Cloudflare Tunnel (sale.thgfulfill.com)
After=network.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/cloudflared tunnel run thg-scraper
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable cloudflared
    systemctl start cloudflared
    echo "✅ cloudflared service installed and started"
}

# ── Run steps ─────────────────────────────────────────────────────────────
if [ "$EUID" -ne 0 ]; then
    echo "Run with sudo: sudo bash cloudflare-tunnel.sh"
    exit 1
fi

install_cloudflared
authenticate
create_tunnel
create_config
create_dns
install_service

echo ""
echo "======================================"
echo "  DONE!"
echo "======================================"
echo ""
echo "  Your app is now live at:"
echo "  https://sale.thgfulfill.com"
echo ""
echo "  Check tunnel status:"
echo "  sudo systemctl status cloudflared"
echo "  sudo journalctl -u cloudflared -f"
echo ""
echo "  NOTE: Cloudflare handles HTTPS automatically."
echo "  No Nginx, no SSL certificate needed."
echo ""
