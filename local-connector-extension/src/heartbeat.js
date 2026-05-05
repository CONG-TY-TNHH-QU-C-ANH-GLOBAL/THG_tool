var THGHeartbeat = globalThis.THGHeartbeat || (() => {
  function schedule() {
    chrome.alarms.create(THGShared.HEARTBEAT_ALARM, { periodInMinutes: 0.5 });
  }

  async function run() {
    const cfg = await THGApi.getConfig();
    if (!cfg.deviceToken) return { paired: false };
    let state = await THGFacebookState.collectFacebookState();
    await THGStream.sendHeartbeat(state);
    const targets = await THGStream.fetchTargets().catch(() => []);
    let target = THGFacebookState.chooseTarget(targets, state.fbUserId);
    if (target) {
      if (THGShared.AUTO_FOCUS_FACEBOOK_TAB && (!state.tab || !state.tab.active)) {
        state = await THGFacebookState.ensureFacebookTabVisible();
        target = THGFacebookState.chooseTarget(targets, state.fbUserId) || target;
      }
      await THGStream.sendChromeStatus(target, state).catch(() => {});
      await THGStream.sendScreenshot(target, state).catch(() => {});
      await THGCommands.process(target, state).catch(err => THGShared.storageSet({ lastError: err?.message || String(err) }));
      await THGOutbox.process(target, state).catch(err => THGShared.storageSet({ lastError: err?.message || String(err) }));
    }
    await THGShared.storageSet({
      lastStatus: state.streamStatus,
      lastError: '',
      lastSeenAt: new Date().toISOString()
    });
    return { paired: true, status: state.streamStatus, currentUrl: state.currentUrl, fbUserId: state.fbUserId };
  }

  return { run, schedule };
})();
globalThis.THGHeartbeat = THGHeartbeat;
