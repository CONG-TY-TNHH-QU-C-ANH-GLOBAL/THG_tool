"""Per-entry v2 metadata rules for specs/SPEC_REGISTRY.json (stdlib only).

An entry opts in with "metadata_version": 2 and must then satisfy the
ownership-domain taxonomy rules (see specs/domains/): layer-aware ownership
(experience-owned vs technical-feature-owned), cross-field governance, and
authoritative-uniqueness per ownership node. Legacy (v1) entries carry no v2
fields and are only guarded against stray v2 fields.
"""
import re

SLUG = re.compile(r"^[a-z0-9]+(-[a-z0-9]+)*$")

V2_ENUMS = {
    "layer": {"business", "experience", "technical", "implementation",
              "decision", "evidence", "runbook", "roadmap"},
    "authority": {"authoritative", "supporting", "historical"},
    "lifecycle": {"draft", "active", "superseded", "archived"},
    "implementation_state": {"proposed", "partial", "backed", "not_applicable"},
    "domain_kind": {"product", "product_platform", "platform"},
}
V2_REQUIRED = ["ownership_domain", "domain_kind", "layer", "authority",
               "lifecycle", "implementation_state", "effective"]
V2_FIELDS = set(V2_REQUIRED) | {"experience", "technical_feature",
                                "supported_experiences"}

EXPERIENCE_LAYERS = {"business", "experience"}
FEATURE_LAYERS = {"technical", "implementation", "decision", "evidence", "runbook"}


def _slug_ok(value) -> bool:
    return isinstance(value, str) and bool(SLUG.match(value))


def check_v2_entry(entry, label: str, errors: list[str]) -> None:
    version = entry.get("metadata_version")
    if version is None:
        stray = sorted(f for f in V2_FIELDS if f in entry)
        if stray:
            errors.append(f"{label}: v2 fields {stray} present without 'metadata_version': 2")
        return
    if version != 2:
        errors.append(f"{label}: unsupported metadata_version '{version}'")
        return
    for field in V2_REQUIRED:
        if field not in entry:
            errors.append(f"{label}: metadata_version 2 requires field '{field}'")
    for key, allowed in V2_ENUMS.items():
        if key in entry and entry[key] not in allowed:
            errors.append(f"{label}: invalid {key} '{entry[key]}'")
    if "effective" in entry and not isinstance(entry["effective"], bool):
        errors.append(f"{label}: 'effective' must be a boolean")
    _check_ownership(entry, label, errors)
    _check_governance(entry, label, errors)
    _check_supported_experiences(entry, label, errors)


def _check_ownership(entry, label: str, errors: list[str]) -> None:
    layer = entry.get("layer")
    experience = entry.get("experience")
    feature = entry.get("technical_feature")
    if experience is not None and feature is not None:
        errors.append(f"{label}: 'experience' and 'technical_feature' must not both be set")
        return
    if layer in EXPERIENCE_LAYERS:
        _check_experience_owned(layer, experience, feature, label, errors)
    elif layer in FEATURE_LAYERS:
        _check_feature_owned(layer, experience, feature, label, errors)
    else:
        _check_optional_ownership(experience, feature, label, errors)


def _check_experience_owned(layer, experience, feature, label: str, errors: list[str]) -> None:
    if not _slug_ok(experience):
        errors.append(f"{label}: layer '{layer}' requires 'experience' as a "
                      "lowercase kebab-case slug")
    if feature is not None:
        errors.append(f"{label}: layer '{layer}' must not set 'technical_feature'")


def _check_feature_owned(layer, experience, feature, label: str, errors: list[str]) -> None:
    if not _slug_ok(feature):
        errors.append(f"{label}: layer '{layer}' requires 'technical_feature' as a "
                      "lowercase kebab-case slug")
    if experience is not None:
        errors.append(f"{label}: layer '{layer}' must not set 'experience'")


def _check_optional_ownership(experience, feature, label: str, errors: list[str]) -> None:
    # roadmap: either node or neither, but any set value is a slug
    for key, value in (("experience", experience), ("technical_feature", feature)):
        if value is not None and not _slug_ok(value):
            errors.append(f"{label}: '{key}' must be a lowercase kebab-case slug")


def _check_governance(entry, label: str, errors: list[str]) -> None:
    if entry.get("authority") == "historical" and entry.get("effective") is not False:
        errors.append(f"{label}: authority 'historical' requires effective false")
    lifecycle = entry.get("lifecycle")
    if lifecycle in {"superseded", "archived"} and entry.get("effective") is not False:
        errors.append(f"{label}: lifecycle '{lifecycle}' requires effective false")
    layer = entry.get("layer")
    if layer in EXPERIENCE_LAYERS and entry.get("implementation_state") == "not_applicable":
        errors.append(f"{label}: layer '{layer}' requires implementation_state in "
                      "proposed/partial/backed")


def _check_supported_experiences(entry, label: str, errors: list[str]) -> None:
    supported = entry.get("supported_experiences")
    if supported is None:
        return
    if not entry.get("technical_feature"):
        errors.append(f"{label}: 'supported_experiences' is only allowed on "
                      "technical-feature-owned documents")
    if not isinstance(supported, list):
        errors.append(f"{label}: 'supported_experiences' must be an array")
        return
    bad = [x for x in supported if not _slug_ok(x)]
    if bad:
        errors.append(f"{label}: malformed supported_experiences slugs {bad}")
    if len(set(map(str, supported))) != len(supported):
        errors.append(f"{label}: duplicate supported_experiences values")


def _authoritative_contract_key(entry):
    """(layer, node) key when the entry is an authoritative+active+effective
    v2 contract on an ownership node (business/experience/technical), else None."""
    if not isinstance(entry, dict) or entry.get("metadata_version") != 2:
        return None
    if not (entry.get("authority") == "authoritative"
            and entry.get("lifecycle") == "active"
            and entry.get("effective") is True):
        return None
    layer = entry.get("layer")
    if layer in EXPERIENCE_LAYERS:
        node = entry.get("experience")
    elif layer == "technical":
        node = entry.get("technical_feature")
    else:
        return None
    if not isinstance(node, str) or not node:
        return None
    return layer, node


def check_v2_uniqueness(entries, errors: list[str]) -> None:
    """At most one authoritative+active+effective contract per ownership node
    for the business, experience, and technical layers. v2 entries only."""
    seen: dict[tuple[str, str], str] = {}
    for entry in entries:
        key = _authoritative_contract_key(entry)
        if key is None:
            continue
        if key in seen:
            layer, node = key
            errors.append(f"duplicate authoritative {layer} contract for '{node}' "
                          f"(entries '{seen[key]}' and '{entry.get('id')}')")
        else:
            seen[key] = entry.get("id")
