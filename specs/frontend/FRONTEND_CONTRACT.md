# FRONTEND_CONTRACT.md
# Component Contracts & Prop Interfaces
# Defines the public API each component must satisfy post-refactor

## UI Primitives (src/modules/autoflow/components/ui/)

### Avatar
```typescript
interface AvatarProps {
  text: string          // 1–2 chars to display
  bg?: string           // background color, default "#4f46e5"
  size?: number         // px, default 30
}
```

### Badge
```typescript
interface BadgeProps {
  label: string         // must match a key in statusColor map
}
// statusColor keys: Hot|Warm|Cold|Active|Converted|Pending|Live|Ended|Suspended|Enterprise|Pro|Starter
```

### Label
```typescript
interface LabelProps {
  text: string
}
```

### Input (wraps <input>)
```typescript
// Passes all HTMLInputElement props through
// Applies autoflow dark-theme styles
type InputProps = React.InputHTMLAttributes<HTMLInputElement>
```

### Row
```typescript
interface RowProps extends React.HTMLAttributes<HTMLDivElement> {
  style?: React.CSSProperties
  children: React.ReactNode
}
// Applies: display:flex, alignItems:center
```

---

## Page Components

### Landing
```typescript
interface LandingProps {
  onLogin: () => void
  onRegister: () => void
  onAdmin: () => void
}
```

### Auth
```typescript
interface AuthProps {
  mode: 'login' | 'register' | 'forgot' | 'success'
  onModeChange: (mode: AuthProps['mode']) => void
  onSuccess: (role: 'admin' | 'staff') => void
  onBack: () => void
}
// NOTE: 'success' view is conceptually a separate confirmation screen,
// consider splitting into AuthSuccess component
```

### SuperAdmin
```typescript
interface SuperAdminProps {
  onBack: () => void
}
```

### MainApp
```typescript
interface MainAppProps {
  role: 'admin' | 'staff'
  onLogout: () => void   // was: goLanding
}
```

### SettingsPage
```typescript
interface SettingsPageProps {
  org: Organization      // from OrgState.activeOrg
}
```

---

## View Components (tab content — all lazy-loaded)

### LeadsView
```typescript
interface LeadsViewProps {
  orgId: string
  isAdmin: boolean
}
// Internal state: statusFilter ('All'|'Hot'|'Warm'|'Cold')
// Data: useLeads(orgId, statusFilter)
```

### BrowserView
```typescript
interface BrowserViewProps {
  orgId: string
}
// Data: useFacebookSession(orgId)
```

### InboxView
```typescript
interface InboxViewProps {
  orgId: string
}
// Data: useThreads(orgId), useMessages(threadId)
// Internal state: activeThreadId
```

### PostingView
```typescript
interface PostingViewProps {
  orgId: string
}
// Data: usePosts(orgId)
// Internal state: statusFilter
```

### CommentingView
```typescript
interface CommentingViewProps {
  orgId: string
}
// Data: useComments(orgId)
```

### LeaderboardView
```typescript
interface LeaderboardViewProps {
  orgId: string
  isAdmin: boolean
}
// Data: useStaff(orgId), useKpiConfig(orgId)
// Internal state: showConfig (admin only)
```

### DataPrivateView
```typescript
interface DataPrivateViewProps {
  orgId: string
}
// Data: useFiles(orgId)
// Internal state: isDragging
```

---

## Hooks

### useAuth
```typescript
interface UseAuthReturn {
  user: AuthUser | null
  role: Role
  orgId: string | null
  token: string | null
  login: (email: string, password: string) => Promise<void>
  logout: () => void
  isAuthenticated: boolean
}
```

### useOrg
```typescript
interface UseOrgReturn {
  activeOrg: Organization | null
  orgs: Organization[]
  switchOrg: (orgId: string) => void
  isLoading: boolean
}
```

### useLeads
```typescript
interface UseLeadsReturn {
  leads: Lead[]
  isLoading: boolean
  error: Error | null
  refetch: () => void
}
function useLeads(orgId: string, status?: LeadStatus | 'All'): UseLeadsReturn
```

### useThreads
```typescript
interface UseThreadsReturn {
  threads: Thread[]
  messages: (threadId: number) => Message[]
  sendMessage: (threadId: number, content: string) => Promise<void>
  isLoading: boolean
}
function useThreads(orgId: string): UseThreadsReturn
```

### useFacebookSession
```typescript
interface UseFacebookSessionReturn {
  status: FacebookStatus
  connect: () => Promise<void>
  disconnect: () => Promise<void>
  isConnecting: boolean
}
function useFacebookSession(orgId: string): UseFacebookSessionReturn
```

### useLeaderboard
```typescript
interface UseLeaderboardReturn {
  scored: ScoredStaff[]
  config: KPIConfig
  updateConfig: (config: KPIConfig) => Promise<void>
  isSaving: boolean
}
interface ScoredStaff extends StaffMember {
  pts: number
}
function useLeaderboard(orgId: string): UseLeaderboardReturn
```

### useFiles
```typescript
interface UseFilesReturn {
  files: FileRecord[]
  upload: (file: File) => Promise<void>
  remove: (fileId: number) => Promise<void>
  isUploading: boolean
}
function useFiles(orgId: string): UseFilesReturn
```

### useStaff
```typescript
interface UseStaffReturn {
  staff: StaffMember[]
  add: (data: { name: string; email: string; role: string }) => Promise<void>
  toggleStatus: (staffId: number) => Promise<void>
  remove: (staffId: number) => Promise<void>
  isLoading: boolean
}
function useStaff(orgId: string): UseStaffReturn
```

---

## Services (no React — pure async functions)

### api.ts
```typescript
// Base client — inject auth token from AuthStore
function get<T>(path: string): Promise<T>
function post<T>(path: string, body: unknown): Promise<T>
function put<T>(path: string, body: unknown): Promise<T>
function del(path: string): Promise<void>
function upload<T>(path: string, file: File): Promise<T>
```

All services use these base functions. Services do NOT access React state directly — they accept tokens as parameters or read from a non-React auth store.

---

## RBAC Contract

| Component/Action | admin | staff | super_admin |
|---|---|---|---|
| Browser tab visible | ✓ | ✗ | ✗ |
| Posting tab visible | ✓ | ✗ | ✗ |
| Commenting tab visible | ✓ | ✗ | ✗ |
| Settings accessible | ✓ | ✗ | ✗ |
| Leaderboard KPI config | ✓ | ✗ | ✗ |
| Staff management | ✓ | ✗ | ✗ |
| Org switcher visible | ✓ | ✗ | ✗ |
| My Leads (own only) | — | ✓ | ✗ |
| Super Admin portal | ✗ | ✗ | ✓ |

**Enforcement rule:** RBAC must be checked at the hook/service layer, not only in UI visibility. A `staff` user calling `staffService.add()` directly must receive 403 from the API. The frontend tab visibility is a UX concern only.

---

## Multi-tenancy Contract

Every data hook and service call MUST include `orgId`. When `OrgState.switchOrg()` is called:
1. TanStack Query invalidates all queries for the previous org
2. `orgId` in auth context updates
3. All views re-fetch with the new `orgId`
4. Local UI state (filters, active thread, etc.) resets

No data array may ever be an unscoped global variable.
