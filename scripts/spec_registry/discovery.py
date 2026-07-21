"""Find and parse SPEC_MANIFEST.json files under specs/domains/."""
import json
from pathlib import Path

from .model import MANIFEST_NAME, MANIFEST_SCHEMA_VERSION, Manifest, Node


def find_manifest_paths(root: Path) -> list[Path]:
    return sorted((root / "specs" / "domains").rglob(MANIFEST_NAME))


def load_manifests(root: Path, errors: list[str]) -> list[Manifest]:
    manifests = []
    for path in find_manifest_paths(root):
        manifest = _load_one(root, path, errors)
        if manifest is not None:
            manifests.append(manifest)
    return manifests


def _load_one(root: Path, path: Path, errors: list[str]) -> Manifest | None:
    label = path.relative_to(root).as_posix()
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        errors.append(f"{label}: unreadable manifest: {exc}")
        return None
    if not isinstance(data, dict):
        errors.append(f"{label}: manifest root must be a JSON object")
        return None
    if data.get("manifest_schema_version") != MANIFEST_SCHEMA_VERSION:
        errors.append(f"{label}: manifest_schema_version must be {MANIFEST_SCHEMA_VERSION}")
        return None
    node = _parse_node(data.get("node"), label, errors)
    entries = data.get("entries")
    if node is None or not isinstance(entries, list):
        if not isinstance(entries, list):
            errors.append(f"{label}: 'entries' must be a list")
        return None
    bad = [e for e in entries if not isinstance(e, dict)]
    if bad:
        errors.append(f"{label}: every entry must be a JSON object")
        return None
    return Manifest(path=path.relative_to(root), directory=path.relative_to(root).parent,
                    node=node, entries=tuple(entries))


def _parse_node(raw, label: str, errors: list[str]) -> Node | None:
    if not isinstance(raw, dict):
        errors.append(f"{label}: 'node' must be a JSON object")
        return None
    missing = [k for k in ("kind", "id", "ownership_domain", "domain_kind") if k not in raw]
    if missing:
        errors.append(f"{label}: node missing fields {missing}")
        return None
    supported = raw.get("supported_experiences", [])
    if not isinstance(supported, list) or any(not isinstance(s, str) for s in supported):
        errors.append(f"{label}: node.supported_experiences must be a list of strings")
        return None
    return Node(kind=raw["kind"], id=raw["id"], ownership_domain=raw["ownership_domain"],
                domain_kind=raw["domain_kind"], supported_experiences=tuple(supported))
