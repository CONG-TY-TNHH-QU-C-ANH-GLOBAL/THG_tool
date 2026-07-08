// PR-C1C characterization — THGCrawlIdentity pure URL/ID helpers, extracted
// verbatim from crawl.js. Pins the behavior the crawl loop depends on.
//   Run: node --test content/crawl_post_identity.test.mjs
import { test } from 'node:test';
import assert from 'node:assert';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const ID = require('./facebook/crawl_post_identity.js');
const ORIGIN = 'https://www.facebook.com';

test('extractPostFBID: permalink wins, photo fbid rejected, posts last', () => {
  assert.strictEqual(ID.extractPostFBID('https://www.facebook.com/groups/1/permalink/123/'), '123');
  assert.strictEqual(ID.extractPostFBID('https://www.facebook.com/x?story_fbid=456&id=9'), '456');
  assert.strictEqual(ID.extractPostFBID('https://www.facebook.com/photo/?fbid=789'), '', 'photo fbid must NOT be taken as post id');
  assert.strictEqual(ID.extractPostFBID('https://www.facebook.com/photo.php?set=gm.555&fbid=1'), '555', 'set=gm. is the parent post id');
  assert.strictEqual(ID.extractPostFBID('https://www.facebook.com/groups/1/posts/999/'), '999');
  assert.strictEqual(ID.extractPostFBID('https://www.facebook.com/x?fbid=42'), '42', 'non-photo ?fbid= is a post id');
  assert.strictEqual(ID.extractPostFBID(''), '');
});

test('extractGroupFBID: path form then idorvanity', () => {
  assert.strictEqual(ID.extractGroupFBID('https://www.facebook.com/groups/1312868109620530?x=1'), '1312868109620530');
  assert.strictEqual(ID.extractGroupFBID('https://www.facebook.com/photo?idorvanity=777'), '777');
  assert.strictEqual(ID.extractGroupFBID('https://www.facebook.com/feed'), '');
});

test('canonicalPostPermalink: group vs bare, empty post id', () => {
  assert.strictEqual(ID.canonicalPostPermalink('1', '2'), 'https://www.facebook.com/groups/1/permalink/2/');
  assert.strictEqual(ID.canonicalPostPermalink('', '2'), 'https://www.facebook.com/permalink.php?story_fbid=2');
  assert.strictEqual(ID.canonicalPostPermalink('1', ''), '');
});

test('looksLikePostURL: post shapes true, photo/empty false', () => {
  assert.strictEqual(ID.looksLikePostURL('https://www.facebook.com/groups/1/permalink/2/'), true);
  assert.strictEqual(ID.looksLikePostURL('https://www.facebook.com/x?story_fbid=2'), true);
  assert.strictEqual(ID.looksLikePostURL('https://www.facebook.com/photo/?fbid=2'), false);
  assert.strictEqual(ID.looksLikePostURL(''), false);
  assert.strictEqual(ID.looksLikePostURL('https://www.facebook.com/groups/1'), false);
});

test('hashKey: deterministic hex, differs by input', () => {
  assert.strictEqual(ID.hashKey('abc'), ID.hashKey('abc'));
  assert.notStrictEqual(ID.hashKey('abc'), ID.hashKey('abd'));
  assert.match(ID.hashKey('anything'), /^[0-9a-f]+$/);
});

test('stripPostQueryParams: drops comment/tracking, keeps path + real params', () => {
  assert.strictEqual(
    ID.stripPostQueryParams('https://www.facebook.com/groups/1/permalink/2/?comment_id=9&ref=x&__tn__=y', ORIGIN),
    'https://www.facebook.com/groups/1/permalink/2/',
  );
  // A benign non-tracking param is preserved.
  assert.strictEqual(
    ID.stripPostQueryParams('https://www.facebook.com/groups/1/permalink/2/?sorting_setting=CHRONOLOGICAL', ORIGIN),
    'https://www.facebook.com/groups/1/permalink/2/?sorting_setting=CHRONOLOGICAL',
  );
  assert.strictEqual(ID.stripPostQueryParams('', ORIGIN), '');
});
