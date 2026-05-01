# Local Connector Production Workflow

## Direction

Cloud browser remains available, but it is no longer the only production path
for Facebook accounts. The primary scalable path is:

1. Staff signs in to the THG workspace.
2. Staff creates a short-lived pairing code from the Browser dashboard.
3. THG Local Connector runs on the staff machine where Chrome/Facebook is
   already trusted.
4. The connector claims the pairing code at `/api/connectors/pair` and receives
   a long-lived device token that is never shown in the dashboard.
5. The connector heartbeats to `/api/agent/heartbeat` with device, Facebook tab,
   stream, and account status.
6. Cloud backend keeps prompt workflows, business memory, leads, outbox,
   staff KPI, audit logs, and routing policy.
7. The connector executes Facebook actions locally and reports results back to
   the cloud.

This preserves the value of a visible Facebook workflow while avoiding cloud
login risk from unfamiliar datacenter Chrome profiles.

## Invariants

- One connector belongs to exactly one `org_id`.
- A dashboard pairing code is short-lived, one-time, stored hashed, and shown
  only during setup.
- A connector device token is returned only to the local app after successful
  pairing and stored hashed in `agent_tokens`.
- A connector can be bound to one Facebook account slot, but one workspace may
  have many connector/account pairs.
- Dashboard data is fetched from real connector heartbeats, not mocked UI.
- If Facebook shows checkpoint/CAPTCHA, the connector must report
  `human_required`; automation must not bypass it.
- Cloud browser CDP polling must stay opt-in during login/checkpoint flows.

## Current Implementation

- `agent_tokens` now stores connector metadata:
  `kind`, `transport`, `assigned_account_id`, `capabilities_json`,
  `current_url`, `fb_user_id`, and `stream_status`.
- `GET /api/connectors` lists org-scoped Local Connector devices.
- `POST /api/connectors` is a legacy alias for creating a short-lived pairing
  code. It must not expose long-lived connector tokens to the dashboard.
- `POST /api/connectors/pairing-code` creates a short-lived dashboard pairing
  code.
- `POST /api/connectors/pair` lets the desktop connector exchange the pairing
  code for its device token.
- `PUT /api/connectors/:id/account` binds a connector to an account slot.
- `DELETE /api/connectors/:id` revokes the connector token.
- `/api/agent/heartbeat` accepts richer presence fields from desktop or
  extension connectors.
- Browser dashboard shows Local Connector devices beside cloud browser
  workspaces.
- Local Connector binaries are built with `scripts/build-local-connector.sh`
  or `scripts/build-local-connector.ps1` and served from
  `data/downloads` through `/downloads/*`.

## Next Milestones

1. Desktop Connector app:
   - stores token securely,
   - discovers Chrome tabs,
   - reports current Facebook URL/user,
   - executes outbox/comment/inbox jobs locally.
2. Live observation:
   - WebRTC tab/window stream from connector to dashboard,
   - action timeline and pause/resume control.
3. Extension helper:
   - reads DOM metadata safely,
   - normalizes post/comment/inbox extraction,
   - sends structured events to the desktop connector.
4. Routing:
   - jobs prefer an online connector bound to the selected account,
   - fallback to cloud browser only when explicitly enabled.
