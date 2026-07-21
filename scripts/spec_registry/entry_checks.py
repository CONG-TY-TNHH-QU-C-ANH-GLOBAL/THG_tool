"""Per-entry validation for SPEC_MANIFEST.json entries (Metadata V3)."""
from pathlib import Path, PurePosixPath
import re

from .model import (AUTHORITY, COMPAT_MATURITY, COMPAT_REQUIRED, COMPAT_STATUS,
                    COMPAT_TYPE, ENTRY_REQUIRED, IMPLEMENTATION_STATE, LIFECYCLE,
                    METADATA_VERSION, NODE_INHERITED_FIELDS, NODE_LAYERS, Manifest)

SLUG = re.compile(r"^[a-z0-9]+(-[a-z0-9]+)*$")


def check_entry(root: Path, m: Manifest, entry: dict, errors: list[str]) -> None:
    eid = entry.get("id", "<missing id>")
    label = f"{m.label()}#{eid}"
    for field in ENTRY_REQUIRED:
        if field not in entry:
            errors.append(f"{label}: missing required field '{field}'")
    for field in NODE_INHERITED_FIELDS:
        if field in entry:
            errors.append(f"{label}: node-inherited field '{field}' must not appear in an entry")
    if isinstance(eid, str) and not SLUG.match(eid):
        errors.append(f"{label}: entry id must be a kebab-case slug")
    if entry.get("metadata_version") != METADATA_VERSION:
        errors.append(f"{label}: metadata_version must be {METADATA_VERSION}")
    check_entry_enums(m, entry, label, errors)
    check_entry_governance(entry, label, errors)
    check_entry_document(root, m, entry, label, errors)
    check_entry_compat(entry, label, errors)


def check_entry_enums(m: Manifest, entry: dict, label: str, errors: list[str]) -> None:
    layer = entry.get("layer")
    allowed = NODE_LAYERS.get(m.node.kind, set())
    if layer not in allowed:
        errors.append(f"{label}: layer '{layer}' is not valid for a {m.node.kind} node")
    for field, allowed_values in (("authority", AUTHORITY), ("lifecycle", LIFECYCLE),
                                  ("implementation_state", IMPLEMENTATION_STATE)):
        if field in entry and entry[field] not in allowed_values:
            errors.append(f"{label}: invalid {field} '{entry[field]}'")
    if "effective" in entry and not isinstance(entry["effective"], bool):
        errors.append(f"{label}: 'effective' must be a boolean")


def check_entry_governance(entry: dict, label: str, errors: list[str]) -> None:
    if entry.get("authority") == "historical" and entry.get("effective") is not False:
        errors.append(f"{label}: authority 'historical' requires effective false")
    if entry.get("lifecycle") in {"superseded", "archived"} and entry.get("effective") is not False:
        errors.append(f"{label}: lifecycle '{entry.get('lifecycle')}' requires effective false")
    if entry.get("layer") in {"business", "experience"} \
            and entry.get("implementation_state") == "not_applicable":
        errors.append(f"{label}: layer '{entry.get('layer')}' requires implementation_state "
                      "in proposed/partial/backed")


def check_entry_document(root: Path, m: Manifest, entry: dict, label: str,
                         errors: list[str]) -> None:
    document = entry.get("document")
    if not isinstance(document, str) or not document:
        errors.append(f"{label}: 'document' must be a non-empty relative path")
        return
    pure = PurePosixPath(document)
    if pure.is_absolute() or ".." in pure.parts:
        errors.append(f"{label}: document '{document}' escapes the manifest node")
        return
    if not (root / m.directory / pure).is_file():
        errors.append(f"{label}: document does not exist: {(m.directory / pure).as_posix()}")


def check_entry_compat(entry: dict, label: str, errors: list[str]) -> None:
    compat = entry.get("compatibility")
    if not isinstance(compat, dict):
        if "compatibility" in entry:
            errors.append(f"{label}: 'compatibility' must be a JSON object")
        return
    for field in COMPAT_REQUIRED:
        if field not in compat:
            errors.append(f"{label}: compatibility missing field '{field}'")
    for field, allowed in (("status", COMPAT_STATUS), ("type", COMPAT_TYPE),
                           ("maturity", COMPAT_MATURITY)):
        if field in compat and compat[field] not in allowed:
            errors.append(f"{label}: invalid compatibility {field} '{compat[field]}'")
