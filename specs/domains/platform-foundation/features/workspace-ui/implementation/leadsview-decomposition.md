# LeadsView Decomposition (refactor-only follow-up)

**Track:** Frontend view decomposition. **Status:** PLAN (not yet implemented).
**Type:** Refactor-only — MUST NOT change behavior (no new filters, no API changes,
no copy changes). Tests/build are the contract: `npm --prefix frontend run build`
stays green and the rendered UI is identical.

## Why

`frontend/src/modules/autoflow/components/views/LeadsView.tsx` is ~1040 lines — a
god view on the file-size allowlist (CLAUDE.md Engineering Guardrails). The Lead
Lifecycle work (PR-4) added the lifecycle tabs as a new component but still had to
wire ~30 lines into LeadsView (tab state, source switch, counts, render). Further
lifecycle UI (date-bucket chips, coverage filters, reply filter, archived restore
button) must NOT be added inline — the view has to be split first.

## Target structure (feature-folder, each file < 200 lines)

```
components/leads/
  LeadsView.tsx          # thin container: data hooks + compose panels (< 150)
  LeadFiltersPanel.tsx   # sidebar: LifecycleTabs + score/role/intent filters + search
  LeadListPanel.tsx      # middle pane: the lead rows + engagement/lifecycle badges
  LeadDetailPanel.tsx    # right pane: selected lead detail + actions (incl. archive/restore)
  LeadSummaryStats.tsx   # the top stat strip (total / hot / warm / avg score)
  LifecycleTabs.tsx      # already extracted (PR-4)
hooks/
  useLeadFilters.ts      # filter state + the filteredLeads predicate (pure-ish)
```

## Extraction order (each step builds + renders identically)

1. **LeadSummaryStats** — pure presentational; pass `totals` as props. Lowest risk.
2. **LifecycleTabs** — done (PR-4).
3. **LeadFiltersPanel** — move the sidebar JSX + the `FILTERS`/`ROLE_FILTERS`/
   `INTENT_FILTERS` consts; lift filter state into `useLeadFilters`.
4. **LeadListPanel** — move the list rendering + row helpers
   (`engagementContext`, `relativeTime`, badge mapping).
5. **LeadDetailPanel** — move the detail pane; this is where the deferred
   **archived restore button** (wire `useArchivedLeads().restore`) and the manual
   **archive action** (`lifecycleService.archiveLead`) land — but as a SEPARATE
   behavior PR after the split, never mixed into the refactor.
6. **useLeadFilters** — extract the `filteredLeads`/counts `useMemo`s last, once
   panels consume props.

## Rules

- One panel per PR; each PR is refactor-only and states so in its report.
- No logic change — move + re-prop only (feedback_extraction_is_not_redesign).
- Remove `LeadsView.tsx` from `scripts/file_size_allowlist.txt` once it is < 200.
- New deferred UI (filter chips, restore/archive buttons) ships as behavior PRs
  AFTER the split, with tests — see [[project_lead_lifecycle_work_queue]].
