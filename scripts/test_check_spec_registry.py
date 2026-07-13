"""Regression tests for the spec-registry validator (stdlib only).

Run: python scripts/test_check_spec_registry.py
Covers the legacy (v1) checks staying intact and the per-entry v2
metadata rules in spec_registry_v2.py. Exit 0 on PASS, non-zero on FAIL.
"""
import sys

import check_spec_registry as csr
from spec_registry_v2 import check_v2_entry, check_v2_uniqueness


def legacy_entry(**over):
    entry = {
        "id": "legacy-doc", "path": "CLAUDE.md", "title": "Legacy",
        "domain": "platform", "type": "architecture", "status": "active",
        "maturity": "reviewed", "owner": "unassigned", "last_reviewed": None,
        "verified_against": [], "related_code": [], "related_tests": [],
        "supersedes": [], "superseded_by": None, "tags": [], "notes": "",
    }
    entry.update(over)
    return entry


def v2_entry(**over):
    entry = legacy_entry(id="v2-doc", path="AGENTS.md", metadata_version=2,
                         ownership_domain="facebook-sales-intelligence",
                         domain_kind="product", layer="technical",
                         authority="authoritative", lifecycle="active",
                         implementation_state="partial", effective=True,
                         technical_feature="multi-group-fresh-lead-crawl")
    entry.update(over)
    return entry


def entry_errors(entry):
    errors: list[str] = []
    csr.check_entry(entry, 0, errors)
    return errors


def v2_errors(**over):
    errors: list[str] = []
    check_v2_entry(v2_entry(**over), "t", errors)
    return errors


def expect(errors, fragment):
    assert any(fragment in e for e in errors), f"expected '{fragment}' in {errors}"


def main() -> int:
    # 1. valid legacy entry
    assert entry_errors(legacy_entry()) == []
    # 2. valid mixed legacy/v2 registry (per-entry + cross-entry)
    mixed = [legacy_entry(), v2_entry()]
    errors: list[str] = []
    for i, e in enumerate(mixed):
        csr.check_entry(e, i, errors)
    csr.check_cross_entry(mixed, errors)
    assert errors == [], errors
    # 3. stray v2 field on v1 entry
    expect(entry_errors(legacy_entry(layer="technical")), "without 'metadata_version'")
    # 4. unsupported metadata_version
    expect(v2_errors(metadata_version=3), "unsupported metadata_version")
    # 5. missing required v2 field
    no_auth = v2_entry()
    del no_auth["authority"]
    errors = []
    check_v2_entry(no_auth, "t", errors)
    expect(errors, "requires field 'authority'")
    # 6. invalid enum
    expect(v2_errors(lifecycle="implementation_backed"), "invalid lifecycle")
    # 7. non-boolean effective
    expect(v2_errors(effective="true"), "'effective' must be a boolean")
    # 8. malformed slug
    expect(v2_errors(technical_feature="Fresh Lead"), "kebab-case slug")
    # 9. both experience and technical_feature populated
    expect(v2_errors(experience="fresh-lead-discovery"), "must not both be set")
    # 10. neither populated for a technical layer
    expect(v2_errors(technical_feature=None), "requires 'technical_feature'")
    # 11. experience layer using not_applicable
    expect(v2_errors(layer="experience", technical_feature=None,
                     experience="fresh-lead-discovery",
                     implementation_state="not_applicable"),
           "proposed/partial/backed")
    # 12. duplicate supported_experiences
    expect(v2_errors(supported_experiences=["a-b", "a-b"]), "duplicate supported_experiences")
    # 13. supported_experiences on an experience-owned document
    expect(v2_errors(layer="business", technical_feature=None,
                     experience="fresh-lead-discovery",
                     supported_experiences=["fresh-lead-discovery"]),
           "only allowed on technical-feature-owned")
    # 14. historical document with effective true
    expect(v2_errors(authority="historical"), "requires effective false")
    # 15. archived document with effective true
    expect(v2_errors(lifecycle="archived"), "requires effective false")
    # 16. duplicate authoritative technical documents for one feature
    errors = []
    check_v2_uniqueness([v2_entry(id="a"), v2_entry(id="b")], errors)
    expect(errors, "duplicate authoritative technical contract")
    errors = []
    check_v2_uniqueness([v2_entry(id="a"),
                         v2_entry(id="b", authority="supporting")], errors)
    assert errors == [], errors
    # 17. valid domain roadmap with both ownership-node fields absent
    assert v2_errors(layer="roadmap", technical_feature=None,
                     implementation_state="not_applicable") == []

    print("test_check_spec_registry: 17/17 cases PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
