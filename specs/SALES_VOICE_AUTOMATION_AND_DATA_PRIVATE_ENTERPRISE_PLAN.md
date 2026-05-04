# Sales Voice Automation and Enterprise Data Private Plan

Last updated: 2026-05-04

Audience: Claude Code, Codex, and future engineering agents.

This document clarifies the next product direction after Phase 6 Open-Prompt
Agent. The system must not stop at "AI drafts a comment". The target product is:

> AI Facebook Sales Employee for each organization, trained by that
> organization's private business data, real sales style, and market outcomes.

Automation should feel like a professional sales operator working inside the
workspace, not a chatbot that merely suggests text.

## 1. Product Thesis

The customer does not pay for a scraper, and they do not pay for a pile of AI
drafts.

The customer pays for:

- finding real market demand on Facebook
- filtering out spam, provider ads, competitor posts, and weak leads
- understanding which leads are worth action
- replying/commenting/inboxing with the tone and business logic of their sales
  team
- doing this repeatedly, safely, and observably

The core loop is:

```text
Business data + sales examples
  -> Sales Voice Profile
  -> Market Signal Gate
  -> Crawl/classify leads
  -> Conversation/action decision
  -> Runtime executes comment/inbox/post on Facebook
  -> Dashboard/Telegram logs action + outcome
  -> Outcome updates memory and future messaging
```

## 2. Key Principle: Draft Is Not The Product

Drafts are an internal safety state, not the main user experience.

User-facing product behavior should be:

- "Agent found 20 leads, contacted 7 high-confidence leads, skipped 13 with
  reasons."
- "Agent commented this message on lead X because the post matched these buying
  signals."
- "Lead replied. Agent switched to customer-service mode and answered with
  thread context."

Not:

- "Here are 20 drafts waiting forever."

Draft is allowed only when one of these is true:

- the organization has not enabled auto-execution
- lead confidence is below the configured action threshold
- business profile or sales voice is not strong enough
- required private data is missing
- account/session is not ready
- Facebook checkpoint or login wall appears
- the action hits cooldown/dedup/thread guardrails

When auto-execution is enabled and guardrails pass, approved outbound rows are
runtime work items, not content suggestions.

## 3. Sales Voice Profile

Add a first-class organization-level "Sales Voice Profile". This is different
from business profile.

Business profile answers:

- Who are we?
- What do we sell?
- Who do we target?
- What signals/reject rules matter?

Sales Voice Profile answers:

- How do our best salespeople talk?
- How direct or consultative are we?
- How do we open a comment?
- How do we open an inbox?
- What CTAs do we use?
- What phrases must never be used?
- How do we answer pricing, shipping, lead time, trust, and objections?

### Inputs

Data Private should accept these real inputs:

- comment examples from sales team
- inbox conversation examples
- winning scripts
- failed scripts
- pricing sheets
- FAQs
- objection-handling notes
- product/service docs
- brand tone rules
- customer personas
- screenshots or documents uploaded by the org

### Derived Fields

Store or summarize into org-scoped memory:

- `org:{id}:sales_voice_summary`
- `org:{id}:comment_style_rules`
- `org:{id}:inbox_style_rules`
- `org:{id}:objection_handling`
- `org:{id}:cta_rules`
- `org:{id}:forbidden_phrases`
- `org:{id}:pricing_summary`
- `org:{id}:sales_examples_summary`

Do not dump raw large files into prompts. Retrieve/summarize.

## 4. Agent Decision Model

For every candidate lead, the agent should decide:

```text
Should act?
  -> yes/no

Which action?
  -> comment | inbox | both | skip | human_required

Why?
  -> evidence from post + business profile + sales policy

What message?
  -> generated from sales voice + business data + lead context

What guardrail result?
  -> allowed | duplicate | cooldown | low_confidence | missing_context
```

Required structured output for action planning:

```json
{
  "lead_id": 123,
  "action": "comment",
  "decision": "execute",
  "confidence": 0.86,
  "evidence": ["asking for fulfillment", "ships to US", "buyer intent"],
  "reject_reason": "",
  "message": "Natural comment using org sales voice",
  "guardrails": {
    "dedup": "ok",
    "cooldown": "ok",
    "thread_state": "no_prior_thread",
    "account_ready": true
  }
}
```

The Go control plane must still own:

- org/account injection
- account ownership checks
- browser/session readiness
- max action caps
- cooldown/dedup/thread guardrails
- final execution state

The AI/Brain proposes reasoning and message. Go decides whether it may execute.

## 5. Outbound Execution Pipeline

Current gap: `QueueOutboundForOrg` and `/api/agent/outbox` exist, but THG Local
Runtime must consume approved outbound items and perform real Facebook actions.

Target pipeline:

```text
Prompt from Dashboard/Telegram
  -> Agent resolves skill/action
  -> Crawl command runs in THG Local Runtime
  -> Results stored as leads
  -> Market Signal Gate accepts high-confidence leads
  -> Sales Voice generator creates comment/inbox/post content
  -> QueueOutboundForOrg applies guardrails
  -> if org outbound_mode=auto and guardrails pass: status=approved
  -> THG Local Runtime polls approved outbox
  -> Runtime opens target Facebook post/profile/page
  -> Runtime performs comment/inbox/post
  -> Runtime marks sent/failed
  -> Telegram + Dashboard log the exact action and outcome
```

### Runtime Responsibilities

THG Local Runtime should:

- poll approved outbox for its org/account
- execute only rows matching an active logged-in Facebook account
- perform supported actions:
  - `comment`
  - `inbox`
  - `group_post`
  - later: `profile_post`, `fanpage_post`, `fanpage_inbox`
- mark `/api/agent/outbox/:id/sent` or `/failed`
- include clear error categories:
  - `facebook_login_required`
  - `facebook_checkpoint`
  - `target_not_found`
  - `composer_not_found`
  - `send_button_not_found`
  - `permission_denied`
  - `timeout`

Runtime must not invent content. It executes the server-approved message.

## 6. Enterprise Data Private Redesign

The current Data Private screen is functionally useful but does not yet feel
enterprise-grade. It is a stack of panels. It should become an operating center
for AI memory, governance, and data readiness.

Target positioning:

> Data Private is the organization's AI operating memory.

It should look and behave closer to a big-tech admin product:

- clear hierarchy
- dense but calm information
- API-backed state only
- auditability
- source trust levels
- readiness score
- data lineage
- role-based governance
- no fake metrics

### Information Architecture

Replace one long vertical page with a workspace-grade layout:

```text
Header
  - Data readiness score
  - Last sync
  - Agent context health
  - Auto-action eligibility

Left rail / tabs
  1. Overview
  2. Business Profile
  3. Sales Voice
  4. Knowledge Sources
  5. Pricing & Offers
  6. Customers & Segments
  7. Rules & Guardrails
  8. Audit & Learning
```

### 6.1 Overview

Show real readiness, not decoration:

- Business profile completeness
- Sales voice completeness
- Pricing/offer data present
- Reject rules present
- Connected sources count
- Last successful sync
- Agent can auto-act: yes/no and why

Example cards:

- "Business profile: 82% complete"
- "Sales voice: needs 5 more examples"
- "Pricing: 1 Google Sheet synced"
- "Auto-action: disabled by policy"
- "Risk: no negative-signal rules configured"

### 6.2 Business Profile

Structured, guided, not one giant form:

- Brand identity
- Offer/services
- Markets
- Ideal customers
- Positive buying signals
- Negative/reject signals
- Competitors/providers to reject
- Compliance notes

Each field should explain how it affects classification and automation.

### 6.3 Sales Voice

New first-class section.

Inputs:

- paste comment examples
- paste inbox examples
- upload sales scripts
- connect Sheet with scripts
- mark examples as "good", "bad", "too aggressive", "too generic"

Outputs:

- generated sales voice summary
- comment rules
- inbox rules
- CTA rules
- forbidden phrases
- sample generated messages preview

The preview must be generated from real stored context, not fake UI.

### 6.4 Knowledge Sources

Enterprise source table:

- source name
- type: file, sheet, drive, manual memory, URL
- status: ready, needs_auth, failed, syncing
- trust level
- last synced
- rows/items read
- last error
- used by agent: yes/no
- owner

Actions:

- sync
- disconnect
- view summary
- mark trusted/untrusted
- delete

Never fake Google Drive sync. Drive remains `needs_auth` until read-only OAuth
is implemented.

### 6.5 Pricing & Offers

Pricing is important for sales automation. Pull it out of generic files:

- connected pricing sheets
- product/service packages
- quote rules
- shipping/lead-time notes
- discount rules
- when to ask human

Agent should use this when replying to leads.

### 6.6 Customers & Segments

Define target segments without hardcoding industries:

- segment name
- target role
- intent signals
- reject signals
- source hints
- message angle
- confidence threshold
- preferred action: comment/inbox/post/observe

Example:

```text
Segment: POD / dropship sellers needing fulfillment
Target role: customer
Positive signals: looking for supplier, fulfillment, ship to US, sourcing China/VN
Negative signals: provider ads, recruitment, spam links, agency self-promotion
Preferred action: comment first, inbox after reply or high confidence
```

### 6.7 Rules & Guardrails

Make automation policy explicit:

- observe only
- suggest only
- auto-comment high-confidence leads
- auto-inbox high-confidence leads
- max actions per hour/day
- cooldown per lead/profile
- stop when lead replies
- human-required conditions
- Telegram notification rules

This is where `org:{id}:outbound_mode` and future campaign policy should be
managed by admins, not by prompt.

### 6.8 Audit & Learning

Show how the AI learns:

- recent skill executions
- recent crawl decisions
- accepted/rejected leads with evidence
- messages sent
- replies received
- examples learned from sales
- corrections by human

This is how the product earns trust.

## 7. UX Principles For Enterprise Feel

Do:

- use calm density, tables, tabs, clear panels
- show real state and timestamps
- show "why this matters" through tooltips or secondary text
- expose audit logs and data lineage
- make configuration feel like an operating system
- use consistent component primitives

Do not:

- create marketing hero sections inside dashboard
- use fake cards or placeholder metrics
- make Data Private look like a file uploader page
- bury sales voice in a generic textarea
- rely on emojis as primary status
- present "drafts" as the main automation outcome

## 8. Implementation Phases

### Phase A — Plan and Contract

- Add this spec to `AGENTS.md` start list.
- Update `PRODUCTION_FLOW.md` to state: drafts are internal fallback, not the
  primary automation UX.
- Define server contracts for Sales Voice Profile and action execution.

### Phase B — Data Model

Add org-scoped tables or context keys:

- `sales_voice_examples`
- `sales_voice_profile`
- `agent_action_decisions`
- `lead_action_history`

Minimum first slice can use `user_context` summaries plus a table for raw
examples:

```sql
sales_voice_examples (
  id,
  org_id,
  type,          -- comment | inbox | post | objection
  label,         -- good | bad | too_aggressive | too_generic
  content,
  source,
  created_by,
  created_at
)
```

### Phase C — Agent Brain / Generator

- Extend Agent Brain allowed tools with:
  - `scan_fanpage_inbox`
  - `care_fanpage`
  - `post_to_profile`
- Add action planning schema for leads.
- Add Sales Voice prompt block from Data Private summaries.
- If user provides a template in prompt, use it as a style/template input.
- If no template, generate from business profile + sales voice + lead context.

### Phase D — Runtime Action Execution

- THG Local Runtime polls approved outbox.
- Execute comment/inbox/group post for its assigned account.
- Mark sent/failed.
- Stream action to Browser dashboard.
- Notify Telegram on sent/failed.

### Phase E — Enterprise Data Private UI

Refactor `DataPrivateView.tsx` into components:

```text
data-private/
  DataPrivateShell.tsx
  DataReadinessHeader.tsx
  BusinessProfileSection.tsx
  SalesVoiceSection.tsx
  KnowledgeSourcesSection.tsx
  PricingOffersSection.tsx
  CustomerSegmentsSection.tsx
  RulesGuardrailsSection.tsx
  AuditLearningSection.tsx
```

Keep all state API-backed. No fake counts.

### Phase F — Evaluation

Add tests/fixtures:

- sales voice examples produce distinct comment tone
- no template still generates useful contextual message
- provider ads are rejected for customer-targeting orgs
- high-confidence auto mode creates approved outbound
- non-auto org does not execute
- Runtime marks sent/failed correctly
- Dashboard and Telegram prompts create identical action plans

## 9. Acceptance Criteria

The system is ready for this phase when:

- A user can upload/paste sales examples in Data Private.
- The agent can explain the learned sales voice.
- A prompt like "comment toàn bộ hot leads theo văn phong sales team" creates
  action decisions for real leads.
- If org auto mode is on and guardrails pass, Runtime executes those comments
  on Facebook.
- If org auto mode is off, the UI frames pending items as "requires policy
  approval", not as the main product value.
- Telegram receives action logs after comment/inbox/post execution.
- Dashboard Browser shows the account/session where automation happened.
- Every action has audit evidence: lead, message, reason, guardrails, account,
  source, timestamp.

## 10. Non-Goals

- Do not hardcode POD, HR, logistics, or any vertical into the core crawler.
- Do not fake Drive sync or fake media counts.
- Do not let AI flip outbound mode via prompt.
- Do not execute actions without org/account/session ownership checks.
- Do not store or transmit Facebook passwords.
- Do not optimize for volume over precision. Precision is the product moat.

