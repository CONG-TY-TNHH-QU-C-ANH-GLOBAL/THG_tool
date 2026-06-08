var THGHeartbeat = globalThis.THGHeartbeat || (() => {
  // Two INDEPENDENT guards. The whole bug class "connector goes OFFLINE while a
  // comment runs" came from coupling liveness with heavy work in one serialized
  // cycle: a 30–60s+ (or hung) outbound op blocked runOnce, and the run()
  // guard made the next 30s alarm a no-op, so NO heartbeat was sent for >90s →
  // backend marked the connector offline (agentOnline: last_seen <= 90s) AND the
  // long op risked the MV3 service worker being recycled mid-flight.
  //
  // Fix: the alarm always runs the FAST heartbeat (one ping + status), which
  // returns in ~1–2s and keeps the connector ONLINE every 30s. Heavy work
  // (crawl + outbox) is kicked off separately with its OWN guard and is NOT
  // awaited by the heartbeat, so a long comment can never starve liveness.
  let hbRunning = null;
  let workRunning = null;

  function schedule() {
    chrome.alarms.create(THGShared.HEARTBEAT_ALARM, { periodInMinutes: 0.5 });
  }

  // kickWork runs the heavy, potentially-slow processors (crawl commands, then
  // outbox). Guarded so overlapping alarms don't start a second cycle while one
  // is mid-flight. Fire-and-forget from the heartbeat path; errors land in
  // storage. The in-flight chrome.tabs.sendMessage inside outbox keeps the SW
  // alive, and the 30s heartbeat alarm re-wakes it — together they bound the
  // window the SW can be recycled in.
  function kickWork(target, state) {
    if (workRunning) return workRunning;
    workRunning = (async () => {
      await THGCommands.process(target, state).catch(err => THGShared.storageSet({ lastError: err?.message || String(err) }));
      await THGOutbox.process(target, state).catch(err => THGShared.storageSet({ lastError: err?.message || String(err) }));
    })().finally(() => { workRunning = null; });
    return workRunning;
  }

  // heartbeatOnce is the FAST liveness path. sendHeartbeat goes out FIRST so the
  // backend's last_seen is refreshed before any slower status/screenshot calls.
  // It does NOT await kickWork — that is the whole point of the decoupling.
  async function heartbeatOnce() {
    const cfg = await THGApi.getConfig();
    if (!cfg.deviceToken) return { paired: false };
    const state = await THGFacebookState.collectFacebookState();
    await THGStream.sendHeartbeat(state);
    await THGShared.storageSet({
      lastStatus: state.streamStatus,
      lastError: '',
      lastSeenAt: new Date().toISOString()
    });
    const targets = await THGStream.fetchTargets().catch(() => []);
    const target = THGFacebookState.chooseTarget(targets, state.fbUserId);
    if (target) {
      await THGStream.sendChromeStatus(target, state).catch(() => {});
      // Stream removed (PR-F): the dashboard no longer needs a live screenshot —
      // UID + "Facebook connected" status is the source of truth. We keep
      // heartbeat + chrome-status (liveness + identity), and drop the
      // captureVisibleTab upload entirely.
      // Kick heavy work WITHOUT awaiting — liveness must not depend on it.
      kickWork(target, state);
    }
    return { paired: true, status: state.streamStatus, currentUrl: state.currentUrl, fbUserId: state.fbUserId };
  }

  async function run() {
    if (hbRunning) return hbRunning;
    hbRunning = heartbeatOnce().finally(() => {
      hbRunning = null;
    });
    return hbRunning;
  }

  return { run, schedule };
})();
globalThis.THGHeartbeat = THGHeartbeat;
