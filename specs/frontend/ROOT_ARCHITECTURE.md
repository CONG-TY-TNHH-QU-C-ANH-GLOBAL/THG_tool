# ROOT_ARCHITECTURE.md
# AutoFlow Platform — Frontend Architecture Spec
# Source-of-truth derived from: autoflow_platform.experimental.tsx (692 lines, audited 2026-04-27)

## Product Overview

AutoFlow is a multi-tenant Facebook sales automation SaaS. Staff and admin users manage AI agents that scrape Facebook groups for leads, classify them, and send automated messages.

## User Roles

| Role | Access |
|---|---|
| `super_admin` | Platform-wide: all orgs, billing, system health |
| `admin` | Org-scoped: all tabs including Browser, Posting, Commenting, Settings |
| `staff` | Org-scoped: Leads (own), Inbox, Leaderboard, Data Private only |

## View State Machine

```
landing
  ├── onLogin → auth(mode=login)
  ├── onRegister → auth(mode=register)
  └── onAdmin → superadmin

auth
  ├── login → success(role=admin|staff) → app
  ├── register → step1 → step2 → success → app
  └── forgot → sent

app (MainApp)
  ├── tabs (admin): leads | browser | inbox | posting | commenting | leaderboard | data | settings
  └── tabs (staff): leads | inbox | leaderboard | data

superadmin
  └── tabs: orgs | users | system
```

## Component Tree (target — post-refactor)

```
Root
├── Landing
│   ├── NavBar
│   ├── HeroSection
│   ├── StatsBar
│   ├── FeaturesGrid
│   ├── PricingGrid
│   └── Footer
├── Auth
│   ├── LoginForm
│   ├── RegisterForm (2-step)
│   └── ForgotPasswordForm
├── SuperAdmin
│   ├── StatsGrid
│   ├── OrgTable
│   ├── UserList
│   └── SystemHealth
└── MainApp
    ├── Sidebar
    │   ├── BrandLogo
    │   ├── OrgSwitcher (admin only)
    │   ├── NavItems
    │   └── UserFooter
    ├── TopBar
    └── TabContent (lazy-loaded views)
        ├── LeadsView
        ├── BrowserView
        ├── InboxView
        ├── PostingView
        ├── CommentingView
        ├── LeaderboardView
        ├── DataPrivateView
        └── SettingsPage
            ├── BrandingTab
            ├── SecurityTab
            ├── StaffTab
            ├── AgentsTab
            └── BillingTab
```

## State Architecture

### Global State (Context or Zustand store)
```typescript
interface AuthState {
  token: string | null
  user: { id: string; name: string; email: string } | null
  role: 'admin' | 'staff' | 'super_admin'
  orgId: string | null
}

interface OrgState {
  activeOrg: Organization | null
  orgs: Organization[]
  switchOrg: (orgId: string) => void
}
```

### Server State (TanStack Query)
All queries keyed by `orgId`. Invalidated on `OrgState.switchOrg()`.

```typescript
// Keys
['orgs', orgId, 'leads', { status }]
['orgs', orgId, 'threads']
['orgs', orgId, 'threads', threadId, 'messages']
['orgs', orgId, 'posts']
['orgs', orgId, 'staff']
['orgs', orgId, 'files']
['orgs', orgId, 'kpi-config']
['orgs', orgId, 'facebook', 'status']
```

### Local State (stays in components)
- Tab selection
- Filter values (lead status filter)
- Form input state
- Modal open/close
- Drag state

## Module Structure (target)

```
src/modules/autoflow/
├── index.ts                          ← public API
├── autoflow_platform.experimental.tsx ← FREEZE after extraction complete
├── components/
│   ├── ui/
│   │   ├── Avatar.tsx               (was Av)
│   │   ├── Badge.tsx                (was Bdg)
│   │   ├── Label.tsx                (was Lbl)
│   │   ├── Input.tsx                (was Inp)
│   │   └── Row.tsx
│   ├── Landing.tsx
│   ├── Auth.tsx
│   ├── SuperAdmin.tsx
│   ├── Sidebar.tsx
│   ├── TopBar.tsx
│   ├── SettingsPage.tsx
│   └── views/
│       ├── LeadsView.tsx
│       ├── BrowserView.tsx
│       ├── InboxView.tsx
│       ├── PostingView.tsx
│       ├── CommentingView.tsx
│       ├── LeaderboardView.tsx
│       └── DataPrivateView.tsx
├── hooks/
│   ├── useAuth.ts
│   ├── useOrg.ts
│   ├── useLeads.ts
│   ├── useThreads.ts
│   ├── usePosts.ts
│   ├── useStaff.ts
│   ├── useFiles.ts
│   ├── useKpiConfig.ts
│   ├── useFacebookSession.ts
│   └── useLeaderboard.ts
├── services/
│   ├── api.ts                       ← base axios/fetch client with auth header
│   ├── authService.ts
│   ├── leadsService.ts
│   ├── threadsService.ts
│   ├── postsService.ts
│   ├── staffService.ts
│   ├── fileService.ts
│   ├── kpiService.ts
│   └── facebookService.ts
├── types/
│   └── index.ts                     ← all TypeScript interfaces
├── constants/
│   └── styles.ts                    ← design tokens (replaces D, PB, SB, sc)
└── autoflow.css                     ← @keyframes, global styles
```

## Design Token System

## Data Private Component Update

`DataPrivateView.tsx` should stay an orchestrator. Feature UI for the private
knowledge hub is split into focused components under
`components/data/`:

- `DataStatsGrid.tsx`
- `BusinessMemoryPanel.tsx`
- `DataSourcesPanel.tsx`
- `FileUploadPanel.tsx`
- `ContextSummaryPanel.tsx`
- `PrivateFilesTable.tsx`

Server state for external data sources is owned by:

- `services/dataSourceService.ts`
- `hooks/useDataSources.ts`
- backend `data_sources` table
- backend handlers in `internal/server/data_connector_handlers.go`

No Data Private panel should display fake source counts, fake sync state, or
fake Drive media. Google Drive stays `needs_auth` until read-only Drive OAuth is
implemented.

## Outbound Automation State

Outbound execution is API-backed through `outbound_messages`. Draft remains the
default status, but explicit auto/execute prompts can queue rows as `approved`
for local agents to send immediately. The final backend gate is
`CanQueueOutboundForOrg`, which blocks duplicate comments, repeated inboxes
inside cooldown, closed conversations, and non-org-scoped targets.

Dashboard chat and Telegram must both call the org-scoped AI path
`ProcessPromptForOrg`. Tool calls such as `search_groups`,
`comment_all_leads`, `inbox_all_leads`, and `create_job_post` must produce real
jobs/outbox rows, not fake UI state.

## Design Token System

Replace all inline style factories with typed constants:
```typescript
// constants/styles.ts
export const colors = {
  bg: '#0d101a',
  surface: '#1e2130',
  border: '#2a2f45',
  muted: '#9ca3af',
  primary: '#4f46e5',
  // ...
}

export const statusColor: Record<string, string> = {
  Hot: '#ef4444', Warm: '#f59e0b', Cold: '#3b82f6',
  Active: '#22c55e', Converted: '#6366f1', Pending: '#6b7280',
  // ...
}
```
