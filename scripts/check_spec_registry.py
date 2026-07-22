"""Spec registry drift guard (stdlib only).

Canonical registry metadata lives in per-ownership-node SPEC_MANIFEST.json
files under specs/domains/ (see specs/registry/README.md). This checker:

  - validates every manifest (structure, Metadata V3, ownership, governance,
    authority uniqueness, document existence, exactly-once coverage);
  - regenerates the aggregate in memory and fails when the committed
    specs/SPEC_REGISTRY.json is stale or hand-edited (drift).

specs/SPEC_REGISTRY.json is generated output — never edit it manually; edit
the owning SPEC_MANIFEST.json and run `python scripts/build_spec_registry.py`.

Exit 0 on PASS, non-zero on FAIL.
"""
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parent))

from spec_registry.discovery import load_manifests  # noqa: E402
from spec_registry.generation import drift_errors, generate_registry  # noqa: E402
from spec_registry.validation import validate_manifests  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]


def main() -> int:
    errors: list[str] = []
    manifests = load_manifests(ROOT, errors)
    validate_manifests(ROOT, manifests, errors)
    if not errors:
        registry = generate_registry(manifests)
        errors.extend(drift_errors(ROOT, registry))
        print(f"registry entries: {len(registry['entries'])}")

    if errors:
        print(f"FAIL: {len(errors)} problem(s) found:")
        for err in errors:
            print(f"  - {err}")
        print("RESULT: FAIL")
        return 1

    print("RESULT: PASS - manifests are valid and specs/SPEC_REGISTRY.json is in sync.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
