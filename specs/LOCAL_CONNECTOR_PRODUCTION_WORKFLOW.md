# Local Connector Production Workflow

## Direction

The production path for Facebook must use the customer's own trusted Chrome
profile, not a fresh cloud Chrome login and not a separate managed Chrome
profile by default.

Primary UX:

1. Staff opens the Browser dashboard.
2. Staff downloads or installs **THG Chrome Extension**.
3. Staff keeps Facebook signed in on their normal personal/work Chrome profile.
4. Staff creates a short-lived pairing code from the Browser dashboard.
5. Staff pastes that code into the THG Extension popup.
6. The extension receives a long-lived device token stored only in Chrome local
   extension storage.
7. The extension heartbeats to the backend with Facebook tab status, current
   URL, `c_user` presence, and active-tab screenshots when a Browser workspace
   target is running.
8. Dashboard Browser view observes the real signed-in Facebook tab and routes
   prompt jobs to that connector.

The desktop app remains a native companion for future OS-level screen sharing,
pause/resume hotkeys, and advanced local execution. It must not be presented as
the main Facebook login path because asking users to switch Chrome profiles or
understand DevTools/remote-debugging increases checkpoint risk and support
friction.

## User-Facing Language

Avoid exposing these terms in the normal setup flow:

- remote debugging
- CDP
- DevTools port
- cloud profile
- user-data-dir

Use this language instead:

- "Cài THG Extension vào Chrome đang đăng nhập Facebook."
- "Tạo mã kết nối."
- "Dán mã vào popup extension."
- "Mở tab Facebook thật."
- "Dashboard đã nhận tín hiệu Facebook."

## Invariants

- One connector belongs to exactly one `org_id`.
- A dashboard pairing code is short-lived, one-time, stored hashed, and shown
  only during setup.
- A connector device token is returned only to the extension/native connector
  after successful pairing and stored hashed in `agent_tokens`.
- The user who paired a device can disconnect that device from the workspace in
  the Browser dashboard. Admin/founder users can disconnect org devices for
  operations and incident response.
- The production connector should be `extension_connector` with
  `transport=chrome_extension` when the user chooses personal Chrome.
- Dashboard data is fetched from real connector heartbeats and screenshots, not
  mocked UI.
- If Facebook shows checkpoint/CAPTCHA, the connector must report
  `human_required`; automation must not bypass it.
- Automation must not ask for or store the user's Facebook password.
- Cloud browser CDP polling must stay opt-in during login/checkpoint flows.

## Current Implementation

- `agent_tokens` stores connector metadata:
  `kind`, `transport`, `assigned_account_id`, `capabilities_json`,
  `current_url`, `fb_user_id`, and `stream_status`.
- `GET /api/connectors` lists org-scoped Local Connector devices.
- `POST /api/connectors/pairing-code` creates a short-lived dashboard pairing
  code.
- `POST /api/connectors/pair` lets the extension or native connector exchange
  the pairing code for its device token. It accepts `extension_connector` and
  `desktop_connector`.
- `/api/agent/heartbeat` accepts richer presence fields from extension/native
  connectors.
- `/api/agent/chrome-status` is the explicit Chrome/Facebook status endpoint,
  so the backend can show `chrome_not_connected`, `chrome_connected`, or
  `facebook_*` instead of pretending that a paired device is ready.
- `/api/agent/screenshot` stores active Facebook tab screenshots from the
  connector; the dashboard reads them from
  `/api/connectors/screen?account_id=...`.
- Browser dashboard shows Local Connector devices beside browser workspaces.
- When an org has an online connector, `POST /api/browser/workspaces/new`
  creates a local account slot instead of starting a cloud browser.
- `POST /api/browser/workspaces/:id/start` marks that account as
  `local_starting`; the extension polls `/api/agent/browser-targets` and reports
  the active Facebook tab for that target.
- `local_ready`, `local_login_required`, and `local_human_required` are treated
  as first-class browser states in the dashboard.
- `POST /api/connectors/:id/disconnect` is the dashboard disconnect endpoint.
  It revokes the device token, clears streamed local screenshots for that
  device, and marks its local sessions stopped. `DELETE /api/connectors/:id`
  remains as a compatibility alias.
- Browser start/new-session only uses Local Connector when a connector is
  online. If a device was paired but extension/app is closed, the API returns
  `LOCAL_CONNECTOR_OFFLINE` instead of silently waiting forever.
- `scripts/build-local-connector.sh` and `scripts/build-local-connector.ps1`
  package `local-connector-extension` as
  `data/downloads/thg-chrome-extension.zip`.

## Next Milestones

1. Publish THG Chrome Extension through Chrome Web Store so setup becomes a
   one-click install instead of zip sideloading.
2. Add a WebSocket command channel for the extension:
   - target account routing,
   - active tab claim/release,
   - pause/resume,
   - action timeline.
3. Add structured DOM extraction in content scripts:
   - post metadata,
   - comment thread metadata,
   - inbox state,
   - checkpoint state.
4. Add local action executor with safety policy:
   - no CAPTCHA bypass,
   - stop on checkpoint,
   - per-lead dedupe,
   - conversation memory before comment/inbox.
5. Keep desktop native companion optional for future full-screen observation or
   OS-level permissions, but do not make it part of the default Facebook login
   path.
