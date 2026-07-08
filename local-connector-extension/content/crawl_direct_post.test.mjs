// P1.3E direct-post target-aware extraction — pure-gate unit tests. These mirror the backend
// directpost.Validate invariants on the extension side so a poisoned candidate never leaves the
// browser. The DOM scan + bounded async wait (selectDirectPostTargetItem / crawlDirectPostTarget)
// need a live FB DOM and are covered by the documented manual smoke plan, not here.
//   Run: node content/crawl_direct_post.test.mjs
import assert from 'node:assert';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
// crawl_post_identity sets globalThis.THGCrawlIdentity, which directPostVerdict
// uses (extractPostFBID) — load it before the direct-post gate module.
require('./facebook/crawl_post_identity.js');
const C = require('./facebook/crawl_direct_post.js');

const TARGET = { post_fbid: '2047168032857197', group_ref: '1312868109620530' };

// --- boilerplate detection (the "Facebook Facebook…" production content) ---
assert.strictEqual(C.directPostBoilerplate('Facebook Facebook Facebook Facebook'), true, 'repeated Facebook chrome is boilerplate');
assert.strictEqual(C.directPostBoilerplate('Like Comment Share'), true, 'UI actions only is boilerplate');
assert.strictEqual(C.directPostBoilerplate('   '), true, 'empty is boilerplate');
assert.strictEqual(C.directPostBoilerplate('Em ở Q7 cần gửi hàng đông lạnh đi Texas ạ'), false, 'a real post is not boilerplate');

// --- verdict: identity match gating ---
// Wrong post id → not the requested post (keep scanning), never a hard fail.
assert.deepStrictEqual(
  C.directPostVerdict({ post_fbid: '9999999999', source_url: 'https://www.facebook.com/groups/1312868109620530/permalink/9999999999/', content: 'real shipping post here ok' }, TARGET),
  { match: false }, 'different post id must not match',
);

// Production bug: source_url IS the requested permalink but author_profile_url is a FOREIGN group.
assert.deepStrictEqual(
  C.directPostVerdict({
    post_fbid: TARGET.post_fbid,
    source_url: 'https://www.facebook.com/groups/1312868109620530/permalink/2047168032857197/',
    author_profile_url: 'https://www.facebook.com/groups/976129910262051/user/100029/',
    content: 'Tuyển dụng Front-end ReactJS, VueJS, Angular real text',
  }, TARGET),
  { match: true, ok: false, reason: 'direct_post_group_mismatch' }, 'foreign-group author must be group_mismatch',
);

// Identity matches but content is FB boilerplate → boilerplate failure.
assert.deepStrictEqual(
  C.directPostVerdict({ post_fbid: TARGET.post_fbid, source_url: 'https://www.facebook.com/groups/1312868109620530/permalink/2047168032857197/', author_profile_url: 'https://www.facebook.com/nhii.tran', content: 'Facebook Facebook Facebook Facebook' }, TARGET),
  { match: true, ok: false, reason: 'direct_post_boilerplate_content' }, 'boilerplate content must fail typed',
);

// Clean target: correct post id, no foreign group, real content → ok.
assert.deepStrictEqual(
  C.directPostVerdict({ post_fbid: TARGET.post_fbid, source_url: 'https://www.facebook.com/groups/1312868109620530/permalink/2047168032857197/', author_profile_url: 'https://www.facebook.com/nhii.tran', content: 'Em ở Q7 cần gửi hàng đông lạnh đi Texas ạ' }, TARGET),
  { match: true, ok: true }, 'clean target must be emittable',
);

// post id resolvable from source_url when post_fbid field is empty.
assert.strictEqual(
  C.directPostVerdict({ post_fbid: '', source_url: 'https://www.facebook.com/groups/1312868109620530/permalink/2047168032857197/', author_profile_url: 'https://www.facebook.com/nhii.tran', content: 'Em ở Q7 cần gửi hàng đông lạnh đi Texas ạ' }, TARGET).ok,
  true, 'post id from source_url must resolve',
);

console.log('crawl direct-post target gate: PASS');
