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

Most existing specs are currently `unreviewed`; they will be classified in small
batches by later PRs (PR24B+).

## Adding a new spec

1. Add the Markdown file under `specs/`.
2. Add a matching entry to `SPEC_REGISTRY.json` (all required fields — see
   `SPEC_GOVERNANCE.md` §5). New documents default to `status: "unreviewed"`,
   `maturity: "unknown"`.
3. Run the checker (below) and commit both files together.

## Updating an existing spec

- When a spec's status, maturity, or domain changes, update its registry entry
  in the **same PR**.
- When marking a spec `superseded`, set `superseded_by` to the replacement's id.

## Running the registry checker

```bash
python scripts/check_spec_registry.py
```

It validates that the registry parses, every entry is well-formed, enum values
are valid, all registered paths exist, and every `specs/*.md` file is registered
(except `README.md` and `SPEC_GOVERNANCE.md`). Exit code is non-zero on failure.
