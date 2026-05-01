const serverUrlInput = document.getElementById('serverUrl');
const pairingCodeInput = document.getElementById('pairingCode');
const pairButton = document.getElementById('pairButton');
const syncButton = document.getElementById('syncButton');
const forgetButton = document.getElementById('forgetButton');
const statusBox = document.getElementById('status');

function sendMessage(message) {
  return chrome.runtime.sendMessage(message);
}

function setStatus(text, tone = '') {
  statusBox.textContent = text;
  statusBox.className = `status ${tone}`.trim();
}

async function refreshStatus() {
  const res = await sendMessage({ type: 'status' });
  if (!res?.ok) {
    setStatus(res?.error || 'Not paired yet', 'error');
    return;
  }
  const cfg = res.config || {};
  serverUrlInput.value = cfg.serverUrl || 'https://sale.thgfulfill.com';
  if (!cfg.deviceToken) {
    setStatus('Not paired. Create a pairing code in the Browser dashboard, then paste it here.');
    return;
  }
  const live = res.live || {};
  const fb = live.fbUserId ? `FB ${live.fbUserId}` : 'No Facebook user yet';
  setStatus(`Paired: ${cfg.connectorName || 'Chrome Extension'}\n${live.status || cfg.lastStatus || 'online'} - ${fb}`, 'ok');
}

pairButton.addEventListener('click', async () => {
  pairButton.disabled = true;
  setStatus('Pairing...');
  try {
    const res = await sendMessage({
      type: 'pair',
      serverUrl: serverUrlInput.value,
      code: pairingCodeInput.value
    });
    if (!res?.ok) throw new Error(res?.error || 'Pairing failed');
    pairingCodeInput.value = '';
    await refreshStatus();
  } catch (err) {
    setStatus(err?.message || String(err), 'error');
  } finally {
    pairButton.disabled = false;
  }
});

syncButton.addEventListener('click', async () => {
  syncButton.disabled = true;
  try {
    await refreshStatus();
  } finally {
    syncButton.disabled = false;
  }
});

forgetButton.addEventListener('click', async () => {
  forgetButton.disabled = true;
  try {
    await sendMessage({ type: 'forget' });
    setStatus('Local extension token removed. Disconnect the device in THG dashboard if you want to revoke it on the server.');
  } finally {
    forgetButton.disabled = false;
  }
});

refreshStatus().catch(err => setStatus(err?.message || String(err), 'error'));
