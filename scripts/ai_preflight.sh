#!/usr/bin/env bash
# ai_preflight.sh — safe, read-only preflight for AI-assisted development.
#
# Shows where you are and runs the two architecture guards. It NEVER mutates the
# working tree: destructive clean (git reset/clean) stays a manual, deliberate
# step. Run this before starting a queue item; run ai_validate.sh before pushing.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "== branch / HEAD =="
git rev-parse --abbrev-ref HEAD
git log -1 --format='%H %s'

echo
echo "== git status --short =="
git status --short

echo
echo "== import-boundary guard =="
bash scripts/check_import_boundaries.sh

echo
echo "== file-size guard =="
python scripts/check_file_size.py

echo
echo "== docs governance guard =="
bash scripts/check_docs_governance.sh

echo
echo "preflight OK"
