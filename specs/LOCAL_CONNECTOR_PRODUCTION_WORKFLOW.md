# Chrome Extension Connector Production Workflow

This document replaces the previous connector flow. Production should use
one connector path only: THG Chrome Extension installed inside the user's
trusted Chrome profile.

## User Flow

1. Staff opens the Browser dashboard.
2. Staff clicks `Install from Chrome Web Store`.
3. Staff installs the signed THG Chrome Extension in the Chrome profile that
   already has the intended Facebook account.
4. Staff creates a short-lived pairing code from the Browser dashboard.
5. Staff pastes the code into the extension popup.
6. The extension exchanges the one-time code for a device token.
7. Staff opens a logged-in Facebook tab.
8. Browser dashboard receives stream frames, Facebook identity, crawl results,
   input acknowledgements, and outbound execution logs from the extension.

## Security Model

- The extension never asks for or stores a Facebook password.
- The hard Facebook identity is `c_user`; display name, username and profile
  URL are operator labels only.
- Pairing codes are short-lived and one-time-use.
- Device tokens can be revoked from the Browser dashboard.
- Every connector token is org-scoped and may be assigned to a specific
  Facebook account slot.
- Outbound actions still pass `QueueOutboundForOrg`, dedup, cooldown,
  conversation state and org auto-mode policy before the extension can execute.
- Checkpoints and CAPTCHA remain human-required. THG does not bypass them.

## Implementation State

- Connector API endpoints live under `/api/connectors/*` and keep `/api/agent/*`
  aliases for old integrations.
- Pairing/token state lives in `agent_tokens`.
- Stream/session state lives in `browser_sessions`.
- Dashboard input and crawl commands use `connector_commands`.
- Approved outbound messages are polled via `/api/connectors/outbox`.
- `CHROME_EXTENSION_ID` is enough to enable the production install button;
  `CHROME_EXTENSION_STORE_URL` can override the generated Web Store link.
- While Chrome Web Store review is pending, internal testers can enable a
  temporary beta lane with `CHROME_EXTENSION_BETA_ENABLED=true` and point the
  dashboard to a secure install guide or artifact URL. If the package lives on
  the VPS, set `CHROME_EXTENSION_BETA_PACKAGE_PATH` and the API will expose it
  at `/api/system/extension-beta-package`. This lane should be turned off after
  approval.
- The zip produced by `scripts/build-chrome-extension.ps1` and
  `scripts/build-chrome-extension.sh` is only for Chrome Web Store upload or
  CI validation. It is not served to users.

## Next Milestones

1. Add extension auto-update/version enforcement on the dashboard.
2. Harden structured DOM extraction in content scripts:
   - post metadata
   - author profile
   - comments
   - Messenger/fanpage inbox threads
3. Add a production fanpage-care adapter before enabling `scan_fanpage_inbox`
   and `care_fanpage` live execution.
