# Spec Registry — Ownership Manifests (Metadata V3)

Canonical spec-registry metadata lives in small `SPEC_MANIFEST.json` files
colocated with the ownership node that owns each document:

- `specs/domains/<domain>/SPEC_MANIFEST.json` — domain node (DOMAIN.md,
  roadmap, domain-level decisions/evidence);
- `specs/domains/<domain>/experiences/<experience>/SPEC_MANIFEST.json`;
- `specs/domains/<domain>/features/<feature>/SPEC_MANIFEST.json`.

`specs/SPEC_REGISTRY.json` is a **generated, committed projection** for
existing consumers. **Never edit it manually.** The workflow:

1. Edit the nearest ownership-node `SPEC_MANIFEST.json`.
2. `python scripts/build_spec_registry.py` (regenerates the aggregate).
3. `python scripts/check_spec_registry.py` (validates manifests + drift).
4. Commit the manifest **and** the regenerated registry together.

CI runs the checker; a stale or hand-edited aggregate fails the build.

## Manifest shape

Contract: [spec-manifest.schema.json](spec-manifest.schema.json)
(`manifest_schema_version: 1`). Executable validation lives in
`scripts/spec_registry/` — the schema file is the reviewable reference, not a
second validator.

Rules:

- `document` is relative to the manifest directory; `path` in the generated
  registry is derived (`manifest dir + document`). Never hand-write full paths.
- Node identity (`kind`, `id`, `ownership_domain`, `domain_kind`,
  `supported_experiences`) lives once in `node` and is inherited by every
  entry; repeating an inherited field inside an entry is an error.
- A manifest may only register documents inside its own node directory
  (`../` escapes are rejected). A directory with no registered documents has
  no manifest. `README.md` navigation files are not registered.
- `compatibility` preserves the legacy flattened fields (`domain`, `type`,
  `status`, `maturity`) that existing registry consumers read. They are
  compatibility metadata, not authority semantics.

## Metadata V3 semantics

Node kinds: `domain`, `experience`, `technical_feature`. Layers per kind:

| node kind | layers |
|---|---|
| domain | domain, roadmap, decision, evidence, runbook, implementation |
| experience | business, experience, roadmap, decision, evidence, runbook |
| technical_feature | technical, implementation, decision, evidence, runbook, roadmap |

Authority semantics (unchanged from V2):

- **Normative authority** = `authority: authoritative` + `lifecycle: active`
  + `effective: true`.
- **Runtime authority** = normative + `implementation_state: backed`.
- `historical` authority and `superseded`/`archived` lifecycle require
  `effective: false`.
- At most one active/effective authoritative document per node for the
  `domain`, `business`, `experience`, `technical`, and `roadmap` layers.
  Supporting decision/evidence/runbook/implementation documents may be many.

## Tooling

- `python scripts/build_spec_registry.py` — validate + regenerate.
- `python scripts/build_spec_registry.py --check` — fail on drift (CI-safe).
- `python scripts/check_spec_registry.py` — full validation + drift (in CI).
- `python scripts/test_check_spec_registry.py` and
  `python scripts/test_build_spec_registry.py` — regression harnesses.

Generation is deterministic: entries sorted by id, `updated_at` is the
maximum `last_reviewed` across entries, stable two-space JSON with LF and a
trailing newline. No wall-clock input.
