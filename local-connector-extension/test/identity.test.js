// Golden smoke for PR-B Facebook Identity Accuracy (no test runner needed).
//   Run: node local-connector-extension/test/identity.test.js
// Guards the two PR-B fixes:
//   B1 — meta.js never accepts a UI-affordance label ("Cover photo", "Ảnh bìa", …)
//        as the Facebook display name.
//   B2 — the c_user cookie is the primary fb_user_id source (regex parity with
//        proof.js currentFBUserID).
const fs = require('fs');
const path = require('path');
const assert = require('assert');

// meta.js is an IIFE that assigns globalThis.THGContentMeta and references
// THGContentShared only INSIDE collectFacebookMeta (not at load), so it loads
// standalone. Indirect eval runs it in global scope.
const metaSrc = fs.readFileSync(path.join(__dirname, '..', 'content', 'meta.js'), 'utf8');
(0, eval)(metaSrc); // eslint-disable-line no-eval
const { isSuspiciousIdentityLabel } = globalThis.THGContentMeta;

// B1 — UI-affordance labels (the "Cover photo" bug) must be rejected.
for (const bad of [
  'Cover photo', 'Ảnh bìa', 'Profile picture', 'Ảnh đại diện', 'See more',
  'Xem thêm', 'Menu', 'Photo', 'Messenger', 'Notifications', '',
]) {
  assert.strictEqual(isSuspiciousIdentityLabel(bad), true, `must reject UI label: "${bad}"`);
}
// B1 — real names must be accepted (not suspicious).
for (const ok of ['Nguyễn Văn A', 'David Anh', 'THG Fulfill', 'Nhiên An', 'Photo Studio ABC']) {
  assert.strictEqual(isSuspiciousIdentityLabel(ok), false, `must accept real name: "${ok}"`);
}

// B2 — c_user cookie extraction parity (proof.js primary source).
const cuser = (cookie) => {
  const m = (cookie || '').match(/(?:^|;\s*)c_user=(\d+)/);
  return m ? m[1] : '';
};
assert.strictEqual(cuser('locale=vi; c_user=100012345; xs=abc'), '100012345');
assert.strictEqual(cuser('c_user=999'), '999');
assert.strictEqual(cuser('datr=xyz; presence=1'), ''); // no c_user → unknown
assert.strictEqual(cuser(''), '');

console.log('PR-B identity golden smoke: PASS');
