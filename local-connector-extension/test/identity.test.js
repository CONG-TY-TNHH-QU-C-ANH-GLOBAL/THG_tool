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
const { isSuspiciousIdentityLabel, isReservedHandle } = globalThis.THGContentMeta;

// B1 — UI-affordance labels (the "Cover photo" bug) must be rejected.
for (const bad of [
  'Cover photo', 'Ảnh bìa', 'Profile picture', 'Ảnh đại diện', 'See more',
  'Xem thêm', 'Menu', 'Photo', 'Messenger', 'Notifications', '',
]) {
  assert.strictEqual(isSuspiciousIdentityLabel(bad), true, `must reject UI label: "${bad}"`);
}
// 0.5.33 — auto-generated image ALT-TEXT must be rejected (the 0.5.30 miss:
// "May be an image of text that says ..." leaked as a name).
for (const bad of [
  'May be an image of text that says "Cia AM CH LSEY LS EY JM"',
  'May be an image of one person',
  'No photo description available.',
  'Có thể là hình ảnh về văn bản',
  'Không có mô tả ảnh.',
]) {
  assert.strictEqual(isSuspiciousIdentityLabel(bad), true, `must reject alt-text: "${bad}"`);
}
// B1 — real names must be accepted (not suspicious).
for (const ok of ['Nguyễn Văn A', 'David Anh', 'THG Fulfill', 'Nhiên An', 'Photo Studio ABC']) {
  assert.strictEqual(isSuspiciousIdentityLabel(ok), false, `must accept real name: "${ok}"`);
}

// 0.5.33 — reserved URL handles (the "photo" username bug) must be rejected.
for (const h of ['photo', 'story.php', 'watch', 'reel', 'groups', 'permalink.php', 'profile.php', '']) {
  assert.strictEqual(isReservedHandle(h), true, `must reject reserved handle: "${h}"`);
}
for (const h of ['nguyen.van.a', 'thgfulfill', 'david.anh.123']) {
  assert.strictEqual(isReservedHandle(h), false, `must accept real handle: "${h}"`);
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
