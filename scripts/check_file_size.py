"""Baseline-aware file-size guardrail.

Rule: a production code file must stay <= 200 lines. Legacy files that already
exceeded this when the guardrail was introduced are listed in
scripts/file_size_allowlist.txt and only WARN (so CI does not fail on day one).

  - non-allowlisted file > 200 lines  -> FAIL  (exit 1)
  - allowlisted   file > 200 lines    -> WARN  (exit 0)
  - nothing over the limit            -> PASS  (exit 0)

The allowlist is a TEMPORARY baseline, NOT a licence to keep growing god files.
Split a file or add a DOCUMENTED exception only if truly necessary.
"""
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[1]
ALLOWLIST_FILE = ROOT / "scripts" / "file_size_allowlist.txt"

MAX_LINES = 200

EXCLUDE_DIRS = {".git", "node_modules", "dist", "build", ".next", "coverage", "vendor"}
EXCLUDE_SUFFIXES = {".min.js", ".map"}

# Always-allowed (generated / vendored) - kept for backward compatibility.
ALLOW_LARGE = {"internal/store/schema.go", "go.sum"}

CODE_EXTS = {".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".sql"}


def load_allowlist() -> set[str]:
    allow = set(ALLOW_LARGE)
    if not ALLOWLIST_FILE.exists():
        return allow
    for raw in ALLOWLIST_FILE.read_text(encoding="utf-8", errors="ignore").splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        # Strip an inline "  # 1508 lines" comment, keep just the path.
        path = line.split("#", 1)[0].strip()
        if path:
            allow.add(path)
    return allow


def main() -> int:
    allow = load_allowlist()
    checked = 0
    fails: list[tuple[str, int]] = []
    warns: list[tuple[str, int]] = []
    sizes: list[tuple[str, int]] = []

    for path in ROOT.rglob("*"):
        if not path.is_file():
            continue
        if any(part in EXCLUDE_DIRS for part in path.parts):
            continue
        rel = path.relative_to(ROOT).as_posix()
        if any(rel.endswith(s) for s in EXCLUDE_SUFFIXES):
            continue
        if path.suffix not in CODE_EXTS:
            continue
        try:
            n = len(path.read_text(encoding="utf-8", errors="ignore").splitlines())
        except Exception:
            continue
        checked += 1
        sizes.append((rel, n))
        if n <= MAX_LINES:
            continue
        if rel in allow:
            warns.append((rel, n))
        else:
            fails.append((rel, n))

    if fails:
        print(f"FAIL: {len(fails)} non-allowlisted file(s) over {MAX_LINES} lines:")
        for rel, n in sorted(fails, key=lambda x: -x[1]):
            print(f"  {n:4d} lines  {rel}")
        print()

    if warns:
        print(f"WARN: {len(warns)} allowlisted legacy file(s) over {MAX_LINES} lines "
              f"(baseline - split when touched, do not grow):")
        for rel, n in sorted(warns, key=lambda x: -x[1])[:10]:
            print(f"  {n:4d} lines  {rel}")
        if len(warns) > 10:
            print(f"  ... and {len(warns) - 10} more (see scripts/file_size_allowlist.txt)")
        print()

    print("--- summary ---")
    print(f"  total checked files          : {checked}")
    print(f"  FAIL (non-allowlisted >200)  : {len(fails)}")
    print(f"  WARN (allowlisted legacy)    : {len(warns)}")
    print("  top 10 largest files:")
    for rel, n in sorted(sizes, key=lambda x: -x[1])[:10]:
        print(f"    {n:4d} lines  {rel}")
    print()

    if fails:
        print("RESULT: FAIL - split the file(s) above, or add a DOCUMENTED exception to "
              "scripts/file_size_allowlist.txt only if truly necessary.")
        return 1

    print("RESULT: PASS - no new oversized production files. "
          "(Allowlisted legacy files are a temporary baseline, not permission to grow them.)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
