# THG Chrome Extension Helper

This extension verifies a signed-in personal Chrome/Facebook session.
Dashboard browser streaming and multi-account automation are handled by
THG Local Runtime, which runs isolated local Chrome profiles on the user's
device/IP.

Production UX:

1. The dashboard creates a short-lived pairing code.
2. The user installs this extension into the same personal Chrome profile where
   Facebook is already trusted and signed in.
3. The user pastes the pairing code into the extension popup.
4. The extension reports Facebook tab status and `c_user` presence to the backend.
5. For dashboard Browser streaming, the user pairs and runs THG Local Runtime.

The extension does not receive a Facebook password and does not try to bypass
Facebook checkpoint or CAPTCHA flows.
