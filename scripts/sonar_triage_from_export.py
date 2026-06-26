#!/usr/bin/env python
"""Triage a SonarQube/SonarCloud issues export into safe cleanup lanes.

Reads a JSON export (the shape returned by SonarQube `api/issues/search`:
either ``{"issues": [...]}`` or a bare list), keeps only ``status == "OPEN"``
issues, and groups them by rule and by file so an AI/human can pick the next
safe slice. Standard library only.

Lanes (conservative — when unsure, escalate):
  GREEN  mechanical, behaviour-preserving smell outside any controlled zone.
  YELLOW go:S3776 / go:S107 — refactor (pure extraction + direct tests).
  RED    controlled-zone path, OR a BUG/VULNERABILITY/SECURITY_HOTSPOT.
         RED is report-only: never fix autonomously (see /thg-red-audit).

Usage:
  python scripts/sonar_triage_from_export.py <export.json>
"""
import argparse
import collections
import json
import os
import sys

# Substrings that mark a controlled zone (behaviour-sensitive). Path matched
# case-insensitively. DTO/wire/handlers are softer hints (payload-contract risk).
CONTROLLED = (
    "auth", "session", "cookie", "migration", "schema", "queue", "outbox",
    "ledger", "action_ledger", "execution_attempts", "coordination",
    "connector", "jobhandlers", "crawler", "runtime", "/dto", "wire", "handlers",
)
YELLOW_RULES = ("S3776", "S107")
RED_TYPES = ("BUG", "VULNERABILITY", "SECURITY_HOTSPOT")


def file_of(component):
    # "projectKey:path/to/file.go" -> "path/to/file.go"
    return component.split(":", 1)[-1] if component else "?"


def lane(issue, path):
    low = path.lower()
    if any(h in low for h in CONTROLLED):
        return "RED"
    if issue.get("type") in RED_TYPES:
        return "RED"
    if any(issue.get("rule", "").endswith(r) for r in YELLOW_RULES):
        return "YELLOW"
    return "GREEN"


def validated_export_path(path):
    """Validate the CLI-supplied export path before opening it: normalise it
    (resolves ``..``/symlinks) and require an existing regular file with a data
    extension. Rejects a faulty/unexpected argument with a clear error instead of
    opening an arbitrary target."""
    real = os.path.realpath(path)
    if not os.path.isfile(real):
        raise ValueError(f"export is not a readable file: {path}")
    if os.path.splitext(real)[1].lower() not in (".json", ".txt"):
        raise ValueError(f"export must be a .json/.txt file: {path}")
    return real


def load_open(path):
    with open(validated_export_path(path), encoding="utf-8") as fh:
        data = json.load(fh)
    issues = data.get("issues", data) if isinstance(data, dict) else data
    return [i for i in issues if i.get("status") == "OPEN"]


def main():
    ap = argparse.ArgumentParser(
        description="Triage a Sonar issues export into GREEN/YELLOW/RED lanes.")
    ap.add_argument("export", help="path to the Sonar issues export JSON")
    ap.add_argument("--top", type=int, default=15, help="rows per section (default 15)")
    args = ap.parse_args()

    try:
        issues = load_open(args.export)
    except (OSError, ValueError) as err:
        print(f"error: cannot read export: {err}", file=sys.stderr)
        return 2

    by_lane = collections.Counter()
    by_rule = collections.Counter()
    rule_lane = {}
    green_files = collections.Counter()
    yellow_files = collections.Counter()
    for i in issues:
        path = file_of(i.get("component", ""))
        ln = lane(i, path)
        by_lane[ln] += 1
        rule = i.get("rule", "?")
        by_rule[rule] += 1
        rule_lane.setdefault(rule, ln)
        if ln == "GREEN":
            green_files[path] += 1
        elif ln == "YELLOW":
            yellow_files[path] += 1

    print(f"== sonar triage: {len(issues)} OPEN issues ==")
    print("lanes: " + " · ".join(f"{k}={by_lane[k]}" for k in ("GREEN", "YELLOW", "RED")))

    print(f"\n-- top rules (top {args.top}) --")
    for rule, n in by_rule.most_common(args.top):
        print(f"{n:4}  [{rule_lane[rule]:6}] {rule}")

    print(f"\n-- GREEN candidate files (top {args.top}) --")
    for f, n in green_files.most_common(args.top) or [("(none)", 0)]:
        print(f"{n:4}  {f}")

    print(f"\n-- YELLOW candidate files (S3776/S107, top {args.top}) --")
    for f, n in yellow_files.most_common(args.top) or [("(none)", 0)]:
        print(f"{n:4}  {f}")

    print("\nRED issues are report-only — use /thg-red-audit, do not fix autonomously.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
