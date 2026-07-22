"""Canonical validation for spec-registry manifests (Metadata V3).

Single source of validation truth: build_spec_registry.py and
check_spec_registry.py both call validate_manifests(). Per-entry rules
live in entry_checks.py; this module owns node and cross-manifest rules.
"""
from pathlib import Path

from .entry_checks import SLUG, check_entry
from .model import (DOMAIN_KIND, NODE_KINDS, UNIQUE_AUTHORITY_LAYERS, Manifest,
                    expected_node_for_directory)


def validate_manifests(root: Path, manifests: list[Manifest], errors: list[str]) -> None:
    for m in manifests:
        _check_node(m, errors)
        for entry in m.entries:
            check_entry(root, m, entry, errors)
    _check_cross_manifest(root, manifests, errors)


def _check_node(m: Manifest, errors: list[str]) -> None:
    label = m.label()
    expected = expected_node_for_directory(m.directory)
    if expected is None:
        errors.append(f"{label}: manifest directory is not a valid ownership node")
        return
    kind, node_id, domain = expected
    if (m.node.kind, m.node.id, m.node.ownership_domain) != (kind, node_id, domain):
        errors.append(f"{label}: node ({m.node.kind}/{m.node.id}/{m.node.ownership_domain}) "
                      f"does not match directory (expected {kind}/{node_id}/{domain})")
    if m.node.kind not in NODE_KINDS:
        errors.append(f"{label}: invalid node kind '{m.node.kind}'")
    if m.node.domain_kind not in DOMAIN_KIND:
        errors.append(f"{label}: invalid domain_kind '{m.node.domain_kind}'")
    _check_node_experiences(m, errors)


def _check_node_experiences(m: Manifest, errors: list[str]) -> None:
    label = m.label()
    supported = m.node.supported_experiences
    if supported and m.node.kind != "technical_feature":
        errors.append(f"{label}: supported_experiences is only allowed on technical_feature nodes")
    if len(set(supported)) != len(supported):
        errors.append(f"{label}: duplicate supported_experiences values")
    for slug in supported:
        if not SLUG.match(slug):
            errors.append(f"{label}: malformed supported_experiences slug '{slug}'")


def _check_cross_manifest(root: Path, manifests: list[Manifest], errors: list[str]) -> None:
    _check_unique_ids_and_documents(manifests, errors)
    _check_supported_experience_refs(manifests, errors)
    _check_authority_uniqueness(manifests, errors)
    _check_superseded_refs(manifests, errors)
    _check_coverage(root, manifests, errors)


def _check_unique_ids_and_documents(manifests: list[Manifest], errors: list[str]) -> None:
    seen_ids: dict[str, str] = {}
    seen_paths: dict[str, str] = {}
    for m in manifests:
        for entry in m.entries:
            eid = entry.get("id")
            path = (m.directory / str(entry.get("document"))).as_posix()
            if isinstance(eid, str):
                if eid in seen_ids:
                    errors.append(f"duplicate spec id '{eid}' in {seen_ids[eid]} and {m.label()}")
                seen_ids[eid] = m.label()
            if path in seen_paths:
                errors.append(f"document registered more than once: {path} "
                              f"({seen_paths[path]} and {m.label()})")
            seen_paths[path] = m.label()


def _check_supported_experience_refs(manifests: list[Manifest], errors: list[str]) -> None:
    experience_ids = {m.node.id for m in manifests if m.node.kind == "experience"}
    for m in manifests:
        for slug in m.node.supported_experiences:
            if slug not in experience_ids:
                errors.append(f"{m.label()}: supported experience '{slug}' has no "
                              "experience-node manifest")


def _check_authority_uniqueness(manifests: list[Manifest], errors: list[str]) -> None:
    seen: dict[tuple[str, str, str], str] = {}
    for m in manifests:
        for entry in m.entries:
            layer = entry.get("layer")
            if layer not in UNIQUE_AUTHORITY_LAYERS:
                continue
            if not (entry.get("authority") == "authoritative"
                    and entry.get("lifecycle") == "active"
                    and entry.get("effective") is True):
                continue
            key = (m.node.kind, m.node.id, layer)
            eid = str(entry.get("id"))
            if key in seen:
                errors.append(f"duplicate authoritative {layer} authority for "
                              f"{m.node.kind} '{m.node.id}' (entries '{seen[key]}' and '{eid}')")
            else:
                seen[key] = eid


def _check_superseded_refs(manifests: list[Manifest], errors: list[str]) -> None:
    known = {str(e.get("id")) for m in manifests for e in m.entries}
    known |= {(m.directory / str(e.get("document"))).as_posix()
              for m in manifests for e in m.entries}
    for m in manifests:
        for entry in m.entries:
            target = entry.get("superseded_by")
            if target and target not in known:
                errors.append(f"{m.label()}#{entry.get('id')}: superseded_by '{target}' "
                              "is not a known id or document path")


def _check_coverage(root: Path, manifests: list[Manifest], errors: list[str]) -> None:
    # Every non-README spec document must be registered by exactly one manifest
    # (uniqueness is enforced separately); governance scaffolding is exempt.
    registered = {(m.directory / str(e.get("document"))).as_posix()
                  for m in manifests for e in m.entries}
    specs_dir = root / "specs"
    for md in sorted(specs_dir.rglob("*.md")):
        if md.name in {"README.md", "SPEC_GOVERNANCE.md"}:
            continue
        rel = md.relative_to(root).as_posix()
        if rel not in registered:
            errors.append(f"spec not registered by any manifest: {rel}")
