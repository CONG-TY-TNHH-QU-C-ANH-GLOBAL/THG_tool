# Feature: browser-connector

The visible, operator-observable Chrome extension connector and the
channel-neutral automation core it hosts (`local-connector-extension/`,
`internal/server/agent` connector APIs). No hidden browser pools; login
walls/checkpoints return `human_required`.

- [technical.md](technical.md) — channel-neutral automation platform core +
  Facebook adapter scaffolding (unit-tested, additive; not yet wired into
  manifest.json). Implementation state: **backed**.
- [runbooks/connector-production-workflow.md](runbooks/connector-production-workflow.md)
  — Chrome Web Store install/pairing production workflow.
- [implementation/automation-kit-refactor.md](implementation/automation-kit-refactor.md)
  — BACKLOG/DESIGN: layered multi-platform automation kit refactor (proposed).
- [decisions/browser-gateway-vision.md](decisions/browser-gateway-vision.md)
  — aspirational browser-gateway direction; the realized path is the
  extension connector (draft).
