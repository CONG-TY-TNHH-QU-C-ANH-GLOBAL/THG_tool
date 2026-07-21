#!/usr/bin/env python3
"""Component-structure guard (WARN-ONLY baseline).

Surfaces the flat-package smells defined in specs/domains/platform-foundation/decisions/component-structure-rules.md so
new growth is visible in review, WITHOUT blocking CI while current hotspots are
triaged in specs/COMPONENT_HOTSPOTS.md.

Flags (warnings only — exit code is always 0 unless --strict):
  * a Go package with > MAX_FILES .go files
  * > MAX_PREFIX files sharing the same prefix (text before the first '_')
  * a vague catch-all file (utils.go / helpers.go / misc.go / manager.go) that
    is NOT component-prefixed (e.g. comment_utils.go is fine; utils.go is not)

Usage:
  python scripts/check_component_structure.py          # warn, exit 0
  python scripts/check_component_structure.py --strict # exit 1 if anything flagged
"""
from __future__ import annotations

import os
import sys
from collections import defaultdict

ROOTS = ("internal", "cmd")
MAX_FILES = 15
MAX_PREFIX = 5
VAGUE = {"utils.go", "helpers.go", "misc.go", "manager.go"}

# Packages intentionally exempt from the file-count trigger (with reason).
# Keep this list SHORT and justified — it is a triage record, not a licence.
COUNT_EXEMPT = {
    "internal/models": "data-contracts package; flatness is its correct shape",
    "internal/store": "god-package mid-decomposition (STORE_SUBPACKAGE_REFACTOR.md)",
}


def go_files(path: str) -> list[str]:
    try:
        return sorted(f for f in os.listdir(path) if f.endswith(".go"))
    except OSError:
        return []


def prefix_of(name: str) -> str:
    base = name[:-3]  # strip .go
    base = base[:-5] if base.endswith("_test") else base
    return base.split("_", 1)[0] if "_" in base else ""


def main() -> int:
    strict = "--strict" in sys.argv
    repo = os.getcwd()
    file_warn: list[str] = []
    prefix_warn: list[str] = []
    vague_warn: list[str] = []

    for root in ROOTS:
        for dirpath, _dirs, _files in os.walk(os.path.join(repo, root)):
            files = go_files(dirpath)
            if not files:
                continue
            rel = os.path.relpath(dirpath, repo).replace("\\", "/")
            # The >15 trigger is about SOURCE files (rule 2). Count non-test
            # .go files; report _test.go separately as test_count so a
            # test-heavy package isn't flagged for its tests.
            src = [f for f in files if not f.endswith("_test.go")]
            test_count = len(files) - len(src)

            if len(src) > MAX_FILES and rel not in COUNT_EXEMPT:
                file_warn.append(
                    f"  {len(src):3d} src (+{test_count} test)  {rel}  (> {MAX_FILES} source)")

            clusters: dict[str, int] = defaultdict(int)
            for f in files:
                p = prefix_of(f)
                if p:
                    clusters[p] += 1
            for p, n in sorted(clusters.items(), key=lambda kv: -kv[1]):
                if n > MAX_PREFIX:
                    prefix_warn.append(f"  {n:3d} x {p}_*  in  {rel}  (> {MAX_PREFIX})")

            for f in files:
                if f in VAGUE:
                    vague_warn.append(f"  {rel}/{f}  (vague catch-all; prefix it or document a single responsibility)")

    print("== Component structure guard (warn-only baseline) ==")
    print("Rules: specs/domains/platform-foundation/decisions/component-structure-rules.md | Triage: specs/domains/platform-foundation/decisions/component-hotspots.md\n")

    def section(title: str, rows: list[str]) -> None:
        print(f"[{title}] {len(rows)} flagged")
        for r in rows:
            print(r)
        print()

    section(f"packages > {MAX_FILES} source .go files (excl. _test.go)", file_warn)
    section(f"prefix clusters > {MAX_PREFIX} files", prefix_warn)
    section("vague catch-all files", vague_warn)

    total = len(file_warn) + len(prefix_warn) + len(vague_warn)
    if total == 0:
        print("RESULT: PASS — no component-structure smells.")
        return 0
    print(f"RESULT: {total} warning(s). WARN-ONLY -- does not fail CI "
          f"(use --strict to enforce once hotspots are triaged).")
    return 1 if strict else 0


if __name__ == "__main__":
    raise SystemExit(main())
