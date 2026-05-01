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

function normalizePairingCode(value) {
  const cleaned = String(value || '').toUpperCase().replace(/[^A-Z0-9]/g, '');
  return cleaned.length === 8 ? `${cleaned.slice(0, 4)}-${cleaned.slice(4)}` : cleaned;
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
    setStatus('Chưa kết nối. Tạo mã trong Browser dashboard, sau đó dán mã vào đây.');
    return;
  }
  const live = res.live || {};
  const fb = live.fbUserId ? `FB ${live.fbUserId}` : 'Chưa thấy tài khoản Facebook';
  setStatus(`Đã kết nối: ${cfg.connectorName || 'Chrome Extension'}\n${live.status || cfg.lastStatus || 'online'} - ${fb}`, 'ok');
}

pairButton.addEventListener('click', async () => {
  pairButton.disabled = true;
  setStatus('Đang kết nối...');
  try {
    pairingCodeInput.value = normalizePairingCode(pairingCodeInput.value);
    const res = await sendMessage({
      type: 'pair',
      serverUrl: serverUrlInput.value,
      code: pairingCodeInput.value
    });
    if (!res?.ok) throw new Error(res?.error || 'Kết nối thất bại');
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
    setStatus('Đã xóa token trên extension. Vào THG dashboard để disconnect thiết bị trên server.');
  } finally {
    forgetButton.disabled = false;
  }
});

refreshStatus().catch(err => setStatus(err?.message || String(err), 'error'));
