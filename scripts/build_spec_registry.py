"""Generate specs/SPEC_REGISTRY.json from SPEC_MANIFEST.json sources.

Usage:
  python scripts/build_spec_registry.py          # validate manifests + write registry
  python scripts/build_spec_registry.py --check  # fail when committed registry is stale

The committed registry is a generated projection for existing consumers.
Never edit it manually; edit the owning SPEC_MANIFEST.json and re-run this.
"""
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parent))

from spec_registry.discovery import load_manifests  # noqa: E402
from spec_registry.generation import drift_errors, generate_registry, write_registry  # noqa: E402
from spec_registry.validation import validate_manifests  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]


def build(check_only: bool) -> int:
    errors: list[str] = []
    manifests = load_manifests(ROOT, errors)
    validate_manifests(ROOT, manifests, errors)
    if errors:
        _report(errors)
        return 1
    registry = generate_registry(manifests)
    if check_only:
        errors = drift_errors(ROOT, registry)
        if errors:
            _report(errors)
            return 1
        print(f"registry up to date ({len(registry['entries'])} entries from "
              f"{len(manifests)} manifests)")
        return 0
    write_registry(ROOT, registry)
    print(f"wrote specs/SPEC_REGISTRY.json ({len(registry['entries'])} entries from "
          f"{len(manifests)} manifests)")
    return 0


def _report(errors: list[str]) -> None:
    print(f"FAIL: {len(errors)} problem(s) found:")
    for err in errors:
        print(f"  - {err}")


def main() -> int:
    args = sys.argv[1:]
    if args not in ([], ["--check"]):
        print(__doc__)
        return 2
    return build(check_only=bool(args))


if __name__ == "__main__":
    sys.exit(main())
