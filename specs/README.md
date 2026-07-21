# specs/

This folder holds the project's specs: behavior contracts, architecture notes,
migration plans, investigations, runbooks, and reports. Specs are project
memory — they must stay trustworthy, not rot into stale Markdown.

## Governance

- **Rules:** [`SPEC_GOVERNANCE.md`](SPEC_GOVERNANCE.md) — lifecycle statuses,
  spec types, maturity levels, required fields, and review/PR workflow.
- **Registry:** [`SPEC_REGISTRY.json`](SPEC_REGISTRY.json) — machine-readable
  index of every spec with its status, domain, type, maturity, and links to the
  code/tests it governs. This is the source of truth for "what specs exist and
  how much do we trust them" — the registry lists the specs, so this README does
  not duplicate that list.

## Folder layout

Specs are organized **by domain/module**, like code — each domain owns its
specs under `specs/<domain>/` (e.g. `specs/telegram/`, `specs/facebook/`,
`specs/knowledge/`). Only the governance files live at the root:

```
specs/README.md
specs/SPEC_GOVERNANCE.md
specs/SPEC_REGISTRY.json
specs/<domain>/<SPEC>.md
```

Folders are **domain-first, never status-first** — do not create `active/`,
`draft/`, `stale/`, or `archived/` folders. A spec's status lives in
`SPEC_REGISTRY.json` (it changes over time); its domain folder is stable.
The registry remains the single source of truth for what specs exist.

## Status meanings (summary)

| status | meaning |
| --- | --- |
| `active` | currently trusted; guides implementation/refactor |
| `draft` | useful but incomplete; not yet binding |
| `unreviewed` | not yet audited under spec governance (default) |
| `stale` | likely outdated; do not trust without review |
| `deprecated` | intentionally retired, kept for history |
| `superseded` | replaced by another spec (`superseded_by`) |
| `archived` | historical/report-only |

## Adding a new spec

1. Add the Markdown file under its ownership node in `specs/domains/`.
2. Add a matching entry to that node's `SPEC_MANIFEST.json` (see
   `specs/registry/README.md` for the schema and workflow).
3. Run `python scripts/build_spec_registry.py` to regenerate
   `SPEC_REGISTRY.json` — it is generated output, never edited by hand.
4. Run the checker (below) and commit the manifest and the regenerated
   registry together.

## Updating an existing spec

- When a spec's metadata changes, update its `SPEC_MANIFEST.json` entry and
  regenerate the registry in the **same PR**.
- When marking a spec `superseded`, set `superseded_by` to the replacement's id.

## Running the registry checker

```bash
python scripts/check_spec_registry.py
```

It validates that the registry parses, every entry is well-formed, enum values
are valid, all registered paths exist, and every `specs/**/*.md` file (scanned
recursively across domain folders) is registered, except the root `README.md`
and `SPEC_GOVERNANCE.md`. Exit code is non-zero on failure.
