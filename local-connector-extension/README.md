# THG Chrome Extension Helper

This extension is the primary production Local Connector path.

Production UX:

1. The dashboard creates a short-lived pairing code.
2. The user installs this extension into the same personal Chrome profile where
   Facebook is already trusted and signed in.
3. The user pastes the pairing code into the extension popup.
4. The extension reports Facebook tab status, `c_user` presence, and active-tab
   screenshots to the backend when a Browser workspace target is active.

The extension does not receive a Facebook password and does not try to bypass
Facebook checkpoint or CAPTCHA flows.
