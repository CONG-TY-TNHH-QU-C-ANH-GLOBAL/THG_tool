"""Regression tests for registry generation and drift detection (stdlib only).

Run: python scripts/test_build_spec_registry.py
Manifest-validation tests live in test_check_spec_registry.py.
"""
from pathlib import Path
import json
import sys
import tempfile

sys.path.insert(0, str(Path(__file__).resolve().parent))

from spec_registry.generation import drift_errors, generate_registry, render  # noqa: E402
from spec_registry.testkit import ROOT, entry, expect, manifest, real_manifests  # noqa: E402

CASES = []


def case(fn):
    CASES.append(fn)
    return fn


@case
def deterministic_generation():
    registry = generate_registry(real_manifests())
    ids = [e["id"] for e in registry["entries"]]
    assert ids == sorted(ids)
    reviewed = [e["last_reviewed"] for e in registry["entries"] if e["last_reviewed"]]
    assert registry["updated_at"] == max(reviewed)
    assert registry["generated"] is True


@case
def repeated_generation_is_byte_identical():
    first = render(generate_registry(real_manifests()))
    second = render(generate_registry(real_manifests()))
    assert first == second
    assert first.endswith("\n")


@case
def aggregate_drift_detection():
    registry = generate_registry(real_manifests())
    with tempfile.TemporaryDirectory() as tmp:
        tmp_root = Path(tmp)
        expect(drift_errors(tmp_root, registry), "is missing")
        (tmp_root / "specs").mkdir()
        (tmp_root / "specs/SPEC_REGISTRY.json").write_text("{}\n", encoding="utf-8")
        expect(drift_errors(tmp_root, registry), "stale or hand-edited")
    assert drift_errors(ROOT, registry) == []


@case
def compatibility_field_flattening():
    m = manifest(supported=("engagement-approval",), entries=[entry()])
    registry = generate_registry([m])
    flat = registry["entries"][0]
    assert flat["domain"] == "facebook" and flat["type"] == "behavior"
    assert flat["status"] == "active" and flat["maturity"] == "test_pinned"
    assert flat["node_kind"] == "technical_feature"
    assert flat["node_id"] == "comment-automation"
    assert flat["ownership_domain"] == "facebook-sales-intelligence"
    assert flat["supported_experiences"] == ["engagement-approval"]
    assert flat["path"].endswith("features/comment-automation/technical.md")


@case
def legacy_consumer_compatibility():
    legacy_fields = ("id", "path", "title", "domain", "type", "status", "maturity",
                     "owner", "last_reviewed", "verified_against", "related_code",
                     "related_tests", "supersedes", "superseded_by", "tags", "notes")
    committed = json.loads((ROOT / "specs/SPEC_REGISTRY.json").read_text(encoding="utf-8"))
    assert "schema_version" in committed and isinstance(committed["entries"], list)
    for flat in committed["entries"]:
        for field in legacy_fields:
            assert field in flat, (flat.get("id"), field)
        assert (ROOT / flat["path"]).is_file(), flat["path"]


def main() -> int:
    for fn in CASES:
        fn()
    print(f"test_build_spec_registry: {len(CASES)}/{len(CASES)} cases PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
