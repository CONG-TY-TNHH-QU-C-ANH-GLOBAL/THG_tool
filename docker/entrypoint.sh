#!/bin/bash
set -e

DISPLAY_NUM=${DISPLAY_NUM:-99}
VNC_PORT=${VNC_PORT:-5900}
PROFILE_DIR=${PROFILE_DIR:-/profile}
SCREEN_RES=${SCREEN_RES:-1280x800x24}

echo "[Browser] Starting virtual display :${DISPLAY_NUM} (${SCREEN_RES})"

# Remove stale X locks from a previous crash
rm -f "/tmp/.X${DISPLAY_NUM}-lock" "/tmp/.X11-unix/X${DISPLAY_NUM}" 2>/dev/null || true

# Start Xvfb (virtual framebuffer — no physical monitor needed)
Xvfb ":${DISPLAY_NUM}" -screen 0 "${SCREEN_RES}" -ac +extension RANDR &
XVFB_PID=$!
echo "[Browser] Xvfb pid=${XVFB_PID}"
sleep 1

# Start x11vnc — binds to all interfaces inside container (host maps to 127.0.0.1 only)
x11vnc \
    -display ":${DISPLAY_NUM}" \
    -nopw \
    -listen 0.0.0.0 \
    -rfbport "${VNC_PORT}" \
    -forever \
    -quiet \
    -shared \
    -xkb \
    &
echo "[Browser] x11vnc started on port ${VNC_PORT}"
sleep 1

# Remove Chrome singleton locks left by a previous container crash
rm -f \
    "${PROFILE_DIR}/SingletonLock" \
    "${PROFILE_DIR}/SingletonCookie" \
    "${PROFILE_DIR}/SingletonSocket" \
    2>/dev/null || true

export DISPLAY=":${DISPLAY_NUM}"

CDP_PORT=${CDP_PORT:-9222}

echo "[Browser] Launching Chrome (profile=${PROFILE_DIR}, cdp=:${CDP_PORT})"
exec google-chrome \
    --no-first-run \
    --no-default-browser-check \
    --disable-notifications \
    --disable-infobars \
    --disable-blink-features=AutomationControlled \
    --no-sandbox \
    --disable-dev-shm-usage \
    --disable-setuid-sandbox \
    --user-data-dir="${PROFILE_DIR}" \
    --window-size=1280,800 \
    --start-maximized \
    --remote-debugging-port="${CDP_PORT}" \
    --remote-debugging-address=0.0.0.0 \
    "https://www.facebook.com"
