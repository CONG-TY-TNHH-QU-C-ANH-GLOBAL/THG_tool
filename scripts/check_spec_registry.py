"""Spec registry drift guard (stdlib only).

Validates specs/SPEC_REGISTRY.json against the lifecycle rules described in
specs/SPEC_GOVERNANCE.md. This is a foundation checker: it proves the registry
is well-formed and in sync with the files on disk. It does NOT audit spec
content and does NOT mutate anything.

Checks:
  - JSON parses; schema_version present; entries is a list
  - every entry has the required fields
  - status / type / maturity use allowed enum values
  - every entry path exists on disk
  - every specs/**/*.md file is registered (except the two governance docs)
  - no duplicate id; no duplicate path
  - superseded_by, when set, points at a known id or registered path

Exit 0 on PASS, non-zero on FAIL.
"""
from pathlib import Path
import json
import sys

ROOT = Path(__file__).resolve().parents[1]
SPECS_DIR = ROOT / "specs"
REGISTRY = SPECS_DIR / "SPEC_REGISTRY.json"

# Files that live in specs/ but are governance scaffolding, not registered specs.
NOT_REGISTERED = {"README.md", "SPEC_GOVERNANCE.md"}

REQUIRED_FIELDS = [
    "id", "path", "title", "domain", "type", "status", "maturity",
    "owner", "last_reviewed", "verified_against", "related_code",
    "related_tests", "supersedes", "superseded_by", "tags", "notes",
]

STATUS = {"active", "draft", "unreviewed", "stale", "deprecated", "superseded", "archived"}
TYPE = {"behavior", "architecture", "contract", "migration", "investigation",
        "runbook", "report", "roadmap", "policy", "unknown"}
MATURITY = {"snapshot", "reviewed", "test_pinned", "implementation_backed", "superseded", "unknown"}

# Per-entry v2 metadata opt-in ("metadata_version": 2, ownership_domain
# taxonomy) — rules live in spec_registry_v2.py beside this script.
from spec_registry_v2 import check_v2_entry, check_v2_uniqueness  # noqa: E402


def load_registry(errors: list[str]):
    if not REGISTRY.exists():
        errors.append(f"registry not found: {REGISTRY.relative_to(ROOT).as_posix()}")
        return None
    try:
        data = json.loads(REGISTRY.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        errors.append(f"registry is not valid JSON: {exc}")
        return None
    if not isinstance(data, dict):
        errors.append("registry root must be a JSON object")
        return None
    if "schema_version" not in data:
        errors.append("registry missing 'schema_version'")
    if not isinstance(data.get("entries"), list):
        errors.append("registry 'entries' must be a list")
        return None
    return data


def check_entry(entry, idx: int, errors: list[str]) -> None:
    where = f"entries[{idx}]"
    if not isinstance(entry, dict):
        errors.append(f"{where} must be an object")
        return
    label = entry.get("id", where)
    for field in REQUIRED_FIELDS:
        if field not in entry:
            errors.append(f"{label}: missing required field '{field}'")
    if entry.get("status") not in STATUS:
        errors.append(f"{label}: invalid status '{entry.get('status')}'")
    if entry.get("type") not in TYPE:
        errors.append(f"{label}: invalid type '{entry.get('type')}'")
    if entry.get("maturity") not in MATURITY:
        errors.append(f"{label}: invalid maturity '{entry.get('maturity')}'")
    path = entry.get("path")
    if isinstance(path, str) and not (ROOT / path).exists():
        errors.append(f"{label}: path does not exist: {path}")
    check_v2_entry(entry, label, errors)


def _index_unique(entries, key: str, kind: str, errors: list[str]) -> dict[str, int]:
    """Map each string value of `key` to its first index, flagging duplicates."""
    seen: dict[str, int] = {}
    for idx, entry in enumerate(entries):
        value = entry.get(key) if isinstance(entry, dict) else None
        if not isinstance(value, str):
            continue
        if value in seen:
            errors.append(f"duplicate {kind} '{value}' (entries {seen[value]} and {idx})")
        else:
            seen[value] = idx
    return seen


def validate_unique_ids_and_paths(entries, errors: list[str]):
    ids = _index_unique(entries, "id", "id", errors)
    paths = _index_unique(entries, "path", "path", errors)
    return ids, paths


def validate_superseded_refs(entries, known, errors: list[str]) -> None:
    for entry in entries:
        if not isinstance(entry, dict):
            continue
        sb = entry.get("superseded_by")
        if sb and sb not in known:
            errors.append(f"{entry.get('id')}: superseded_by '{sb}' is not a known id or path")


def check_cross_entry(entries, errors: list[str]) -> None:
    ids, paths = validate_unique_ids_and_paths(entries, errors)
    validate_superseded_refs(entries, set(ids) | set(paths), errors)
    check_v2_uniqueness(entries, errors)


def check_coverage(entries, errors: list[str]) -> None:
    # Specs live in per-domain subfolders (specs/<domain>/<file>.md), so scan
    # recursively. The two governance docs at the root are not registered.
    registered = {e.get("path") for e in entries if isinstance(e, dict)}
    for md in sorted(SPECS_DIR.rglob("*.md")):
        if md.name in NOT_REGISTERED:
            continue
        rel = md.relative_to(ROOT).as_posix()
        if rel not in registered:
            errors.append(f"spec not registered: {rel}")


def main() -> int:
    errors: list[str] = []
    data = load_registry(errors)
    if data is not None:
        entries = data["entries"]
        for idx, entry in enumerate(entries):
            check_entry(entry, idx, errors)
        check_cross_entry(entries, errors)
        check_coverage(entries, errors)
        print(f"registry entries: {len(entries)}")

    if errors:
        print(f"FAIL: {len(errors)} problem(s) found:")
        for err in errors:
            print(f"  - {err}")
        print("RESULT: FAIL")
        return 1

    print("RESULT: PASS - spec registry is well-formed and in sync with specs/.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
