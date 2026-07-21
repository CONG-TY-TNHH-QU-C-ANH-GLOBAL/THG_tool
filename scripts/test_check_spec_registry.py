"""Regression tests for manifest validation rules (stdlib only).

Run: python scripts/test_check_spec_registry.py
Generation/drift tests live in test_build_spec_registry.py.
"""
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parent))

from spec_registry import entry_checks as ec  # noqa: E402
from spec_registry import validation as v  # noqa: E402
from spec_registry.testkit import ROOT, entry, expect, manifest, real_manifests  # noqa: E402

CASES = []


def case(fn):
    CASES.append(fn)
    return fn


@case
def duplicate_id_across_manifests():
    errors: list[str] = []
    a = manifest(entries=[entry()])
    b = manifest(node_id="comment-intelligence", entries=[entry()])
    v._check_unique_ids_and_documents([a, b], errors)
    expect(errors, "duplicate spec id 'sample-spec'")


@case
def duplicate_document_path():
    errors: list[str] = []
    m = manifest(entries=[entry(), entry(id="other-spec")])
    v._check_unique_ids_and_documents([m], errors)
    expect(errors, "registered more than once")


@case
def missing_document():
    errors: list[str] = []
    ec.check_entry(ROOT, manifest(), entry(document="does-not-exist.md"), errors)
    expect(errors, "document does not exist")


@case
def path_escape():
    errors: list[str] = []
    ec.check_entry(ROOT, manifest(), entry(document="../technical.md"), errors)
    expect(errors, "escapes the manifest node")


@case
def node_directory_mismatch():
    errors: list[str] = []
    wrong = manifest(directory=Path("specs/domains/facebook-sales-intelligence/"
                                    "features/comment-intelligence"))
    v._check_node(wrong, errors)
    expect(errors, "does not match directory")


@case
def unsupported_node_layer_pair():
    errors: list[str] = []
    ec.check_entry_enums(manifest(), entry(layer="business"), "t", errors)
    expect(errors, "not valid for a technical_feature node")
    ok: list[str] = []
    ec.check_entry_enums(manifest(), entry(), "t", ok)
    assert ok == []


@case
def domain_level_authority_without_fake_feature():
    errors: list[str] = []
    m = manifest(kind="domain", node_id="platform-foundation",
                 domain="platform-foundation", domain_kind="platform")
    v._check_node(m, errors)
    ec.check_entry(ROOT, m, entry(layer="domain", document="DOMAIN.md"), errors)
    assert errors == [], errors


@case
def technical_authority_uniqueness():
    errors: list[str] = []
    m = manifest(entries=[entry(), entry(id="rival-spec")])
    v._check_authority_uniqueness([m], errors)
    expect(errors, "duplicate authoritative technical authority")
    ok: list[str] = []
    calm = manifest(entries=[entry(), entry(id="rival-spec", authority="supporting")])
    v._check_authority_uniqueness([calm], ok)
    assert ok == []


@case
def experience_layer_uniqueness():
    errors: list[str] = []
    m = manifest(kind="experience", node_id="engagement-approval",
                 entries=[entry(layer="business", implementation_state="partial"),
                          entry(id="rival-spec", layer="business",
                                implementation_state="partial")])
    v._check_authority_uniqueness([m], errors)
    expect(errors, "duplicate authoritative business authority")


@case
def historical_and_archived_require_effective_false():
    errors: list[str] = []
    ec.check_entry_governance(entry(authority="historical"), "t", errors)
    expect(errors, "requires effective false")
    errors = []
    ec.check_entry_governance(entry(lifecycle="archived"), "t", errors)
    expect(errors, "requires effective false")
    ok: list[str] = []
    ec.check_entry_governance(entry(authority="historical", lifecycle="archived",
                                    effective=False), "t", ok)
    assert ok == []


@case
def supported_experience_reference_validation():
    errors: list[str] = []
    feature = manifest(supported=("ghost-experience",), entries=[entry()])
    v._check_supported_experience_refs([feature], errors)
    expect(errors, "has no experience-node manifest")
    ok: list[str] = []
    experience = manifest(kind="experience", node_id="ghost-experience")
    v._check_supported_experience_refs([feature, experience], ok)
    assert ok == []


@case
def real_repo_registered_exactly_once():
    manifests = real_manifests()
    errors: list[str] = []
    v.validate_manifests(ROOT, manifests, errors)
    assert errors == [], errors
    paths = [(m.directory / str(e["document"])).as_posix()
             for m in manifests for e in m.entries]
    assert len(paths) == len(set(paths))


def main() -> int:
    for fn in CASES:
        fn()
    print(f"test_check_spec_registry: {len(CASES)}/{len(CASES)} cases PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
