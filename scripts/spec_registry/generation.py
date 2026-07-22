"""Deterministic generation of specs/SPEC_REGISTRY.json from manifests."""
import json
import os
import re
import tempfile
from pathlib import Path

from .model import METADATA_VERSION, Manifest

REGISTRY_RELATIVE = Path("specs/SPEC_REGISTRY.json")
GENERATED_FROM = "specs/domains/**/SPEC_MANIFEST.json"
DATE = re.compile(r"^\d{4}-\d{2}-\d{2}$")


def generate_registry(manifests: list[Manifest]) -> dict:
    entries = sorted((_flatten(m, e) for m in manifests for e in m.entries),
                     key=lambda e: e["id"])
    return {
        "schema_version": 1,
        "generated": True,
        "generated_from": GENERATED_FROM,
        "updated_at": _updated_at(entries),
        "entries": entries,
    }


def _updated_at(entries: list[dict]) -> str:
    dates = [e["last_reviewed"] for e in entries
             if isinstance(e.get("last_reviewed"), str) and DATE.match(e["last_reviewed"])]
    return max(dates) if dates else "unknown"


def _flatten(m: Manifest, entry: dict) -> dict:
    compat = entry.get("compatibility") or {}
    flat = {
        "id": str(entry.get("id")),
        "path": (m.directory / str(entry.get("document"))).as_posix(),
        "title": entry.get("title"),
        "domain": compat.get("domain"),
        "type": compat.get("type"),
        "status": compat.get("status"),
        "maturity": compat.get("maturity"),
        "metadata_version": METADATA_VERSION,
        "ownership_domain": m.node.ownership_domain,
        "domain_kind": m.node.domain_kind,
        "node_kind": m.node.kind,
        "node_id": m.node.id,
        "layer": entry.get("layer"),
        "authority": entry.get("authority"),
        "lifecycle": entry.get("lifecycle"),
        "implementation_state": entry.get("implementation_state"),
        "effective": entry.get("effective"),
    }
    if m.node.kind == "technical_feature":
        flat["supported_experiences"] = list(m.node.supported_experiences)
    flat.update({
        "owner": entry.get("owner"),
        "last_reviewed": entry.get("last_reviewed"),
        "verified_against": entry.get("verified_against"),
        "related_code": entry.get("related_code"),
        "related_tests": entry.get("related_tests"),
        "supersedes": entry.get("supersedes"),
        "superseded_by": entry.get("superseded_by"),
        "tags": entry.get("tags"),
        "notes": entry.get("notes"),
    })
    return flat


def render(registry: dict) -> str:
    return json.dumps(registry, ensure_ascii=False, indent=2) + "\n"


def write_registry(root: Path, registry: dict) -> None:
    target = root / REGISTRY_RELATIVE
    body = render(registry).encode("utf-8")
    fd, tmp = tempfile.mkstemp(dir=target.parent, prefix=".spec_registry_", suffix=".tmp")
    try:
        with os.fdopen(fd, "wb") as handle:
            handle.write(body)
        os.replace(tmp, target)
    except OSError:
        Path(tmp).unlink(missing_ok=True)
        raise


def committed_registry_text(root: Path) -> str | None:
    target = root / REGISTRY_RELATIVE
    if not target.is_file():
        return None
    return target.read_text(encoding="utf-8")


def drift_errors(root: Path, registry: dict) -> list[str]:
    """Empty when the committed aggregate matches the manifest-derived output."""
    committed = committed_registry_text(root)
    if committed is None:
        return [f"{REGISTRY_RELATIVE.as_posix()} is missing — run "
                "`python scripts/build_spec_registry.py`"]
    if committed != render(registry):
        return [f"{REGISTRY_RELATIVE.as_posix()} is stale or hand-edited — regenerate with "
                "`python scripts/build_spec_registry.py` (never edit it manually; edit the "
                "owning SPEC_MANIFEST.json instead)"]
    return []
