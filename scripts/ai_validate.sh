#!/usr/bin/env bash
# ai_validate.sh — standard validation gate before pushing an AI-assisted PR.
#
# Wraps the checks every PR must pass. Fails fast on the first failure
# (set -e), so a green run means tests + build + vet + guards + whitespace
# are all clean. No CI config, no frameworks.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "== go test ./... =="
go test ./...

echo "== go build ./... =="
go build ./...

echo "== go vet ./... =="
go vet ./...

echo "== import-boundary guard =="
bash scripts/check_import_boundaries.sh

echo "== file-size guard =="
python scripts/check_file_size.py

echo "== docs governance guard =="
bash scripts/check_docs_governance.sh

echo "== git diff --check (whitespace) =="
git diff --check

echo
echo "validate OK"
