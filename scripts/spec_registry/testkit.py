"""Shared in-memory fixtures for the spec-registry test harnesses."""
from pathlib import Path

from .discovery import load_manifests
from .model import Manifest, Node

ROOT = Path(__file__).resolve().parents[2]


def entry(**over) -> dict:
    base = {
        "id": "sample-spec", "document": "technical.md", "title": "Sample",
        "metadata_version": 3, "layer": "technical", "authority": "authoritative",
        "lifecycle": "active", "implementation_state": "backed", "effective": True,
        "owner": "unassigned", "last_reviewed": "2026-07-21", "verified_against": [],
        "related_code": [], "related_tests": [], "supersedes": [],
        "superseded_by": None, "tags": [], "notes": "",
        "compatibility": {"domain": "facebook", "type": "behavior",
                          "status": "active", "maturity": "test_pinned"},
    }
    base.update(over)
    return base


def manifest(kind="technical_feature", node_id="comment-automation",
             domain="facebook-sales-intelligence", domain_kind="product",
             entries=(), supported=(), directory=None) -> Manifest:
    if directory is None:
        if kind == "domain":
            directory = Path("specs/domains") / domain
        else:
            sub = "experiences" if kind == "experience" else "features"
            directory = Path("specs/domains") / domain / sub / node_id
    node = Node(kind=kind, id=node_id, ownership_domain=domain,
                domain_kind=domain_kind, supported_experiences=tuple(supported))
    return Manifest(path=directory / "SPEC_MANIFEST.json", directory=directory,
                    node=node, entries=tuple(entries))


def expect(errors: list[str], fragment: str) -> None:
    assert any(fragment in e for e in errors), f"expected '{fragment}' in {errors}"


def real_manifests() -> list[Manifest]:
    errors: list[str] = []
    manifests = load_manifests(ROOT, errors)
    assert errors == [], errors
    return manifests
