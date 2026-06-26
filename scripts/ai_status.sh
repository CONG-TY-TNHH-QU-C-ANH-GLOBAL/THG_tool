#!/usr/bin/env bash
# ai_status.sh — one-glance status for AI-assisted development.
# Prints branch, HEAD, working-tree status, and the next READY queue item.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "branch: $(git rev-parse --abbrev-ref HEAD)"
echo "HEAD:   $(git log -1 --format='%h %s')"

echo
echo "status:"
git status --short || true

echo
echo "next queue item:"
grep -m1 '^### READY' docs/ai/AUTOPILOT_QUEUE.md || echo "  (no READY item)"
