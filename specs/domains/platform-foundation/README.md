# Domain: platform-foundation

Ownership domain (kind: **platform**) — the runtime, tenancy, storage, and
structural foundations every product domain builds on. Facebook is only the
first surface; nothing here may become Facebook-only.

Structure: `features/` hold technical contracts and their implementation /
decisions / evidence / runbooks; domain-level `decisions/` hold binding
structural rules that span features; `DOMAIN.md` is the canonical platform
vocabulary (mandatory reading — synonyms are rejected on review).

## Vocabulary

- [DOMAIN.md](DOMAIN.md) — canonical entities (Platform, User, Service,
  Workspace, Membership, Capability, Session, Worker, AutomationJob),
  identity rules, storage-vs-domain separation.

## Features

- [runtime-topology](features/runtime-topology/README.md) — current runtime
  composition and store-domain boundaries, script-enforced by
  `scripts/check_topology.sh` (CI).
- [browser-connector](features/browser-connector/README.md) — visible Chrome
  extension connector, channel-neutral automation core, production workflow.
- [ai-cost-controls](features/ai-cost-controls/README.md) — Phase-1 LLM cost
  controls: usage logs, bounded cache, token capture.
- [workspace-ui](features/workspace-ui/README.md) — workspace frontend plans
  and mock-era evidence (no authoritative technical contract yet).

## Decisions

- [component-structure-rules](decisions/component-structure-rules.md) —
  ACTIVE_BINDING component/package structure rules, enforced warn-only by
  `scripts/check_component_structure.py`.

## Not yet migrated

The rest of this domain's specs still live in the flat legacy folders
(`specs/platform/`, `specs/store/`, `specs/browser/`, `specs/frontend/`,
`specs/ai/`, `specs/migration/`) and are tracked in
[`specs/SPEC_REGISTRY.json`](../../SPEC_REGISTRY.json). They migrate here in
later batches.
