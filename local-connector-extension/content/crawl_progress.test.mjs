// PR-C1A boundary characterization — buildCrawlProgressMessage.
// Pins the thg_crawl_progress payload shape BEFORE any telemetry (PR-C1B) or
// checkpoint phase (PR-C2) fields are added, so a future change that alters the
// existing wire fields is caught here. Behavior-preserving refactor guard only.
//   Run: node content/crawl_progress.test.mjs   (or via `node --test`)
import assert from 'node:assert';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const C = require('./crawl.js');

const task = { task_id: 'autocrawl-27-495423', intent: 'facebook_crawl' };
const SRC = 'https://www.facebook.com/groups/1312868109620530';

// Exact payload today: type + crawler_version + task_id + intent + account_id
// + stage + fetched + max + source_url. Nothing else (no telemetry yet).
const msg = C.buildCrawlProgressMessage(task, 50, 'scraping', 5, 50, SRC);
assert.deepStrictEqual(msg, {
  type: 'thg_crawl_progress',
  crawler_version: 'scroll-target-v3-cursor',
  task_id: 'autocrawl-27-495423',
  intent: 'facebook_crawl',
  account_id: 50,
  stage: 'scraping',
  fetched: 5,
  max: 50,
  source_url: SRC,
}, 'progress payload shape must stay byte-identical to the pre-refactor literal');

// No stray/new keys leak in (guards against an accidental telemetry field).
assert.deepStrictEqual(Object.keys(msg).sort(), [
  'account_id', 'crawler_version', 'fetched', 'intent', 'max', 'source_url', 'stage', 'task_id', 'type',
], 'no unexpected keys in the C1A progress payload');

// Defaults mirror the old inline literal exactly: missing task/account → ''/0.
assert.deepStrictEqual(
  C.buildCrawlProgressMessage(null, 0, 'started', 0, 20, ''),
  {
    type: 'thg_crawl_progress',
    crawler_version: 'scroll-target-v3-cursor',
    task_id: '',
    intent: 'facebook_crawl',
    account_id: 0,
    stage: 'started',
    fetched: 0,
    max: 20,
    source_url: '',
  },
  'nil task / zero account fall back to the same defaults as the old literal',
);

console.log('crawl_progress.test.mjs: ok');
