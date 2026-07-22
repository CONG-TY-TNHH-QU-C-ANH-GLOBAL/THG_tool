"""Data model + vocabulary for spec-registry manifests (Metadata V3)."""
from dataclasses import dataclass, field
from pathlib import Path

MANIFEST_SCHEMA_VERSION = 1
METADATA_VERSION = 3
MANIFEST_NAME = "SPEC_MANIFEST.json"

NODE_KINDS = ("domain", "experience", "technical_feature")

# Layers allowed per node kind (V3).
NODE_LAYERS = {
    "domain": {"domain", "roadmap", "decision", "evidence", "runbook", "implementation"},
    "experience": {"business", "experience", "roadmap", "decision", "evidence", "runbook"},
    "technical_feature": {"technical", "implementation", "decision", "evidence", "runbook", "roadmap"},
}

# One active/effective authoritative document per (node, layer) for these layers.
UNIQUE_AUTHORITY_LAYERS = {"domain", "business", "experience", "technical", "roadmap"}

AUTHORITY = {"authoritative", "supporting", "historical"}
LIFECYCLE = {"draft", "active", "superseded", "archived"}
IMPLEMENTATION_STATE = {"proposed", "partial", "backed", "not_applicable"}
DOMAIN_KIND = {"product", "product_platform", "platform"}

# Legacy compatibility vocabulary preserved for existing registry consumers.
COMPAT_STATUS = {"active", "draft", "unreviewed", "stale", "deprecated", "superseded", "archived"}
COMPAT_TYPE = {"behavior", "architecture", "contract", "migration", "investigation",
               "runbook", "report", "roadmap", "policy", "unknown"}
COMPAT_MATURITY = {"snapshot", "reviewed", "test_pinned", "implementation_backed",
                   "superseded", "unknown"}

# Fields every manifest entry must provide (compatibility carries the rest).
ENTRY_REQUIRED = (
    "id", "document", "title", "metadata_version", "layer", "authority",
    "lifecycle", "implementation_state", "effective", "owner", "last_reviewed",
    "verified_against", "related_code", "related_tests", "supersedes",
    "superseded_by", "tags", "notes", "compatibility",
)
COMPAT_REQUIRED = ("domain", "type", "status", "maturity")

# Node-level fields that must never be repeated inside an entry.
NODE_INHERITED_FIELDS = ("ownership_domain", "domain_kind", "node_kind", "node_id",
                         "supported_experiences", "experience", "technical_feature")


@dataclass(frozen=True)
class Node:
    kind: str
    id: str
    ownership_domain: str
    domain_kind: str
    supported_experiences: tuple[str, ...] = ()


@dataclass(frozen=True)
class Manifest:
    path: Path            # the SPEC_MANIFEST.json file
    directory: Path       # repo-relative node directory
    node: Node
    entries: tuple[dict, ...] = field(default_factory=tuple)

    def label(self) -> str:
        return self.path.as_posix()


def expected_node_for_directory(directory: Path) -> tuple[str, str, str] | None:
    """(kind, node_id, ownership_domain) implied by a manifest's directory,
    or None when the directory is not a valid ownership-node location."""
    parts = directory.parts
    if len(parts) < 3 or parts[0] != "specs" or parts[1] != "domains":
        return None
    domain = parts[2]
    if len(parts) == 3:
        return "domain", domain, domain
    if len(parts) == 5 and parts[3] == "experiences":
        return "experience", parts[4], domain
    if len(parts) == 5 and parts[3] == "features":
        return "technical_feature", parts[4], domain
    return None
