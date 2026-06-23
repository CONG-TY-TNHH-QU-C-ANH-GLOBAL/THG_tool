# API_SPEC.md
# AutoFlow Frontend ↔ Backend Contract
# Derived from: autoflow_platform.experimental.tsx mock data structures
# Backend: already exists, cannot be modified — this spec documents the REQUIRED contract

## Base URL
```
/api/v1
```
All requests require `Authorization: Bearer <token>` except auth endpoints.

## Data Types

```typescript
type Role = 'admin' | 'staff' | 'super_admin'
type PlanTier = 'Starter' | 'Pro' | 'Enterprise'
type LeadStatus = 'Hot' | 'Warm' | 'Cold'
type ThreadStatus = 'Active' | 'Converted' | 'Pending'
type PostStatus = 'Live' | 'Ended'
type StaffStatus = 'Active' | 'Suspended'
type OrgStatus = 'Active' | 'Suspended'

interface Organization {
  id: string
  name: string
  abbr: string        // 2–3 chars, e.g. "VF"
  plan: PlanTier
  color: string       // hex, e.g. "#4f46e5"
  status: OrgStatus
}

interface Lead {
  id: number
  name: string
  status: LeadStatus
  group: string       // Facebook group name
  agent: string       // Agent ID, e.g. "Agent_01"
  last: string        // relative time, e.g. "2g", "30p"
  score: number       // 0–100
  phone: string
  facebookUrl?: string
}

interface Thread {
  id: number
  lead: string
  agent: string
  last: string        // last message preview
  time: string
  status: ThreadStatus
  unread: number
}

interface Message {
  id: number
  from: 'lead' | 'agent'
  content: string
  time: string        // "HH:MM"
  agentId?: string
}

interface Post {
  id: number
  group: string
  content: string
  time: string
  likes: number
  comments: number
  shares: number
  status: PostStatus
}

interface Comment {
  id: number
  agent: string
  lead: string
  post: string
  comment: string
  time: string
}

interface StaffMember {
  id: number
  name: string
  email: string
  role: string        // "Sales" | "Senior Sales" | "Team Lead" | "Junior Sales"
  status: StaffStatus
  joined: string      // "DD/MM/YYYY"
  convs: number
  converted: number
  cmts: number        // NOTE: missing from current mock — must be added to backend response
}

interface FileRecord {
  id: number
  name: string
  size: string        // human readable, e.g. "2.4 MB"
  date: string        // "DD/MM"
  indexed: boolean
}

interface KPIConfig {
  conv: number        // points per conversation
  conv2: number       // points per converted lead
  cmt: number         // points per comment
  bonus: number       // threshold for bonus
  bonusAmt: number    // bonus amount in VND
  pen: number         // penalty threshold (below this = penalty)
  penAmt: number      // penalty amount in VND
}

interface FacebookStatus {
  connected: boolean
  account?: string    // Facebook account display name
  expiresAt?: string  // ISO date
  groups?: number
  leadsToday?: number
  agents?: number
}

// Super Admin types
interface OrgSummary {
  id: number
  name: string
  plan: PlanTier
  users: number
  status: OrgStatus
  joined: string      // "DD/MM/YYYY"
  rev: string         // e.g. "₫6.9M"
}
```

## Auth Endpoints

### POST /auth/login
```
Request:  { email: string, password: string }
Response: { token: string, role: Role, orgId: string, user: { id, name, email } }
Errors:   401 { error: "invalid_credentials" }
```

### POST /auth/register
```
Request:  {
  name: string,
  email: string,
  password: string,
  org: {
    name: string,
    industry: string,
    size: string,    // "1-5" | "6-20" | "21-50" | "50+"
    plan: PlanTier,
    referral?: string
  }
}
Response: { orgId: string, trialDays: number, admin: { email: string } }
```

### POST /auth/forgot
```
Request:  { email: string }
Response: 204 (always, never leak if email exists)
```

## Organization Endpoints

### GET /orgs
```
Response: Organization[]
Note: returns only orgs the authenticated user belongs to
```

### PUT /orgs/:orgId
```
Request:  Partial<{ name, abbr, color, industry }>
Response: Organization
```

## Leads Endpoints

### GET /orgs/:orgId/leads
```
Query:    ?status=Hot|Warm|Cold  (optional, omit for all)
Response: Lead[]
```

### POST /orgs/:orgId/leads
```
Request:  { name, phone, group, facebookUrl? }
Response: Lead
```

## Threads Endpoints

### GET /orgs/:orgId/threads
```
Response: Thread[]
```

### GET /orgs/:orgId/threads/:threadId/messages
```
Response: Message[]
```

### POST /orgs/:orgId/threads/:threadId/messages
```
Request:  { content: string }
Response: Message
```

## Posts Endpoints

### GET /orgs/:orgId/posts
```
Query:    ?status=Live|Ended  (optional)
Response: Post[]
```

### POST /orgs/:orgId/posts
```
Request:  { group: string, content: string }
Response: Post
```

## Comments Endpoint

### GET /orgs/:orgId/comments
```
Response: Comment[]
```

## Staff Endpoints

### GET /orgs/:orgId/staff
```
Response: StaffMember[]
```

### POST /orgs/:orgId/staff
```
Request:  { name: string, email: string, role: string }
Response: StaffMember
Note: Backend sends invite email; staff sets password on first login
```

### PUT /orgs/:orgId/staff/:staffId
```
Request:  { status: StaffStatus }
Response: StaffMember
```

### DELETE /orgs/:orgId/staff/:staffId
```
Response: 204
```

## Files Endpoints

### GET /orgs/:orgId/files
```
Response: FileRecord[]
```

### POST /orgs/:orgId/files
```
Request:  multipart/form-data  { file: File }
Response: FileRecord
Note: Backend indexes file for AI context asynchronously
```

### DELETE /orgs/:orgId/files/:fileId
```
Response: 204
```

## KPI Config Endpoints

### GET /orgs/:orgId/kpi-config
```
Response: KPIConfig
```

### PUT /orgs/:orgId/kpi-config
```
Request:  KPIConfig
Response: KPIConfig
```

## Facebook Session Endpoints

### GET /orgs/:orgId/facebook/status
```
Response: FacebookStatus
```

### POST /orgs/:orgId/facebook/connect
```
Response: { sessionId: string, expiresAt: string }
Note: Triggers backend Chrome session establishment
```

### DELETE /orgs/:orgId/facebook/session
```
Response: 204
```

## Super Admin Endpoints

### GET /admin/orgs
```
Response: OrgSummary[]
Requires: role = super_admin
```

### PUT /admin/orgs/:orgId/status
```
Request:  { status: OrgStatus }
Response: OrgSummary
```

## Error Response Format

```typescript
interface APIError {
  error: string       // machine-readable code
  message: string     // human-readable Vietnamese
  field?: string      // for validation errors
}
```

HTTP status codes: 200, 201, 204, 400, 401, 403, 404, 422, 500
