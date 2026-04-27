# KNOWN_ISSUES.md
# AutoFlow Platform — Known Bugs & Technical Debt
# Source: autoflow_platform.experimental.tsx audit (2026-04-27)

## CRITICAL — Must fix before production

### KI-001: Temporal Dead Zone crash
- **File:** autoflow_platform.experimental.tsx:419
- **Code:** `const [conn,setConn]=useState(false);loading`
- **Problem:** `loading` appears as an expression statement after the semicolon. `loading` is declared with `const` on line 420. Under ES6 strict mode + TDZ semantics, accessing a `const` binding before its declaration is a `ReferenceError`. In any properly configured bundler with strict mode, this file cannot execute.
- **Fix:** Remove the stray `;loading` text from line 419. It was likely a typo during editing.
- **Status:** OPEN

### KI-002: Content component re-created inside MainApp render
- **File:** autoflow_platform.experimental.tsx:437
- **Code:** `const Content=()=>{...}` inside the `MainApp` function body
- **Problem:** A new function reference is created on every render of `MainApp`. React treats this as a different component type, causing full unmount+remount of all tab content on every state update (tab switch, filter change, etc.). This is O(n) unnecessary DOM operations.
- **Fix:** Move each tab view to a named exported function component and import it at module level. Render directly: `{tab === 'leads' && <LeadsView .../>}`.
- **Status:** OPEN

### KI-003: `@keyframes spin` style tag rendered inside component
- **File:** autoflow_platform.experimental.tsx:618
- **Code:** `<style>{\`@keyframes spin{to{transform:rotate(360deg)}}\`}</style>` inside `MainApp` return
- **Problem:** Style node is re-injected on every render. Browser may apply the style multiple times.
- **Fix:** Move to `autoflow.css`, import at module level.
- **Status:** OPEN

---

## HIGH — Causes feature breakage

### KI-004: `cmts` field missing from StaffMember data
- **File:** autoflow_platform.experimental.tsx:9 (STAFF0), 576
- **Problem:** `STAFF0` array defines staff without a `cmts` field. Leaderboard uses `s.cmts||"—"` which evaluates to `"—"` for all staff. Comment column is permanently broken.
- **Fix (mock):** Add `cmts: <number>` to each entry in STAFF0.
- **Fix (real):** `StaffMember` interface must include `cmts: number`; backend must return it; `GET /orgs/:orgId/staff` response must include it.
- **Spec impact:** API_SPEC.md StaffMember interface already corrected.
- **Status:** OPEN

### KI-005: KPI penalty config key mismatch
- **File:** autoflow_platform.experimental.tsx:422, 582
- **Problem:** `cfg` initialized with `penAmt: 100000` (line 422). Penalty display on line 582 checks `cfg.penAmt?.toLocaleString()||cfg.penaltyAmt?.toLocaleString()||0`. The `penaltyAmt` key does not exist in `cfg`. The `||0` rescues the display showing `0` instead of correct amount. Admin-configured penalties are silently not displayed.
- **Fix:** Remove `cfg.penaltyAmt` reference. Use only `cfg.penAmt`.
- **Status:** OPEN

### KI-006: KPI scoring operator precedence bug
- **File:** autoflow_platform.experimental.tsx:433
- **Code:** `s.convs*cfg.conv+s.converted*cfg.conv2+s.cmts*(cfg.cmt||2)||s.convs*cfg.conv+s.converted*cfg.conv2`
- **Problem:** `||` has lower precedence than `+`. If the entire sum is `0` (possible for a staff member with zero activity), the expression falls through to the right side, dropping comment points entirely and computing a duplicate. This is also a maintenance hazard — the formula is duplicated.
- **Fix:**
  ```typescript
  const pts = (s.convs * cfg.conv) + (s.converted * cfg.conv2) + ((s.cmts ?? 0) * cfg.cmt)
  ```
- **Status:** OPEN

### KI-007: Org switch does not scope data
- **File:** autoflow_platform.experimental.tsx:416, 432
- **Problem:** `const [org,setOrg]=useState(ORGS[0])` switches the UI org label and color, but `llist`, `THREADS`, `POSTS`, `CMTS` all read from global unscoped mock arrays. A different org shows the same data.
- **Fix:** All data queries must be parameterized by `orgId`. Implemented via hooks per FRONTEND_CONTRACT.md.
- **Status:** OPEN (requires API wiring)

### KI-008: Staff tab accessible to staff role via setTab
- **File:** autoflow_platform.experimental.tsx:612, 428–429
- **Problem:** TABS for staff does not include `"settings"`. However, `tab` state is a string and `setTab` is a closure. The `SettingsPage` render branch at line 612 has no role guard: `if(tab==="settings") return <SettingsPage org={org}/>`. If a staff user could call `setTab("settings")` (e.g., via browser dev tools manipulating React state, or a UI regression), they'd get full admin settings.
- **Fix:** Add role guard: `if(tab==="settings" && isAdmin) return <SettingsPage org={org}/>`.
- **Status:** OPEN

---

## MEDIUM — UX/display issues

### KI-009: File upload zone has no upload logic
- **File:** autoflow_platform.experimental.tsx:590–594
- **Problem:** `onDrop={()=>setDrag(false)}` only resets drag visual state. `e.dataTransfer.files` is never read. No file is actually uploaded. The upload zone is decorative.
- **Fix:** Implement `useFiles` hook with actual `POST /orgs/:orgId/files` call.
- **Status:** OPEN

### KI-010: Fragment list missing `key` prop
- **File:** autoflow_platform.experimental.tsx:188
- **Code:** `{[1,2].map(s=><><div key={s} ...>`
- **Problem:** The outer fragment `<>` has no `key`. React warns about this. The `key` on the inner `<div>` does not satisfy React's requirement for the list item root element.
- **Fix:** `{[1,2].map(s=><Fragment key={s}><div ...>`
- **Status:** OPEN

### KI-011: Auth login hardcodes admin role
- **File:** autoflow_platform.experimental.tsx:172
- **Code:** `onClick={()=>onSuccess("admin")}`
- **Problem:** Login button always calls `onSuccess("admin")` regardless of credentials. `Staff login →` button (line 176) calls `onSuccess("staff")` — also without credential validation. Authentication is theater.
- **Fix:** Wire `authService.login(email, password)` and derive role from JWT response.
- **Status:** OPEN (requires API wiring)

### KI-012: Fixed height container clips content
- **File:** autoflow_platform.experimental.tsx:617, 499
- **Problem:** `MainApp` root div has `height:630` (px, not responsive). Inbox panel has `height:420`. On screens smaller than these heights or when content grows (e.g., many staff members in leaderboard), content is clipped.
- **Fix:** Replace fixed heights with `minHeight` + `flex` or viewport units.
- **Status:** OPEN

### KI-013: `select` options have no `value` props
- **File:** autoflow_platform.experimental.tsx:208–210
- **Problem:** `<option>Sản xuất</option>` — no `value` attribute. On submit, the form captures the display text. If the backend expects machine values (e.g., `"manufacturing"` not `"Sản xuất"`), values will fail validation.
- **Fix:** Add `value="..."` to all `<option>` elements.
- **Status:** OPEN

---

## LOW — Code quality / maintenance

### KI-014: Cryptic variable names throughout
- `D` → `rootStyles`, `PB` → `primaryButton`, `SB` → `secondaryButton`
- `sc` → `statusColor`, `lf` → `leadFilter`, `athr` → `activeThread`
- `ns` → `newStaffForm`, `conn` → `isFacebookConnected`
- `Av` → `Avatar`, `Bdg` → `Badge`, `Lbl` → `Label`, `Inp` → `Input`
- **Impact:** Onboarding time for new devs, maintenance errors
- **Fix:** Rename systematically during extraction into component files.

### KI-015: No TypeScript types
- **File:** Entire file
- **Problem:** All props, state, and data are untyped. Type errors are not caught at compile time.
- **Fix:** Add types as specified in FRONTEND_CONTRACT.md during extraction.

### KI-016: No accessibility attributes
- **File:** All interactive elements
- **Problem:** No `aria-label`, `role`, `tabIndex` on custom interactive elements. Screen readers cannot navigate the app.
- **Fix:** Add ARIA attributes during component extraction.

### KI-017: Inline style objects allocated on every render
- **File:** Lines 15–20 and throughout
- **Problem:** `card()`, `PB()`, `SB()` create new objects on every call. Not a problem at prototype scale; at production scale with hundreds of components this adds GC pressure.
- **Fix:** Promote to `useMemo` for dynamic styles, or module-level `const` for static styles.

---

## Resolved

_None yet — this is the initial audit._
