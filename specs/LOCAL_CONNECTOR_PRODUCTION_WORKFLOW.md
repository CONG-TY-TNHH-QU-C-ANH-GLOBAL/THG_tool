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
- The user who paired a device can disconnect that device from the workspace in
  the Browser dashboard. Admin/founder users can disconnect org devices for
  operations and incident response.
- A connector device may run many local Chrome account profiles. Each Facebook
  account slot gets its own persistent local Chrome `user-data-dir` and DevTools
  port so sessions do not overwrite each other.
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
- When an org has an active Local Connector, `POST /api/browser/workspaces/new`
  creates a local account slot instead of starting a cloud browser.
- `POST /api/browser/workspaces/:id/start` marks that account as
  `local_starting`; the connector polls `/api/agent/browser-targets` and opens a
  matching local Chrome profile.
- The connector captures the visible Chrome frame through CDP and posts it to
  `/api/agent/screenshot`; the dashboard reads it from
  `/api/connectors/screen?account_id=...`.
- `local_ready`, `local_login_required`, and `local_human_required` are treated
  as first-class browser states in the dashboard.
- `DELETE /api/connectors/:id` revokes the device token, clears streamed local
  screenshots for that device, and marks its local sessions stopped. The
  connector exits cleanly on the next heartbeat when the token is rejected.
- Local Connector binaries are built with `scripts/build-local-connector.sh`
  or `scripts/build-local-connector.ps1` and served from
  `data/downloads` through `/downloads/*`.

## Next Milestones

1. Desktop Connector app:
   - execute approved outbox/comment/inbox jobs locally,
   - expose pause/resume per account profile,
   - surface profile mismatch repair actions.
2. Live observation:
   - upgrade screenshot polling to WebRTC/WebSocket streaming,
   - add input forwarding only with explicit user consent,
   - action timeline and pause/resume control.
3. Extension helper:
   - reads DOM metadata safely,
   - normalizes post/comment/inbox extraction,
   - sends structured events to the desktop connector.
4. Routing:
   - jobs prefer an online connector bound to the selected account,
   - fallback to cloud browser only when explicitly enabled.
