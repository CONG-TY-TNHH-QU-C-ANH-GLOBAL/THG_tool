// Sprint 4 — Local Connector Pairing Reliability regression net.
// Pins the extension-only fix that stopped pairing hanging forever on
// "Verifying...". Covers (1-3) THGShared.fetchWithTimeout's AbortController
// deadline / no-retry contract, (4) normalizePairingCode symmetry, and (C5) the
// pairConnector fire-and-forget heartbeat so a stalled heartbeat can never block
// a completed pairing. shared.js / api.js are globalThis-IIFEs (NOT CommonJS) —
// they reference chrome/THGFacebookState/THGHeartbeat only inside function
// bodies, so we install global stubs then eval the file text. Fail-closed.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const __dirname = dirname(fileURLToPath(import.meta.url));

function loadShared() {
  delete globalThis.THGShared;
  const src = readFileSync(join(__dirname, 'shared.js'), 'utf8');
  (0, eval)(src); // installs globalThis.THGShared
  return globalThis.THGShared;
}

test('fetchWithTimeout resolves and passes an AbortSignal, no retry', async () => {
  const Shared = loadShared();
  const sentinel = { ok: true };
  let calls = 0;
  let seenSignal;
  globalThis.fetch = (url, options) => {
    calls += 1;
    seenSignal = options && options.signal;
    return Promise.resolve(sentinel);
  };
  const res = await Shared.fetchWithTimeout('https://x/', {}, 50);
  assert.equal(res, sentinel, 'must return the underlying fetch result');
  assert.ok(seenSignal instanceof AbortSignal, 'must pass an AbortSignal to fetch');
  assert.equal(calls, 1, 'success path must call fetch exactly once (no retry)');
});

test('fetchWithTimeout times out as TimeoutError and does not retry', async () => {
  const Shared = loadShared();
  let calls = 0;
  // Never resolves on its own, but honours the abort signal (real fetch semantics).
  globalThis.fetch = (url, options) => {
    calls += 1;
    return new Promise((_resolve, reject) => {
      options.signal.addEventListener('abort', () => {
        const e = new Error('aborted');
        e.name = 'AbortError';
        reject(e);
      });
    });
  };
  await assert.rejects(
    () => Shared.fetchWithTimeout('https://x/', {}, 20),
    (err) => err && err.name === 'TimeoutError',
    'abort must surface as a tagged TimeoutError',
  );
  assert.equal(calls, 1, 'timeout path must call fetch exactly once (consumed code must not be replayed)');
});

test('normalizePairingCode symmetry (regression guard)', () => {
  const Shared = loadShared();
  assert.equal(Shared.normalizePairingCode('3cek-7k8p'), '3CEK-7K8P');
  assert.equal(Shared.normalizePairingCode('abcd1234'), 'ABCD-1234');
  assert.equal(Shared.normalizePairingCode('AB CD-12 34'), 'ABCD-1234');
  // Non-8 cleaned input passes through uppercased with no hyphen inserted.
  assert.equal(Shared.normalizePairingCode('abc'), 'ABC');
});

// C5 — liveness/fire-and-forget regression guard (REQUIRED). pairConnector must
// RESOLVE once the device token is persisted, even if the post-pair heartbeat
// NEVER resolves. Before the fix it awaited the heartbeat → popup hung forever.
test('C5: pairConnector resolves without waiting on the heartbeat', async () => {
  const Shared = loadShared();

  const storageSetCalls = [];
  Shared.storageSet = (value) => { storageSetCalls.push(value); return Promise.resolve(); };
  Shared.fetchWithTimeout = () => Promise.resolve({
    ok: true,
    json: async () => ({ device_token: 'tok_x', connector: { id: 1, name: 'X' }, pairing_session_id: 7 }),
  });

  globalThis.THGFacebookState = { collectFacebookState: async () => ({}) };
  globalThis.chrome = {
    runtime: { getManifest: () => ({ version: '0.5.57' }) },
    storage: { local: { set: async () => {}, get: async () => ({}), remove: async () => {} } },
  };
  globalThis.navigator = globalThis.navigator || { platform: 'test' };

  let heartbeatRuns = 0;
  // Heartbeat never resolves — proves pairing does not block on it.
  globalThis.THGHeartbeat = { run: () => { heartbeatRuns += 1; return new Promise(() => {}); } };

  delete globalThis.THGApi;
  const apiSrc = readFileSync(join(__dirname, 'api.js'), 'utf8');
  (0, eval)(apiSrc); // installs globalThis.THGApi
  const Api = globalThis.THGApi;

  // Bound by the per-test timeout: if pairConnector awaited the heartbeat this hangs.
  const payload = await Api.pairConnector('https://sale.thgfulfill.com', '3CEK-7K8P');

  assert.equal(payload.device_token, 'tok_x', 'returns the pairing payload');
  assert.equal(heartbeatRuns, 1, 'heartbeat fire-and-forget must be kicked exactly once');
  const persisted = storageSetCalls.find((v) => v && v.deviceToken === 'tok_x');
  assert.ok(persisted, 'device token must be persisted before returning (storageSet called with deviceToken)');
});
