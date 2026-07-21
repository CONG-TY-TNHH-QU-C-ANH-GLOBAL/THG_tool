# Browser Gateway And Facebook Automation Vision

Status: active architecture direction.

This document defines the browser strategy for THG AutoFlow after reviewing
Hermes Browser Automation. The goal is not to copy Hermes directly. The goal is
to adopt the right architectural lesson: agents should talk to a stable Browser
Gateway, not to one hardcoded browser implementation.

## Product Vision

THG AutoFlow is a Facebook Sales Intelligence Browser OS for businesses.

The customer value is not "scrape every group". The customer value is:

1. Understand the organization's market, offer, sales voice and reject rules.
2. Discover and monitor relevant Facebook sources.
3. Extract market signals with evidence.
4. Classify leads into usable customer opportunities.
5. Execute comments, inbox messages and posts with dedup/cooldown/conversation
   guardrails.
6. Show every meaningful automation step in the Browser dashboard and
   Telegram/Dashboard logs.

## Hermes Lessons We Should Adopt

Hermes exposes multiple browser providers behind one tool contract:

- cloud browsers
- local browser sidecars
- local Chrome via CDP
- browser snapshots
- click/type/scroll actions
- vision/console/CDP escape hatches

For THG, the important pattern is the Browser Gateway abstraction:

```text
Agent Brain
  -> action_plan / skill call
  -> Browser Gateway
  -> Provider implementation
  -> Evidence + result + audit log
```

The agent should not care whether the provider is Chrome Extension, cloud
browser, or a future internal worker. It should care about capabilities.

## What We Should Not Copy Directly

- Do not use cloud browser login as the core Facebook account path. Facebook
  trust is strongest on the user's real Chrome/device/profile.
- Do not expose raw CDP or unrestricted JavaScript evaluation to frontend,
  LLM, Telegram, or external prompts.
- Do not build a generic browser agent that answers anything. THG's agent is
  Facebook-scoped.
- Do not rely on pixel-coordinate automation as the primary execution model.
  Pixel stream is for observability. Execution should prefer semantic DOM /
  accessibility targets and evidence.

## Provider Decision

Production core provider:

```text
ChromeExtensionFacebookProvider
```

Why:

- Uses the user's trusted Chrome profile and IP.
- Avoids asking for Facebook passwords inside THG.
- Keeps checkpoint/CAPTCHA human-owned.
- Can stream the real Facebook tab to the Browser dashboard.
- Can run crawl/outbox actions from inside the same logged-in context.
- Lower customer risk than a downloaded desktop executable.

Retired provider:

```text
DesktopLocalRuntimeProvider
```

The old desktop runtime path should be removed from UI, CI, docs, and active
server execution. Existing DB tables can remain for migration compatibility,
but new automation must not depend on `cmd/thg-login` or `/api/agent/jobs/next`.

Future optional providers:

- `PublicWebDiscoveryProvider`: no-login public source discovery.
- `CloudBrowserResearchProvider`: public pages that do not require Facebook
  account session.
- `ManagedEnterpriseDeviceProvider`: dedicated always-on customer device or VM
  running Chrome Extension for 24/7 workloads.

## Browser Gateway Contract

The internal contract should be capability-first:

```go
type ProviderCapability string

const (
  CapabilityStreamFrames ProviderCapability = "stream_frames"
  CapabilityFacebookIdentity ProviderCapability = "facebook_identity"
  CapabilitySemanticSnapshot ProviderCapability = "semantic_snapshot"
  CapabilityCrawlVisiblePosts ProviderCapability = "crawl_visible_posts"
  CapabilityExecuteOutbound ProviderCapability = "execute_outbound"
  CapabilityFanpageInbox ProviderCapability = "fanpage_inbox"
)
```

Every provider response should include:

- `provider`
- `account_id`
- `org_id`
- `status`
- `current_url`
- `capabilities`
- `evidence`
- `error_code`
- `error_message`

## Agent Brain Flow

```text
Dashboard chat / Telegram
  -> Agent Brain validates Facebook scope
  -> Business profile preflight
  -> Browser readiness preflight
  -> Skill registry
  -> Browser Gateway command
  -> Connector command / outbox / scheduler
  -> Result + evidence
  -> Dashboard and Telegram log
```

Rules:

- If the prompt is not Facebook-related, refuse or redirect briefly.
- If business calibration is missing for lead discovery, ask for calibration.
- If Facebook session is missing, return a professional Browser setup message.
- If action is outbound, queue through `QueueOutboundForOrg`.
- If auto mode is not enabled, the store downgrades to draft.
- If auto mode is enabled, the extension may execute approved outbox rows.
- The user should not see "draft-first" as the product promise. The product
  promise is action with guardrails and observability.

## Execution Model

### Crawl

```text
Prompt -> Market Signal Gate -> connector_commands(type=crawl)
Chrome Extension -> content script extracts visible posts
Backend -> scoring/classification -> task_leads + leads
Dashboard -> counts, categories, evidence
```

### Comment / Inbox / Post

```text
Prompt or recurring campaign
  -> lead selection
  -> Sales Voice Memory
  -> QueueOutboundForOrg
  -> approved outbox when org auto mode allows
  -> Chrome Extension polls /api/connectors/outbox
  -> content script executes on Facebook
  -> mark sent/failed
  -> Telegram/Dashboard log
```

### Stream

Stream frames are for user trust and audit. They should not be the only source
of truth for actions.

Preferred action order:

1. semantic DOM target
2. structured content script helper
3. visible text/ARIA selector
4. pixel fallback only when necessary

## 24/7 Reality

Chrome Extension automation only runs while the user's Chrome/device is online.
For 24/7 service tiers, THG should offer:

- staff device always online
- dedicated customer mini PC / VM
- managed enterprise device with Chrome Extension installed

The dashboard must be honest about offline state. Do not fake "running" when no
connector can execute.

## Refactor Boundary

Remove or retire:

- `cmd/thg-login`
- desktop kit download UI
- scripts that package desktop runtime
- CI steps that build desktop runtime
- docs that instruct users to install desktop runtime
- `/api/agent/jobs/next` active execution path

Keep and strengthen:

- `local-connector-extension/`
- `/api/connectors/*`
- `connector_commands`
- `/api/connectors/outbox`
- `QueueOutboundForOrg`
- `applyConnectorIdentity`
- `ConnectorOwnsAccountStream`
- Browser dashboard stream components

Add next:

- `internal/browsergateway/` for provider constants and contracts.
- shared frontend copy/types for Chrome Extension setup.
- semantic snapshot protocol in extension content scripts.
- capability checks before the agent queues crawl/outbound work.

## Non-Negotiables

- Every record is org-scoped.
- One workspace can manage many Facebook accounts.
- One connector token can be bound to one account slot.
- A connector cannot execute work for accounts it does not own.
- Facebook profile mismatch returns a hard 409.
- No raw cookies/passwords are exposed to frontend or LLM.
- No broad scan-all default jobs.
- No hardcoded verticals.
- No fake UI data.
