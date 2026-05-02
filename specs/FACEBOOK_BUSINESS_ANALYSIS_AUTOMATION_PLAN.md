# Facebook Business Analysis Automation Plan

Last updated: 2026-04-30

This document is the product and implementation plan for evolving THG AutoFlow
from "Facebook automation" into a business-specific market intelligence and
sales automation workspace.

It is written for Claude Code, Codex, and future engineering agents. If another
agent needs to continue the work, start here, then read:

- `openspec/root-architecture.md`
- `specs/ROOT_ARCHITECTURE.md`
- `frontend/src/modules/autoflow/`
- `internal/handlers/facebook_crawl/handler.go`
- `internal/ai/business.go`
- `internal/ai/universal.go`
- `internal/jobs/model.go`
- `internal/store/app_store.go`

## 1. Product Direction

The product should not be positioned as a generic scraper or spam automation
tool. The durable business direction is:

> AI Facebook Sales Intelligence Workspace for each business.

The system learns a client's organization, brand, offer, customer profile,
market, uploaded data, pricing, assets, and operating rules. It then uses the
client's own Facebook accounts, visible browser workspaces, and prompt-scoped
automation to find potential customers, classify intent, recommend strategy,
draft outreach, and learn from outcomes.

Automation is the execution layer. The paid product value is the analysis layer:

- who the business should target
- where those customers gather
- which pain points are active right now
- which groups/pages/posts are high quality
- what offer and message angle should be used
- which leads should be contacted first
- which campaign should run this week

### 1.1 Open Workspace Skill Designer

The platform should support user-designed business workflows inside each
workspace. This is not a visual design tool like Figma. It is a Claude-style
conversational designer:

```text
User describes a business workflow
  -> AI turns it into a skill blueprint
  -> system proposes data schema, sources, classifier, UI view, actions
  -> admin reviews and approves
  -> runtime executes it with browser/workspace guardrails
  -> outcomes feed the learning loop
```

The user should be able to say:

```text
"I want an HR module that finds candidates from Facebook recruitment posts,
filters comments, scores resumes, and suggests which candidates match each role."
```

The system should generate a safe blueprint, not arbitrary production code.
Blueprints must compile down to approved primitives:

- data entities
- source types
- classifier fields
- scoring rules
- prompt templates
- dashboard view schema
- outbound action policy
- browser automation tasks
- approval requirements

This turns the product into an open business automation platform. Facebook is
the primary data/action surface, but the business workflow can be sales,
recruitment, sourcing, customer support, competitor research, community
monitoring, or any Facebook-related operating process.

### 1.2 Domain Playbooks

Verticals should be modeled as playbooks, not hardcoded scrapers.

Examples:

- Sales lead intelligence
- POD/dropship sourcing intelligence
- HR/recruitment intelligence
- Real estate buyer/renter discovery
- Education/course enrollment discovery
- Beauty/spa local demand monitoring
- Logistics/export/import demand monitoring
- Agency client prospecting

Each playbook defines:

- target entities
- source discovery rules
- buying or intent signals
- negative signals
- scoring rubric
- recommended dashboard views
- safe outbound actions
- outcome labels

The same platform primitives should power all playbooks.

## 2. Business Workflow

The core workflow is:

```text
Org onboarding
  -> business profile
  -> customer segment map
  -> Facebook source discovery
  -> prompt-scoped crawl jobs
  -> in-crawl filtering and classification
  -> market signals and lead scoring
  -> strategy recommendations
  -> campaign drafts
  -> human approval
  -> browser-executed comment/inbox/posting
  -> outcome tracking
  -> learning loop updates org strategy
```

### 2.1 Org Onboarding

Every organization must provide a business context before meaningful automation:

- Company/brand name
- Industry/vertical
- What they sell or provide
- Ideal customers
- Negative customers or reject rules
- Locations served
- Languages and tone of voice
- Website, fanpage, catalog links
- Pricing or packages
- Differentiators and proof points
- Competitors or alternatives
- Uploaded private files: product list, service deck, FAQ, sales scripts,
  customer examples, policy documents
- Connected private data sources: Google Sheets for pricing/product/policy
  tables and Google Drive folders for real brand media/assets
- Approval policy: what the AI may draft, what requires human approval, what is
  forbidden

The product UX should make this feel like calibration, not a technical form. A
workspace admin should be guided to define:

- who the organization is
- what products/services it offers
- which author role it wants to find: customers, suppliers, partners,
  candidates, or providers
- which buying/request signals indicate useful data
- which promotion/spam/provider signals should be rejected
- target markets and language/tone
- approval and automation boundaries

This preserves the open-source/open-domain architecture while giving the
classifier a precise business lens. The platform should not assume that a
matching keyword is a lead. For one organization, a provider post is a
competitor ad; for another, the same provider post may be the supplier segment
they explicitly asked to find.

Existing hook: `internal/ai/business.go` already has `BusinessProfile`.
Do not hardcode THG, logistics, recruitment, POD, or any single vertical into
the crawler. Store the business profile per org and make all classifiers consume
that context.

### 2.1.1 Data Private and Data Connectors

Many client organizations keep business assets outside the app:

- Google Drive folders for real product images, ad creatives, brand videos,
  proof images, and sales collateral
- Google Sheets for detailed pricing, packages, product catalogs, stock notes,
  SOPs, FAQs, and discount policies
- uploaded PDF/DOCX/XLSX/TXT/CSV files for decks, policies, resumes, scripts,
  and company knowledge

The product surface for this is `Data Private`, implemented as a knowledge hub,
not only a file uploader.

Current implementation:

- `private_files` stores org-scoped uploaded files.
- `data_sources` stores org-scoped external connectors.
- `GET/POST/DELETE /api/data-sources` manage connector records.
- `POST /api/data-sources/:id/sync` syncs source metadata/content into
  summarized AI context.
- Google Sheets quick sync supports published/exportable CSV URLs and standard
  `docs.google.com/spreadsheets/d/...` URLs that can be exported as CSV.
- Google Drive sources are stored as `needs_auth` until read-only Drive OAuth is
  implemented; do not fake Drive media sync.
- AI prompt context includes:
  - `org:{id}:business_profile`
  - `org:{id}:private_files_summary`
  - `org:{id}:data_sources_summary`

Connector rules:

- Every source must be org-scoped.
- Sync must be explicit and auditable.
- Do not scan an entire Drive by default; user must choose a folder/file.
- Use read-only scopes for OAuth connectors.
- Store OAuth tokens encrypted.
- Summarize or retrieve relevant context instead of injecting large raw files
  into prompts.
- Use real uploaded/connected media only. Do not generate AI images.

Frontend ownership:

- `frontend/src/modules/autoflow/components/views/DataPrivateView.tsx` is the
  orchestrator only.
- Data Private subcomponents live under
  `frontend/src/modules/autoflow/components/data/`.
- Service/hook ownership:
  - `services/dataSourceService.ts`
  - `hooks/useDataSources.ts`

### 2.2 Facebook Account Workspace

Each Facebook account belongs to one org. One org can have many Facebook
accounts. A Facebook account runs inside a persistent browser profile.

Required behavior:

- User logs into Facebook from the dashboard Browser view.
- User can observe all automation from the live browser screen.
- Automation must attach to the selected account workspace.
- The system must never mix cookies, profile data, leads, or threads across orgs.
- The system must verify that the browser session still belongs to the expected
  Facebook user identity.

Existing hooks:

- `internal/workspace/`
- `internal/session/`
- `internal/livesession/`
- `internal/server/workspace_handlers.go`
- `internal/server/screen_proxy.go`
- `frontend/src/modules/autoflow/components/views/BrowserView.tsx`

### 2.3 Recurring Crawl Intelligence

The first successful crawl prompt is both an action and a learning event. When an
organization asks for a market segment, source, group, search query, or campaign
context, the backend should persist that need as an org-scoped crawl intent.

Current implementation direction:

- Store learned needs in `org_crawl_intents`.
- Default interval is 30 minutes and never lower than 30 minutes without an
  explicit product decision.
- The scheduled loop reuses the stored source, keywords, selected account, and
  max item cap. It does not call the AI again just to decide what to crawl.
- AI is used for tasks that need judgment: interpreting the first prompt,
  extracting segment keywords, classifying ambiguous leads, drafting outreach,
  and learning strategy.
- Traditional automation handles cheap repeated work: scheduler ticks, queue
  idempotency, Local Runtime commands, Playwright/Chrome crawling, dedup,
  cooldowns, and retries.
- If no logged-in local Facebook runtime is ready, the intent records the error
  and waits for the next interval instead of spinning or creating fake data.
- Admins can inspect and disable crawl intents via API; dashboard controls can
  be layered on top.

This is the cost model for scale: prompts teach the system once, then the
service runs deterministic automation 24/7 and spends AI only where analysis or
language generation creates real value.

### 2.3.1 Market Signal Gate

Open crawling must distinguish the role of the author in each Facebook post.
Broad industries such as logistics, ecommerce, sourcing, HR, or real estate
will always contain two opposite populations in the same groups:

- people asking for a service, supplier, quote, recommendation, hiring help, or
  buying/sourcing support
- people advertising that they provide that same service

The crawler must not treat both as leads just because keywords match. The first
gate is deterministic and low-cost:

- `buyer_demand`: explicit request/problem/question/buying intent
- `provider_promotion`: author is selling/advertising their own service
- `spam_or_low_trust`: mass promotion, low-trust links, unrelated offers
- `keyword_only`: content mentions the topic but has no customer intent

Only `buyer_demand` or strong request/question signals should become hot/warm
leads by default. Provider promotions and keyword-only content should be
rejected or kept cold unless the organization explicitly defines suppliers,
partners, resellers, or candidates as its target segment.

This gate protects trust: the system should prefer fewer accurate leads over a
large list polluted by competitors and ads. AI classifiers can still resolve
ambiguous posts, but the cheap deterministic gate must run first for recurring
jobs and Local Runtime results.

### 2.3.2 Business Calibration UX

Market Signal Gate is personalized by organization context, not by hardcoded
vertical rules. Before a workspace runs meaningful crawl automation, admins
should define the business in a Claude-style calibration flow:

- who the organization is and what it sells
- which author role is the target: customers, suppliers, partners, candidates,
  or providers
- ideal customer/segment description
- positive signals that should be kept
- negative signals and reject rules that should be filtered out
- markets, location, tone, USP, and approval policy

Dashboard Chat and Telegram share the same preflight. If a Facebook crawl prompt
arrives before this context exists, the system must not create a crawler job.
It should ask for the business calibration first, then reuse the saved
org-scoped context for future prompts, recurring crawl intents, Local Runtime
results, AI classification, comments, inbox, and posting.

This lets the same Facebook post be treated differently per workspace. A post
from a supplier is rejected for an organization seeking end customers, but can
become a valid warm lead for an organization explicitly sourcing suppliers or
partners.

### 2.4 Source Discovery

The system needs source discovery, not only manual group URLs.

Source types:

- Facebook groups
- Facebook group posts
- Comments under high-intent posts
- Facebook profiles visible from posts/comments
- Page posts and page comments
- Messenger/inbox threads
- Marketplace or public selling surfaces when available
- External web pages only when the user asks for non-Facebook research

Source discovery should create a source catalog:

```text
source_catalog
  org_id
  platform
  source_type
  url
  title
  segment_id
  relevance_score
  quality_score
  spam_score
  last_checked_at
  decision: use | monitor | reject
  reason
```

For Facebook, source discovery can start from:

- AI-generated group search queries from the business profile
- User-provided groups/pages/profiles
- Past high-yield sources
- Groups mentioned inside posts/comments
- Competitor/customer community names provided by the org

Existing hook: `UniversalGroupQueries` in `internal/ai/universal.go`.

## 3. Customer Intelligence Model

The system should model the customer's market before running outreach.

### 3.1 Customer Segments

Add first-class customer segments per org.

Example for POD/dropship/sourcing:

```text
Segment: POD sellers scaling to US
Needs:
  - reliable fulfillment
  - product sourcing from China or Vietnam
  - lower MOQ
  - shipping to US/EU
  - branded packaging
Signals:
  - "looking for supplier"
  - "fulfillment delay"
  - "ship to US"
  - "agent in China"
  - "Vietnam supplier"
Negative signals:
  - job seeker
  - generic course seller
  - agency selling the same service
Offer angles:
  - Vietnam/China sourcing plus fulfillment
  - faster shipping windows
  - sample support
  - packaging/labeling
```

Recommended table:

```sql
customer_segments (
  id INTEGER PRIMARY KEY,
  org_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  ideal_customer TEXT NOT NULL DEFAULT '',
  pain_points_json TEXT NOT NULL DEFAULT '[]',
  buying_signals_json TEXT NOT NULL DEFAULT '[]',
  negative_signals_json TEXT NOT NULL DEFAULT '[]',
  offer_angles_json TEXT NOT NULL DEFAULT '[]',
  priority INTEGER NOT NULL DEFAULT 50,
  active INTEGER NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
)
```

### 3.2 Market Signals

Do not only save leads. Save reusable market signals.

Signals answer:

- What pain point is appearing?
- Which segment does it match?
- How strong is the buying intent?
- Which source produced it?
- Is it a lead, trend, objection, competitor mention, or content idea?

Recommended table:

```sql
market_signals (
  id INTEGER PRIMARY KEY,
  org_id INTEGER NOT NULL,
  task_id TEXT NOT NULL DEFAULT '',
  segment_id INTEGER NOT NULL DEFAULT 0,
  source_url TEXT NOT NULL DEFAULT '',
  author_profile_url TEXT NOT NULL DEFAULT '',
  author_name TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL DEFAULT '',
  signal_type TEXT NOT NULL DEFAULT 'lead',
  intent TEXT NOT NULL DEFAULT '',
  pain_point TEXT NOT NULL DEFAULT '',
  stage TEXT NOT NULL DEFAULT '',
  score REAL NOT NULL DEFAULT 0,
  priority TEXT NOT NULL DEFAULT 'cold',
  recommended_angle TEXT NOT NULL DEFAULT '',
  ai_reason TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
)
```

Signal types:

- lead
- pain_point
- objection
- competitor_mention
- source_quality
- content_angle
- market_trend
- partnership
- not_relevant

### 3.3 Strategy Recommendations

The product becomes valuable when it says what to do next.

Recommended table:

```sql
strategy_recommendations (
  id INTEGER PRIMARY KEY,
  org_id INTEGER NOT NULL,
  segment_id INTEGER NOT NULL DEFAULT 0,
  title TEXT NOT NULL,
  summary TEXT NOT NULL DEFAULT '',
  evidence_json TEXT NOT NULL DEFAULT '[]',
  recommended_actions_json TEXT NOT NULL DEFAULT '[]',
  campaign_brief_json TEXT NOT NULL DEFAULT '{}',
  confidence REAL NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'draft',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
)
```

Example recommendation:

```text
Title: Target POD sellers looking for US fulfillment alternatives
Evidence:
  - 37 posts mention fulfillment delay
  - 14 posts ask for supplier/agent
  - 8 high-intent comments mention shipping to US
Actions:
  - monitor 5 groups daily
  - contact 23 hot leads manually approved by admin
  - post educational case study about Vietnam sourcing + US shipping
  - draft 3 comment angles and 2 inbox angles
```

## 4. Automation Surface Map

Facebook automation must be split into safe, observable workflows.

### 4.1 Browser Session

Purpose:

- login
- observe automation
- manual takeover
- recover checkpoint
- inspect state

Rules:

- No automation without an active logged-in workspace.
- If checkpoint or login wall appears, job returns `human_required`.
- Browser stream starts only while a viewer is connected.

### 4.2 Group Discovery

Purpose:

- search for groups relevant to customer segments
- score group quality
- propose groups for approval

Output:

- source catalog entries
- group quality scores
- recommended monitor list

Human approval:

- required before joining groups at scale
- required before posting in a new group

### 4.3 Group/Post Monitoring

Purpose:

- read posts within approved groups or user-provided sources
- extract candidate items
- classify during crawling

Rules:

- No broad "scan all" by default.
- Every job needs target URL, query, or campaign context.
- Apply filters and classifier before storing lead-like records.
- Save raw enough context for audit, but avoid unnecessary data hoarding.

Existing hook: `internal/handlers/facebook_crawl/handler.go`.

### 4.4 Comment Mining

Purpose:

- identify buyers inside comment threads, not only post authors
- detect "me too", "inbox price", "need supplier", "ship?", "check ib" style
  intent

Implementation:

- add a crawl source type `facebook_post_comments`
- fetch comments in batches
- classify each comment against org business profile and segments
- create market signals and leads

### 4.5 Profile Enrichment

Purpose:

- inspect public profile/page context before outreach
- improve score with role, location, business type, seller signals

Rules:

- Only collect data visible to the logged-in account.
- Do not attempt to bypass access controls.
- Cache minimal summary, not everything.

### 4.6 Outbound Commenting

Purpose:

- draft relevant comments for approved leads
- optionally execute after approval

Rules:

- Default status is draft.
- Admin approval is the default path.
- Explicit execute/auto mode may queue comments as `approved` immediately when
  the org/campaign/prompt permits speed-first automation.
- Each account has rate limits and cooldowns.
- Message must reference the specific post context.
- Never mass-comment the same template.
- Backend must block duplicate comments for the same org + post URL even if AI
  calls the tool repeatedly.

Existing hooks:

- `internal/ai/universal.go` -> `UniversalComment`
- `outbound_messages`
- `frontend/src/modules/autoflow/components/views/CommentingView.tsx`

### 4.7 Inbox / Messenger

Purpose:

- continue conversations with leads
- summarize thread history
- suggest replies

Rules:

- First-message automation is draft/approval-gated by default.
- Explicit execute/auto mode may queue first messages as `approved`, but only
  after org-scoped guardrails confirm no active duplicate/cooldown conflict.
- Follow-up must use thread history.
- Store conversation state by org and profile URL.
- If `last_inbound_at > last_outbound_at`, treat the thread like customer
  service and answer the latest reply; otherwise do not send repeated inboxes
  inside the cooldown window.

Existing hooks:

- `conversation_threads`
- `conversation_messages`
- `UniversalInbox`
- `GenerateFollowUp`

### 4.8 Posting / Campaign Execution

Purpose:

- create content drafts based on strategy recommendations
- post to owned pages/groups or allowed communities

Rules:

- Posting is higher risk than passive reading.
- Require explicit campaign approval or explicit execute/auto policy.
- Respect group rules and org approval policy.
- Support scheduling, not immediate blast by default.
- Backend should queue `group_post` rows per target group and still apply
  cooldown/dedup before `approved` execution.

### 4.9 Prompt Compatibility and Auto Execution

Dashboard chat and Telegram prompt must route through the same org-scoped AI
operator path. A prompt such as "find customer segment A/B/C on Facebook" should
create a prompt-scoped source discovery/crawl job, not a broad scan-all job. A
follow-up prompt such as "comment these leads" or "inbox hot leads" should act
on the org's real leads from the latest crawler/app lead stores.

Implementation notes:

- Use `ProcessPromptForOrg` for Dashboard and Telegram so business data,
  account mapping, and tool calls remain tenant-scoped.
- Tool calls for `search_groups`, `comment_all_leads`, `inbox_all_leads`, and
  `create_job_post` must write real scheduler jobs or `outbound_messages`.
- `auto=true` or org `outbound_mode=auto` may set outbox status to `approved`.
- `CanQueueOutboundForOrg` remains the final gate for dedup, cooldown, closed
  threads, and reply-aware customer-service behavior.
- Connected data from Data Private/Sheets/Drive summaries is context for AI
  decisions; it does not bypass outbound safety rules.

## 5. AI Pipeline

### 5.1 Parser

Input:

- dashboard chat prompt
- Telegram prompt
- scheduled campaign
- source discovery request

Output:

- versioned `jobs.Task`
- intent
- sources
- business context version
- analysis goal
- filters
- segment targets

Existing hook: `internal/jobs/model.go`.

Needed additions:

```go
BusinessContextVersion string   `json:"business_context_version,omitempty"`
SegmentIDs             []int64  `json:"segment_ids,omitempty"`
AnalysisGoal           string   `json:"analysis_goal,omitempty"`
ActionPolicy           string   `json:"action_policy,omitempty"` // observe_only | draft_only | approval_required | execute_allowed
```

### 5.2 Classifier

Current `UniversalClassify` is a good base, but it should return richer output:

```json
{
  "score": 0.0,
  "priority": "hot|warm|cold|rejected",
  "intent": "potential_customer|partner|competitor|spam|not_relevant",
  "segment": "POD sellers scaling to US",
  "pain_point": "fulfillment delay",
  "stage": "problem_aware|solution_seeking|vendor_comparing|ready_to_buy",
  "recommended_angle": "Vietnam sourcing plus US shipping",
  "reason": "..."
}
```

Implementation location:

- add `UniversalSignalAnalyze` in `internal/ai/universal.go`
- keep `UniversalClassify` backward compatible
- update `facebook_crawl` handler to save signal fields

### 5.3 Strategy Synthesizer

After a job or daily batch, synthesize signals into recommendations.

Input:

- business profile
- customer segments
- recent market signals
- source quality
- lead outcomes

Output:

- opportunity map
- recommended groups/sources
- recommended campaign briefs
- suggested outreach angles
- risks and reject rules to update

Implementation:

- create `internal/ai/strategy.go`
- create `internal/analysis/` or `internal/intelligence/` package for the
  non-LLM aggregation logic
- keep LLM responsible for synthesis, not raw database access

### 5.4 Learning Loop

The system learns from outcomes:

- user converted lead
- user rejected lead
- lead replied
- lead ignored
- account got checkpoint
- source produced spam
- source produced high-quality leads

Existing hooks:

- `learning_profiles`
- `outcome_events`
- `learning_history`

Needed additions:

- outcome UI controls on Leads and Inbox
- source quality update from outcomes
- segment scoring update from outcomes
- strategy recommendation status: draft, approved, executed, archived

## 6. Frontend Product Views

The dashboard should evolve from automation tabs to intelligence workflow tabs.

Recommended navigation:

```text
Overview
Chat
Skill Designer

FACEBOOK
  Browser
  Sources
  Leads
  Inbox
  Posting
  Commenting

INTELLIGENCE
  Business Profile
  Customer Segments
  Market Signals
  Opportunity Map
  Recommended Campaigns
  Domain Playbooks

CONFIG
  Staff
  Security
  Files
  Billing
  Logs
```

### 6.1 Business Profile View

Admin can edit:

- company profile
- offers/services
- target customers
- reject rules
- tone of voice
- competitors
- approval policy
- uploaded files

### 6.2 Customer Segments View

Admin can:

- create/edit customer segments
- accept AI-suggested segments
- add buying/negative signals
- map sources to segments
- set priority

### 6.3 Market Signals View

Show:

- signal feed
- pain point clusters
- trends by source
- source quality
- segment match
- action recommendation

### 6.4 Opportunity Map View

Show:

- best segments
- best sources/groups
- high-value pain points
- weekly opportunity summary
- where to spend effort next

### 6.5 Recommended Campaigns View

Show:

- campaign briefs
- suggested posts/comments/inbox messages
- target sources and lead lists
- expected risk level
- approval and execution controls

### 6.6 Skill Designer View

The Skill Designer is where admins create business-specific automation modules
by describing the workflow they want.

It should feel like a product strategist plus engineer sitting inside the
workspace:

```text
User:
  "Build a recruitment module for our company.
   Crawl HR posts and candidate comments from Facebook groups.
   Let us upload JDs and resumes.
   Score candidate-role fit and suggest outreach messages."

System proposes:
  - data entities: job_position, candidate_signal, resume_profile, match_score
  - sources: recruitment groups, post comments, candidate profiles
  - classifiers: role intent, experience, skills, location, salary fit
  - UI views: Jobs, Candidates, Resume Analysis, Match Board
  - actions: draft comment, draft inbox, export shortlist
  - approval policy: first outreach is approval-required
```

The view should include:

- Prompt/chat panel for designing or editing a workflow.
- Generated blueprint preview.
- Data model preview.
- Classifier/scoring preview.
- Dashboard view preview using approved components.
- Test run/sandbox mode on sample data.
- Publish button requiring admin approval.
- Version history and rollback.

Frontend rule: generated views must be assembled from approved dashboard
components and schemas. Do not let the LLM generate arbitrary frontend code in
production.

## 7. Backend Implementation Phases

### Phase A: Business Context Foundation

Goal: make org business context first-class and org-scoped.

Tasks:

1. Add `business_profiles` or org-scoped replacement for global `user_context`.
2. Backfill from existing `user_context` for org 1.
3. Add store CRUD for business profile.
4. Add API endpoints:
   - `GET /api/business/profile`
   - `PUT /api/business/profile`
   - `POST /api/business/profile/extract`
5. Update frontend Settings/Business Profile UI.
6. Update `ai.LoadProfile` to load by `org_id`, not global context.
7. Update `facebook_crawl` handler to use job org context.

Acceptance:

- Two orgs can have different profiles.
- Classifier output changes based on org profile.
- No global `user_context` dependency in production path.

### Phase B: Customer Segments

Goal: allow every business to define target customer groups.

Tasks:

1. Add `customer_segments` table.
2. Add CRUD in store.
3. Add AI segment extraction from business profile and uploaded files.
4. Add frontend Customer Segments view.
5. Add segment IDs to `jobs.Task`.
6. Update classifier prompt to choose a segment.

Acceptance:

- Admin can create segments manually.
- AI can suggest segments from company profile.
- Crawl jobs can target one or more segments.

### Phase C: Market Signals

Goal: save reusable intelligence, not only leads.

Tasks:

1. Add `market_signals` table.
2. Add `UniversalSignalAnalyze`.
3. Update `facebook_crawl` handler:
   - classify per item
   - save hot/warm leads
   - save non-lead market signals when useful
   - reject spam/noise
4. Add `GET /api/market/signals`.
5. Add Market Signals frontend view.

Acceptance:

- A crawl can produce leads, pain points, objections, and competitor mentions.
- Signal records are org-scoped.
- Cold/rejected content is not dumped into lead tables.

### Phase D: Source Catalog and Discovery

Goal: find and rank places where ideal customers gather.

Tasks:

1. Add `source_catalog` table.
2. Implement AI group query generation per segment.
3. Implement source quality scoring:
   - relevance
   - professionalism
   - spam
   - recent activity
   - historical yield
4. Add source approval flow.
5. Add Sources frontend view.

Acceptance:

- User can provide a broad prompt like "find POD seller communities".
- System proposes sources, does not blindly scan everything.
- Admin approves sources before recurring monitoring.

### Phase E: Strategy Recommendations

Goal: turn data into business advice.

Tasks:

1. Add `strategy_recommendations` table.
2. Implement aggregation logic from recent signals.
3. Implement LLM strategy synthesis.
4. Add API:
   - `POST /api/strategy/generate`
   - `GET /api/strategy/recommendations`
   - `POST /api/strategy/recommendations/:id/approve`
5. Add Opportunity Map and Recommended Campaigns views.

Acceptance:

- System can explain why a segment/source/campaign is recommended.
- Evidence is traceable to signals and leads.
- Human can approve/reject recommendations.

### Phase F: Campaign and Outbound Guardrails

Goal: execute safely after analysis.

Tasks:

1. Add `campaigns` table.
2. Link campaigns to segments, sources, leads, and outbound messages.
3. Add approval states:
   - draft
   - approved
   - scheduled
   - running
   - paused
   - completed
4. Add account-level limits:
   - comments per hour/day
   - inbox per hour/day
   - posts per day
   - random delay bounds
   - cooldown on checkpoint signals
5. Add execution audit log.

Acceptance:

- No production outbound action runs without policy permission.
- User can see browser actions while automation is running.
- Every outbound action is attributable to org, account, user, campaign, and lead.

### Phase G: Learning and Outcome Feedback

Goal: improve targeting and strategy from real results.

Tasks:

1. Add outcome buttons on Leads/Inbox:
   - converted
   - replied
   - ignored
   - bad fit
   - spam
2. Write `outcome_events`.
3. Update source quality and segment quality from outcomes.
4. Update recommendation confidence.
5. Generate weekly business insight summary.

Acceptance:

- Rejecting leads reduces similar future scores.
- Converting leads boosts matching segment/source patterns.
- Weekly summary explains what changed and why.

### Phase H: Workspace Skill Designer

Goal: let each organization design new Facebook-related business workflows from
natural language, without writing custom code for every vertical.

Tasks:

1. Add `skill_blueprints` table:
   - `org_id`
   - `name`
   - `domain`
   - `description`
   - `schema_json`
   - `classifier_json`
   - `views_json`
   - `actions_json`
   - `approval_policy_json`
   - `version`
   - `status`
2. Add `skill_blueprint_versions` for rollback and audit.
3. Add an AI blueprint generator:
   - input: user workflow prompt plus org business profile
   - output: validated blueprint JSON
   - no arbitrary code output
4. Add a validator that checks:
   - every entity has `org_id`
   - every action maps to an approved primitive
   - every outbound action has an approval policy
   - every Facebook source obeys browser/session guardrails
5. Add Skill Designer API:
   - draft blueprint
   - validate blueprint
   - publish blueprint
   - list versions
   - rollback version
6. Add frontend Skill Designer view.
7. Add runtime interpreter:
   - turns blueprint into task parser hints
   - registers dashboard view schemas
   - maps classifier outputs into market signals or domain records
8. Add HR/recruitment as the first non-sales reference blueprint.

Acceptance:

- Admin can describe a new Facebook-related workflow in natural language.
- System generates a blueprint with data schema, classifier, views, and actions.
- Blueprint must pass validation before publishing.
- Published workflow can run without custom Go/TS code for the vertical.
- HR blueprint can analyze recruitment posts/comments, uploaded JDs/resumes,
  candidate strengths, gaps, and match scores.

## 8. API Surface

Recommended endpoints:

```text
Business context:
GET    /api/business/profile
PUT    /api/business/profile
POST   /api/business/profile/extract

Private data connectors:
GET    /api/files
POST   /api/files
DELETE /api/files/:id
GET    /api/data-sources
POST   /api/data-sources
POST   /api/data-sources/:id/sync
DELETE /api/data-sources/:id

Customer segments:
GET    /api/customer-segments
POST   /api/customer-segments
PUT    /api/customer-segments/:id
DELETE /api/customer-segments/:id
POST   /api/customer-segments/suggest

Sources:
GET    /api/sources
POST   /api/sources/discover
POST   /api/sources/:id/approve
POST   /api/sources/:id/reject

Market signals:
GET    /api/market/signals
GET    /api/market/pain-points
GET    /api/market/source-quality

Strategy:
POST   /api/strategy/generate
GET    /api/strategy/recommendations
POST   /api/strategy/recommendations/:id/approve
POST   /api/strategy/recommendations/:id/archive

Campaigns:
GET    /api/campaigns
POST   /api/campaigns
POST   /api/campaigns/:id/approve
POST   /api/campaigns/:id/pause
POST   /api/campaigns/:id/resume

Outcomes:
POST   /api/leads/:id/outcome
POST   /api/signals/:id/outcome

Skill Designer:
GET    /api/skill-blueprints
POST   /api/skill-blueprints/draft
POST   /api/skill-blueprints/validate
POST   /api/skill-blueprints/:id/publish
GET    /api/skill-blueprints/:id/versions
POST   /api/skill-blueprints/:id/rollback
```

All endpoints must be org-scoped and protected by `tenantReady`/auth middleware.

## 9. Code Ownership Map

Use this map when implementing.

```text
Product context:
  internal/ai/business.go
  internal/store/store.go or new internal/store/business.go

AI analysis:
  internal/ai/universal.go
  internal/ai/strategy.go (new)
  internal/analysis/ (new, deterministic aggregation)

Task model:
  internal/jobs/model.go
  internal/parser/
  internal/handlers/facebook_crawl/handler.go

Persistence:
  internal/store/app_store.go
  internal/store/*.go
  db/schema.sql as reference only

API:
  internal/server/api.go
  internal/server/*_handlers.go

Frontend:
  frontend/src/modules/autoflow/services/
  frontend/src/modules/autoflow/hooks/
  frontend/src/modules/autoflow/components/views/
  frontend/src/modules/autoflow/types/index.ts

Workspace Skill Designer:
  internal/ai/blueprint.go (new)
  internal/blueprints/ (new validator/interpreter)
  internal/store/blueprints.go (new)
  internal/server/blueprint_handlers.go (new)
  frontend/src/modules/autoflow/components/views/SkillDesignerView.tsx (new)
```

## 10. Guardrails

Must-have rules:

- Do not reintroduce legacy `internal/server/static/index.html`, `app.js`, or
  `style.css` as the production UI.
- Do not reintroduce hardcoded scan-all behavior.
- Do not use global business context for multi-tenant classification.
- Every new data table must include `org_id` unless it is truly platform-global.
- Every API handler must check org ownership.
- Do not generate AI images. Only use user-uploaded real images/files.
- Do not bypass Facebook access controls.
- Do not hide automation from the user; Browser view must remain observable.
- Default outbound automation to draft/approval-required.
- If Facebook checkpoint/login wall appears, return `human_required`.
- Prefer analysis and recommendations before execution.
- User-designed skills must compile to validated blueprints and approved
  primitives. Do not execute arbitrary LLM-generated code in production.
- Generated dashboard views must use approved components and schema-driven
  rendering, not raw arbitrary TSX/HTML from the model.

## 11. Example: POD / Dropship Business

Business:

```text
We help POD and dropship sellers source products from China/Vietnam,
customize packaging, fulfill orders, and ship to the US/EU.
```

Segments:

- POD sellers scaling to US
- Dropshippers looking for suppliers
- Shopify/Etsy sellers needing product sourcing
- Sellers frustrated with fulfillment delays
- Agencies managing e-commerce brands

Sources:

- POD groups
- Dropshipping groups
- Shopify seller groups
- Etsy seller groups
- China sourcing groups
- Vietnam manufacturing/export groups
- US fulfillment discussion groups

Signals:

- "looking for supplier"
- "ship to US"
- "agent in China"
- "Vietnam supplier"
- "fulfillment delay"
- "MOQ too high"
- "custom packaging"
- "private label"
- "winning product"

Strategy output:

```text
Opportunity:
POD sellers are discussing fulfillment delay and supplier instability.

Recommended campaign:
Educational post + soft comment outreach about Vietnam/China sourcing
with lower MOQ and US shipping support.

Priority leads:
People asking for supplier, fulfillment, shipping, packaging, or sample support.

Rejected:
Course sellers, job seekers, generic spam, people selling the same service.
```

## 12. Example: HR / Recruitment Intelligence

This is the reference non-sales vertical. It proves the platform can go deeper
than lead finding while still staying Facebook-related.

Business:

```text
We are a small or medium business hiring staff from Facebook recruitment groups.
We want to find candidates, analyze resumes, understand strengths/gaps, and
shortlist the best matches for each job.
```

Data the org provides:

- Company profile
- Job descriptions
- Role requirements
- Salary range and location
- Hiring criteria
- Resume files or candidate CV text
- Interview notes
- Reject rules

Facebook sources:

- Recruitment groups
- HR posts
- Candidate comments under job posts
- Candidate profile pages visible to the logged-in account
- Messenger conversations with applicants

Signals:

- "looking for job"
- "send me JD"
- "I have 2 years experience"
- "remote only"
- "HCM/Hanoi"
- role keywords such as sales, accounting, warehouse, designer, developer
- salary expectations
- availability date

Candidate analysis output:

```text
Candidate:
  name
  profile_url
  target_role
  experience_summary
  strengths
  gaps
  location_fit
  salary_fit
  communication_quality
  resume_score
  facebook_signal_score
  overall_match_score
  recommended_next_step
```

HR workflows:

- Crawl candidate comments from recruitment posts.
- Match candidates to open roles.
- Parse uploaded resumes and extract structured experience.
- Compare resume content with Facebook comments/profile signals.
- Generate candidate shortlist.
- Draft personalized inbox messages.
- Generate interview question suggestions.
- Track outcomes: contacted, replied, interviewed, hired, rejected.

Guardrails:

- HR workflows must be explainable and auditable.
- Scores are decision support, not final hiring decisions.
- Avoid protected-class or sensitive personal trait inference.
- Keep candidate data org-scoped and access-controlled.
- First outreach must be approval-required.

## 13. Implementation Priority

Recommended next implementation order:

1. Org-scoped business profile API and UI.
2. Data Private connectors: Google Sheets quick sync, Drive OAuth, media library.
3. Customer segments table/API/UI.
4. Rich signal classifier and market signals table.
5. Source catalog and source discovery.
6. Market Signals and Opportunity Map views.
7. Strategy recommendations.
8. Campaign approval and outbound guardrails.
9. Learning loop from outcomes.
10. Workspace Skill Designer and blueprint validation.
11. HR/recruitment reference blueprint.
12. Production database upgrade to Postgres when traffic grows.

This order keeps the product aligned with business analysis first, and prevents
the engineering path from drifting back into generic automation or fixed
scraper behavior.
