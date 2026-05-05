# THG Chrome Extension Helper

This extension is the production connector for a signed-in Chrome/Facebook
session. It pairs one Chrome profile with one THG workspace account slot,
streams the visible Facebook tab to the Browser dashboard, accepts dashboard
input commands, runs prompt-scoped crawl commands, and executes approved
outbox actions.

Production UX:

1. The dashboard creates a short-lived pairing code.
2. The user installs this extension from Chrome Web Store into the same personal
   Chrome profile where Facebook is already trusted and signed in.
3. The user pastes the pairing code into the extension popup.
4. The extension reports Facebook tab status and `c_user` presence to the backend.
5. The Browser dashboard shows the Facebook tab stream and all automation logs.

The extension does not receive a Facebook password and does not try to bypass
Facebook checkpoint or CAPTCHA flows.

Production install goes through Chrome Web Store. The build package is only for
Chrome Web Store upload/validation.
