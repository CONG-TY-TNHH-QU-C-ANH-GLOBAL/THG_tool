const serverUrlInput = document.getElementById('serverUrl');
const pairingCodeInput = document.getElementById('pairingCode');
const errorMsg = document.getElementById('errorMsg');

const btnStartPair = document.getElementById('btn-start-pair');
const btnToggleAdv = document.getElementById('btn-toggle-advanced');
const advPanel = document.getElementById('adv-panel');

const btnVerifyPair = document.getElementById('btn-verify-pair');
const btnCancelPair = document.getElementById('btn-cancel-pair');

const btnSync = document.getElementById('btn-sync');
const btnForget = document.getElementById('btn-forget');

const steadyDevice = document.getElementById('steady-device');
const steadySession = document.getElementById('steady-session');

function setScreen(id) {
  document.querySelectorAll('.screen').forEach(el => el.classList.remove('active'));
  document.getElementById(`screen-${id}`).classList.add('active');
}

function sendMessage(message) {
  return chrome.runtime.sendMessage(message);
}

function normalizePairingCode(value) {
  const cleaned = String(value || '').toUpperCase().replace(/[^A-Z0-9]/g, '');
  return cleaned.length > 4 ? `${cleaned.slice(0, 4)}-${cleaned.slice(4, 8)}` : cleaned;
}

pairingCodeInput.addEventListener('input', (e) => {
  e.target.value = normalizePairingCode(e.target.value);
  e.target.classList.remove('error');
  errorMsg.style.display = 'none';
});

btnToggleAdv.addEventListener('click', () => {
  advPanel.classList.toggle('open');
});

btnStartPair.addEventListener('click', () => {
  setScreen('entering');
  pairingCodeInput.focus();
});

btnCancelPair.addEventListener('click', () => {
  setScreen('idle');
  pairingCodeInput.value = '';
  pairingCodeInput.classList.remove('error');
  errorMsg.style.display = 'none';
});

async function refreshStatus() {
  try {
    const res = await sendMessage({ type: 'status' });
    if (!res || !res.ok) {
      setScreen('idle');
      return;
    }
    const cfg = res.config || {};
    serverUrlInput.value = cfg.serverUrl || 'https://sale.thgfulfill.com';
    
    if (!cfg.deviceToken) {
      setScreen('idle');
      return;
    }

    const live = res.live || {};
    let fbInfo = 'Không tìm thấy FB';
    if (live.fbDisplayName) {
      fbInfo = live.fbDisplayName;
    } else if (live.fbUserId) {
      fbInfo = `FB ${live.fbUserId}`;
    }

    steadyDevice.textContent = cfg.connectorName || 'THG Chrome Extension';
    steadySession.textContent = fbInfo;
    steadySession.style.color = live.fbUserId ? 'var(--success)' : 'var(--text-muted)';

    setScreen('steady');
  } catch (err) {
    setScreen('idle');
  }
}

btnVerifyPair.addEventListener('click', async () => {
  const code = pairingCodeInput.value.replace(/[^A-Z0-9]/g, '');
  if (code.length !== 8) {
    pairingCodeInput.classList.add('error');
    errorMsg.textContent = 'Code must be 8 characters.';
    errorMsg.style.display = 'block';
    return;
  }

  btnVerifyPair.disabled = true;
  btnVerifyPair.textContent = 'Verifying...';
  
  try {
    const res = await sendMessage({
      type: 'pair',
      serverUrl: serverUrlInput.value,
      code: pairingCodeInput.value
    });
    
    if (!res || !res.ok) {
      throw new Error(res?.error || 'Pairing failed');
    }
    
    pairingCodeInput.value = '';
    await refreshStatus();
  } catch (err) {
    pairingCodeInput.classList.add('error');
    errorMsg.textContent = err?.message || String(err);
    errorMsg.style.display = 'block';
  } finally {
    btnVerifyPair.disabled = false;
    btnVerifyPair.textContent = 'Verify & Pair';
  }
});

btnSync.addEventListener('click', async () => {
  btnSync.disabled = true;
  btnSync.textContent = 'Syncing...';
  try {
    await refreshStatus();
  } finally {
    btnSync.disabled = false;
    btnSync.textContent = 'Sync Now';
  }
});

btnForget.addEventListener('click', async () => {
  if (!confirm('Are you sure you want to forget this workspace connection?')) return;
  
  btnForget.disabled = true;
  try {
    await sendMessage({ type: 'forget' });
    await refreshStatus();
  } finally {
    btnForget.disabled = false;
  }
});

// Init
refreshStatus();
