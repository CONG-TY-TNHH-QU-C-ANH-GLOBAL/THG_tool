// ===== THG Agentic Scraper — Dashboard JS v2 =====

const API = '';
let currentPage = 'dashboard';
let refreshInterval = null;
let activeNicheFilter = '';
let accessToken = localStorage.getItem('thg_token') || '';
let lastApiError = null; // last non-OK response body, read by error handlers

// ===== Auth =====

function showLogin() {
    document.getElementById('loginPage').style.display = 'flex';
    document.getElementById('loginEmail').focus();
}

function hideLogin() {
    document.getElementById('loginPage').style.display = 'none';
}

function showRegisterForm(e) {
    if (e) e.preventDefault();
    document.getElementById('loginFormWrap').style.display = 'none';
    document.getElementById('registerFormWrap').style.display = '';
    document.getElementById('regOrgName').focus();
}

function showLoginForm(e) {
    if (e) e.preventDefault();
    document.getElementById('registerFormWrap').style.display = 'none';
    document.getElementById('loginFormWrap').style.display = '';
    document.getElementById('loginEmail').focus();
}

async function doRegister(e) {
    e.preventDefault();
    const btn = document.getElementById('registerBtn');
    const errEl = document.getElementById('registerError');
    errEl.style.display = 'none';
    btn.disabled = true;
    btn.textContent = 'Đang tạo...';
    try {
        const res = await fetch('/api/register', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                org_name: document.getElementById('regOrgName').value.trim(),
                admin_name: document.getElementById('regAdminName').value.trim(),
                admin_email: document.getElementById('regEmail').value.trim(),
                admin_password: document.getElementById('regPassword').value,
            })
        });
        const data = await res.json().catch(() => ({}));
        if (!res.ok) {
            errEl.textContent = data.error || 'Đăng ký thất bại';
            errEl.style.display = 'block';
            return;
        }
        accessToken = data.access_token;
        localStorage.setItem('thg_token', accessToken);
        if (data.refresh_token) localStorage.setItem('thg_refresh', data.refresh_token);
        if (data.user) {
            localStorage.setItem('thg_user', JSON.stringify(data.user));
            updateSidebarUser(data.user);
        }
        hideLogin();
        showRegisterForm_reset();
        loadDashboard();
        loadNicheTabs();
        if (refreshInterval) clearInterval(refreshInterval);
        refreshInterval = setInterval(() => { if (!document.hidden) refreshData(); }, 15000);
        showToast(`Chào mừng! Tổ chức "${data.org_name}" đã được tạo thành công 🎉`, 'success');
    } catch {
        errEl.textContent = 'Lỗi kết nối server';
        errEl.style.display = 'block';
    } finally {
        btn.disabled = false;
        btn.textContent = 'Tạo tổ chức';
    }
}

function showRegisterForm_reset() {
    document.getElementById('regOrgName').value = '';
    document.getElementById('regAdminName').value = '';
    document.getElementById('regEmail').value = '';
    document.getElementById('regPassword').value = '';
    document.getElementById('registerError').style.display = 'none';
    showLoginForm();
}

async function doLogin(e) {
    e.preventDefault();
    const email = document.getElementById('loginEmail').value;
    const password = document.getElementById('loginPassword').value;
    const btn = document.getElementById('loginBtn');
    const errEl = document.getElementById('loginError');

    btn.disabled = true;
    btn.textContent = 'Đang đăng nhập...';
    errEl.style.display = 'none';

    try {
        const res = await fetch('/api/auth/login', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ email, password })
        });
        const data = await res.json().catch(() => ({}));
        if (!res.ok) {
            errEl.textContent = data.error || 'Email hoặc mật khẩu không đúng';
            errEl.style.display = 'block';
            return;
        }
        accessToken = data.access_token;
        localStorage.setItem('thg_token', accessToken);
        if (data.refresh_token) localStorage.setItem('thg_refresh', data.refresh_token);
        if (data.user) {
            localStorage.setItem('thg_user', JSON.stringify(data.user));
            updateSidebarUser(data.user);
        }
        hideLogin();
        loadDashboard();
        loadNicheTabs();
        if (refreshInterval) clearInterval(refreshInterval);
        refreshInterval = setInterval(() => { if (!document.hidden) refreshData(); }, 15000);
    } catch {
        errEl.textContent = 'Lỗi kết nối server';
        errEl.style.display = 'block';
    } finally {
        btn.disabled = false;
        btn.textContent = 'Đăng nhập';
    }
}

function doLogout() {
    if (refreshInterval) { clearInterval(refreshInterval); refreshInterval = null; }
    const token = accessToken;
    const refreshToken = localStorage.getItem('thg_refresh');
    accessToken = '';
    localStorage.removeItem('thg_token');
    localStorage.removeItem('thg_refresh');
    localStorage.removeItem('thg_user');
    showLogin();
    const headers = {};
    if (token) headers['Authorization'] = `Bearer ${token}`;
    if (refreshToken) headers['X-Refresh-Token'] = refreshToken;
    fetch('/api/auth/logout', { method: 'POST', headers }).catch(() => { });
}

function updateSidebarUser(user) {
    if (!user) return;
    const displayName = user.name || user.email || '';
    const avatarEl = document.getElementById('headerAvatar');
    const nameEl = document.getElementById('headerUserName');
    const menuNameEl = document.getElementById('menuUserName');
    if (avatarEl) avatarEl.textContent = displayName ? displayName[0].toUpperCase() : '?';
    if (nameEl) nameEl.textContent = displayName;
    if (menuNameEl) menuNameEl.textContent = displayName;
    const roleLabel = user.role === 'superadmin' ? '⚡ Super Admin' : (user.role === 'admin' ? '🔑 Admin' : '📊 Sales');
    const orgEl = document.getElementById('headerOrgName');
    if (orgEl) orgEl.textContent = roleLabel;

    // Show MANAGE section; auto-expand for admin if no stored preference
    const isAdmin = user.role === 'admin' || user.role === 'superadmin';
    const items = document.getElementById('manageSectionItems');
    const chevron = document.getElementById('manageChevron');
    if (items) {
        const stored = localStorage.getItem('thg_manage_expanded');
        const expanded = stored !== null ? stored === '1' : isAdmin;
        items.style.display = expanded ? '' : 'none';
        if (chevron) chevron.textContent = expanded ? '▾' : '▸';
    }
}

function toggleManageSection() {
    const items = document.getElementById('manageSectionItems');
    const chevron = document.getElementById('manageChevron');
    if (!items) return;
    const expanded = items.style.display !== 'none';
    items.style.display = expanded ? 'none' : '';
    if (chevron) chevron.textContent = expanded ? '▸' : '▾';
    localStorage.setItem('thg_manage_expanded', expanded ? '0' : '1');
}

function toggleUserMenu(e) {
    if (e) e.stopPropagation();
    const d = document.getElementById('userMenuDropdown');
    if (!d) return;
    if (d.style.display === 'block') {
        d.style.display = 'none';
        return;
    }
    // Use fixed positioning so the dropdown escapes overflow:hidden on .main-content
    const wrap = document.getElementById('userMenuWrap');
    const rect = wrap.getBoundingClientRect();
    d.style.position = 'fixed';
    d.style.top = (rect.bottom + 6) + 'px';
    d.style.right = (window.innerWidth - rect.right) + 'px';
    d.style.left = '';
    d.style.display = 'block';
}

function closeUserMenu() {
    const d = document.getElementById('userMenuDropdown');
    if (d) d.style.display = 'none';
}

document.addEventListener('click', function(e) {
    const wrap = document.getElementById('userMenuWrap');
    if (wrap && !wrap.contains(e.target)) closeUserMenu();
});

// ===== Browser / Workspace Page (noVNC + Docker) =====

let browserSelectedAccountID = null;
let browserRFB = null;          // noVNC RFB instance
let browserReconnectTimer = null;
let rfbModule = null;           // cached noVNC RFB class (loaded once from CDN)

async function loadBrowserPage() {
    await loadBrowserWorkspaces();
}

// Dynamically import noVNC RFB from jsDelivr CDN.
// The library runs entirely in the user's browser — no server-side files needed.
async function loadRFB() {
    if (rfbModule) return rfbModule;
    try {
        const mod = await import('https://cdn.jsdelivr.net/npm/@novnc/novnc@1.4.0/core/rfb.js');
        rfbModule = mod.default;
        return rfbModule;
    } catch (e) {
        console.error('[noVNC] Failed to load from CDN:', e);
        return null;
    }
}

async function loadBrowserWorkspaces() {
    const res = await fetchAPI('/api/browser/workspaces');
    if (!res) return;

    const list = document.getElementById('workspaceAccountList');
    if (!list) return;

    const workspaces = res.workspaces || [];
    if (workspaces.length === 0) {
        list.innerHTML = '<div style="padding:12px 4px;text-align:center;color:var(--text-muted);font-size:12px">Nhấn <strong style="color:#8b5cf6">+ Thêm</strong> để thêm tài khoản Facebook.</div>';
        browserShowAddForm();
        return;
    }

    list.innerHTML = workspaces.map(w => {
        const loginBadge = w.logged_in
            ? '<span style="font-size:10px;color:#4ade80">✓ Logged In</span>'
            : '<span style="font-size:10px;color:var(--text-muted)">○ Chưa đăng nhập</span>';
        return `
        <div class="workspace-account-row ${browserSelectedAccountID === w.account_id ? 'selected' : ''}"
             onclick="browserSelectAccount(${w.account_id}, '${esc(w.account_name)}', ${w.running}, ${w.vnc_port || 0}, ${w.logged_in || false})">
            <div>
                <div style="font-size:13px;font-weight:500">${esc(w.account_name)}</div>
                <div style="display:flex;gap:8px;align-items:center;margin-top:3px">
                    ${loginBadge}
                </div>
            </div>
            <span class="workspace-status-pill ${w.running ? 'running' : 'offline'}">
                ${w.running ? '● RUNNING' : '○ offline'}
            </span>
        </div>`;
    }).join('');

    if (browserSelectedAccountID !== null) {
        const selected = workspaces.find(w => w.account_id === browserSelectedAccountID);
        if (selected) updateBrowserControls(selected.running, selected.vnc_port, selected.logged_in);
    }
}

function browserSelectAccount(accountID, accountName, running, vncPort, loggedIn) {
    if (browserSelectedAccountID !== accountID && browserRFB) {
        try { browserRFB.disconnect(); } catch {}
        browserRFB = null;
        showBrowserPlaceholder('Chọn tài khoản bên trái → nhấn START');
    }
    browserSelectedAccountID = accountID;

    document.querySelectorAll('.workspace-account-row').forEach(el => {
        const m = el.getAttribute('onclick').match(/browserSelectAccount\((\d+)/);
        el.classList.toggle('selected', m && parseInt(m[1]) === accountID);
    });

    const loginStatus = loggedIn ? '✓ Đã đăng nhập' : '○ Chưa đăng nhập';
    document.getElementById('browserUrlLabel').textContent = `${accountName} — ${loginStatus}`;
    updateBrowserControls(running, vncPort, loggedIn);

    if (running && vncPort > 0) connectBrowserViewVNC(accountID);
    else if (!running) showBrowserPlaceholder('Nhấn START để mở Chrome — Facebook sẽ tự động đăng nhập nếu đã có session.');
}

function updateBrowserControls(running, vncPort, loggedIn) {
    const startBtn     = document.getElementById('browserStartBtn');
    const stopBtn      = document.getElementById('browserStopBtn');
    const markLoginBtn = document.getElementById('browserMarkLoginBtn');
    const portEl       = document.getElementById('browserPortStatus');
    const portLabel    = document.getElementById('browserCdpPort');

    if (running) {
        startBtn.textContent = '● RUNNING';
        startBtn.className = 'btn btn-success btn-sm active';
        startBtn.disabled = true;
        stopBtn.disabled = false;
        portEl.style.display = 'flex';
        if (portLabel) portLabel.textContent = vncPort || '–';
        // Show "Mark as Logged In" only when running and not yet marked
        if (markLoginBtn) {
            markLoginBtn.style.display = loggedIn ? 'none' : '';
            markLoginBtn.disabled = false;
        }
    } else {
        startBtn.textContent = '▶ START';
        startBtn.className = 'btn btn-success btn-sm';
        startBtn.disabled = !browserSelectedAccountID;
        stopBtn.disabled = true;
        portEl.style.display = 'none';
        if (markLoginBtn) { markLoginBtn.style.display = 'none'; markLoginBtn.disabled = true; }
    }
}

function browserShowAddForm() {
    const f = document.getElementById('browserAddForm');
    if (f) f.style.display = '';
    const inp = document.getElementById('browserNewAccountName');
    if (inp) { inp.value = ''; inp.focus(); }
}
function browserHideAddForm() {
    const f = document.getElementById('browserAddForm');
    if (f) f.style.display = 'none';
}

async function browserAddAccount() {
    const inp = document.getElementById('browserNewAccountName');
    const name = inp ? inp.value.trim() : '';
    if (!name) { showToast('Nhập tên tài khoản', 'error'); inp && inp.focus(); return; }

    inp.disabled = true;
    const res = await fetchAPI('/api/accounts', 'POST', { name, platform: 'facebook' });
    if (inp) inp.disabled = false;
    if (!res || !res.account_id) { showToast('Tạo tài khoản thất bại', 'error'); return; }

    browserHideAddForm();
    browserSelectedAccountID = res.account_id;
    showToast(`Tạo "${name}" thành công — đang khởi động Chrome...`, 'success');
    await loadBrowserWorkspaces();

    // Auto-start the browser for this new account
    showBrowserPlaceholder('Đang khởi động Docker container — Xvfb + Chrome + x11vnc...');
    document.getElementById('browserStartBtn').textContent = '⏳ Đang khởi động...';
    document.getElementById('browserStartBtn').disabled = true;
    const startRes = await fetchAPI(`/api/browser/workspaces/${res.account_id}/start`, 'POST');
    if (!startRes) {
        document.getElementById('browserStartBtn').textContent = '▶ START';
        document.getElementById('browserStartBtn').disabled = false;
        showBrowserPlaceholder('Khởi động thất bại — kiểm tra Docker: docker build -t thg-browser ./docker/');
        return;
    }
    if (startRes.status === 'running') {
        showToast('Chrome đã sẵn sàng! Đang kết nối...', 'success');
        updateBrowserControls(true, startRes.vnc_port, false);
        await loadBrowserWorkspaces();
        showBrowserPlaceholder('Đang kết nối noVNC...');
        setTimeout(() => connectBrowserViewVNC(res.account_id), 500);
    }
}

async function browserStartSelected() {
    if (!browserSelectedAccountID) {
        // No account selected — show add form instead of error toast
        browserShowAddForm();
        return;
    }
    const btn = document.getElementById('browserStartBtn');
    btn.textContent = '⏳ Đang khởi động...';
    btn.disabled = true;
    showBrowserPlaceholder('Đang khởi động Docker container — Xvfb + Chrome + x11vnc...');

    // Server blocks until VNC is ready inside the container (up to 45s)
    const res = await fetchAPI(`/api/browser/workspaces/${browserSelectedAccountID}/start`, 'POST');
    if (!res) {
        btn.textContent = '▶ START';
        btn.disabled = false;
        const errMsg = (lastApiError && lastApiError.error) ||
            'Không khởi động được — đảm bảo Docker đã cài và image đã build: docker build -t thg-browser ./docker/';
        showBrowserPlaceholder(errMsg);
        showToast(errMsg, 'error');
        return;
    }

    if (res.status === 'running') {
        showToast('Browser container sẵn sàng!', 'success');
        updateBrowserControls(true, res.vnc_port);
        await loadBrowserWorkspaces();
        showBrowserPlaceholder('Đang kết nối noVNC...');
        setTimeout(() => connectBrowserViewVNC(browserSelectedAccountID), 500);
    } else {
        btn.textContent = '▶ START';
        btn.disabled = false;
        showToast(res.error || 'Lỗi khởi động container', 'error');
        showBrowserPlaceholder(res.error || 'Khởi động thất bại — xem Logs');
    }
}

async function browserStopSelected() {
    if (!browserSelectedAccountID) return;
    if (!confirm('Dừng browser? Session Facebook vẫn được lưu trong Docker volume.')) return;
    if (browserRFB) {
        try { browserRFB.disconnect(); } catch {}
        browserRFB = null;
    }
    await fetchAPI(`/api/browser/workspaces/${browserSelectedAccountID}/stop`, 'POST');
    updateBrowserControls(false, 0);
    showBrowserPlaceholder('Container đã dừng. Nhấn START để khởi động lại.');
    await loadBrowserWorkspaces();
}

// browserMarkLoggedIn marks the selected account as logged into Facebook.
// User clicks this after they see the Facebook feed in the live browser view.
async function browserMarkLoggedIn() {
    if (!browserSelectedAccountID) return;
    const res = await fetchAPI(`/api/browser/workspaces/${browserSelectedAccountID}/set-logged-in`, 'POST', { logged_in: true });
    if (res && res.ok) {
        showToast('✓ Đã đánh dấu đăng nhập Facebook thành công!', 'success');
        const btn = document.getElementById('browserMarkLoginBtn');
        if (btn) btn.style.display = 'none';
        document.getElementById('browserUrlLabel').textContent =
            document.getElementById('browserUrlLabel').textContent.replace('○ Chưa đăng nhập', '✓ Đã đăng nhập');
        await loadBrowserWorkspaces();
    }
}

// connectBrowserViewVNC connects the noVNC client to /ws/vnc/:accountID
async function connectBrowserViewVNC(accountID) {
    if (browserReconnectTimer) { clearTimeout(browserReconnectTimer); browserReconnectTimer = null; }
    // Disconnect previous RFB session
    if (browserRFB) {
        try { browserRFB.disconnect(); } catch {}
        browserRFB = null;
    }
    if (accountID !== browserSelectedAccountID) return;

    const RFB = await loadRFB();
    if (!RFB) {
        showBrowserPlaceholder('Không tải được noVNC — kiểm tra kết nối internet của bạn');
        return;
    }

    const screen = document.getElementById('browserVNCScreen');
    if (!screen) return;

    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    const url = `${proto}://${location.host}/ws/vnc/${accountID}?token=${encodeURIComponent(accessToken)}`;

    try {
        const rfb = new RFB(screen, url);
        browserRFB = rfb;

        rfb.scaleViewport = true;   // scale remote desktop to fit the div
        rfb.resizeSession = false;  // don't resize the remote session
        rfb.viewOnly = false;       // allow mouse + keyboard input

        rfb.addEventListener('connect', () => {
            showBrowserPlaceholder(null); // hide overlay — live view shows through
        });

        rfb.addEventListener('disconnect', () => {
            if (browserRFB !== rfb) return;
            browserRFB = null;
            if (accountID === browserSelectedAccountID) {
                const stillRunning = document.querySelector('.workspace-status-pill.running');
                if (stillRunning) {
                    showBrowserPlaceholder('Đang kết nối lại...');
                    browserReconnectTimer = setTimeout(() => connectBrowserViewVNC(accountID), 3000);
                } else {
                    showBrowserPlaceholder('Kết nối bị ngắt.');
                }
            }
        });

        rfb.addEventListener('credentialsrequired', () => {
            // x11vnc is started with -nopw, but send empty if asked
            rfb.sendCredentials({ password: '' });
        });

    } catch (e) {
        showBrowserPlaceholder('Lỗi khởi tạo noVNC: ' + e.message);
    }
}

function showBrowserPlaceholder(text) {
    const ph = document.getElementById('browserPlaceholder');
    if (!ph) return;
    if (text === null) {
        ph.style.display = 'none';
    } else {
        ph.style.display = 'flex';
        ph.querySelector('.browser-placeholder-text').textContent =
            text || 'Chọn tài khoản bên trái → nhấn START';
    }
}

// ===== Agent Tokens (admin) =====

async function loadAgentTokens() {
    const user = JSON.parse(localStorage.getItem('thg_user') || '{}');
    const section = document.getElementById('agentTokensSection');
    if (!section) return;
    if (user.role !== 'admin' && user.role !== 'superadmin') { section.style.display = 'none'; return; }
    section.style.display = '';

    const res = await fetchAPI('/api/admin/agent-tokens');
    if (!res) return;

    renderTable('agentTokensTable', res.tokens || [], tok => `
        <td style="font-family:monospace;font-size:12px">${tok.id}</td>
        <td>${esc(tok.name)}</td>
        <td style="font-size:12px">${esc(tok.hostname || '–')} <span style="color:var(--text-muted)">${esc(tok.os || '')}</span></td>
        <td style="font-size:12px">${tok.last_seen ? new Date(tok.last_seen).toLocaleString('vi-VN') : '<span style="color:var(--text-muted)">Chưa kết nối</span>'}</td>
        <td><span class="badge ${tok.active ? 'badge-done' : 'badge-failed'}">${tok.active ? '● Active' : '✕ Revoked'}</span></td>
        <td><button class="btn btn-sm btn-danger" onclick="revokeAgentToken(${tok.id})" ${!tok.active ? 'disabled' : ''}>Thu hồi</button></td>
    `);
}

async function createAgentToken() {
    const name = prompt('Tên token (ví dụ: "Laptop Nguyễn Văn A"):');
    if (!name || !name.trim()) return;
    const res = await fetchAPI('/api/admin/agent-tokens', 'POST', { name: name.trim() });
    if (!res) return;
    await new Promise(r => setTimeout(r, 50));
    alert(`🔑 Token cho "${res.name}":\n\n${res.token}\n\n⚠️ Copy ngay — chỉ hiển thị MỘT LẦN!`);
    showToast('Token đã tạo!');
    loadAgentTokens();
}

async function revokeAgentToken(id) {
    if (!confirm('Thu hồi token này? Agent sẽ không thể kết nối nữa.')) return;
    await fetchAPI(`/api/admin/agent-tokens/${id}`, 'DELETE');
    showToast('Token đã thu hồi');
    loadAgentTokens();
}

// ===== Settings Page =====

async function loadSettingsPage() {
    const user = await fetchAPI('/api/auth/me');
    if (!user) return;
    localStorage.setItem('thg_user', JSON.stringify(user));
    updateSidebarUser(user);
    document.getElementById('profileName').value = user.name || '';
    document.getElementById('profileEmail').value = user.email || '';
    document.getElementById('profileRole').value = user.role === 'superadmin' ? 'Super Admin' : (user.role === 'admin' ? 'Admin' : 'Sales');

    const isAdmin = user.role === 'admin' || user.role === 'superadmin';
    document.getElementById('agentTokensSection').style.display = isAdmin ? '' : 'none';
    document.getElementById('userMgmtSection').style.display = isAdmin ? '' : 'none';
    document.getElementById('orgInfoSection').style.display = '';

    if (isAdmin) {
        loadUsersTable();
        loadAgentTokens();
        loadOrgInfo(user.role);
    }
}

async function loadOrgInfo(role) {
    const res = await fetchAPI('/api/org');
    if (!res || !res.org) return;
    const org = res.org;
    const planLabels = { free: 'Free', pro: 'Pro', enterprise: 'Enterprise' };
    document.getElementById('orgName').value = org.name || '';
    document.getElementById('orgDomain').value = org.domain || '';
    document.getElementById('orgAccountCount').textContent = res.account_count ?? '—';
    document.getElementById('orgAccountLimit').textContent = org.max_accounts === 0 ? '∞' : (org.max_accounts ?? '—');
    document.getElementById('orgPlanName').textContent = planLabels[org.plan_tier] || org.plan_tier;
    document.getElementById('orgPlanBadge').textContent = (planLabels[org.plan_tier] || org.plan_tier).toUpperCase();
    // Sidebar org badge + header org context
    const badge = document.getElementById('sidebarOrgBadge');
    if (badge) { badge.textContent = org.name; badge.style.display = ''; }
    const headerOrg = document.getElementById('headerOrgName');
    if (headerOrg) headerOrg.textContent = org.name;
    if (role === 'admin') {
        document.getElementById('orgName').disabled = false;
        document.getElementById('orgDomain').disabled = false;
        document.getElementById('orgSaveBtn').style.display = '';
    } else {
        document.getElementById('orgName').disabled = true;
        document.getElementById('orgDomain').disabled = true;
    }
}

async function saveOrgSettings() {
    const name = document.getElementById('orgName').value.trim();
    const domain = document.getElementById('orgDomain').value.trim();
    if (!name) return showToast('Tên tổ chức không được để trống', 'error');
    const res = await fetchAPI('/api/org', 'PUT', { name, domain });
    if (res) showToast('Đã lưu cài đặt tổ chức', 'success');
}

async function loadOrgsTable() {
    const res = await fetchAPI('/api/superadmin/orgs');
    if (!res) return;
    const planColors = { free: '#6ee7b7', pro: '#60a5fa', enterprise: '#f59e0b' };
    renderTable('orgsTable', res.organizations || [], org => {
        const planColor = planColors[org.plan_tier] || '#94a3b8';
        return `
            <td><strong>${esc(org.name)}</strong></td>
            <td style="color:var(--text-muted)">${esc(org.domain || '—')}</td>
            <td><span style="padding:2px 8px;border-radius:4px;font-size:12px;background:rgba(0,0,0,0.2);color:${planColor};font-weight:600">${org.plan_tier?.toUpperCase()}</span></td>
            <td style="text-align:center">${org.max_accounts === 0 ? '∞' : org.max_accounts}</td>
            <td><span style="color:${org.active ? '#6ee7b7' : '#f87171'}">${org.active ? 'Hoạt động' : 'Tạm khóa'}</span></td>
            <td>${timeAgo(org.created_at)}</td>
            <td><button class="btn btn-sm btn-ghost" onclick="showEditOrgModal(${org.id},'${esc(org.name)}','${esc(org.domain)}','${org.plan_tier}',${org.max_accounts},${org.active})">✏️</button></td>
        `;
    });
}

function showEditOrgModal(id, name, domain, plan, maxAccounts, active) {
    const plans = ['free', 'pro', 'enterprise'];
    const planOpts = plans.map(p => `<option value="${p}"${p===plan?' selected':''}>${p.charAt(0).toUpperCase()+p.slice(1)}</option>`).join('');
    const html = `
        <div class="modal-overlay" id="editOrgModal" style="display:flex">
            <div class="modal glass" style="max-width:400px;width:100%">
                <div class="modal-header"><h3>✏️ Chỉnh sửa tổ chức</h3></div>
                <div style="padding:20px;display:grid;gap:12px">
                    <div class="form-group"><label>Tên</label><input id="eoName" value="${esc(name)}" style="width:100%"></div>
                    <div class="form-group"><label>Domain</label><input id="eoDomain" value="${esc(domain)}" style="width:100%"></div>
                    <div class="form-group"><label>Gói dịch vụ</label><select id="eoPlan" style="width:100%">${planOpts}</select></div>
                    <div class="form-group"><label>Max FB accounts (0 = không giới hạn)</label><input id="eoMax" type="number" value="${maxAccounts}" min="0" style="width:100%"></div>
                    <div class="form-group" style="display:flex;align-items:center;gap:8px">
                        <input type="checkbox" id="eoActive"${active?' checked':''}> <label for="eoActive">Hoạt động</label>
                    </div>
                    <div style="display:flex;gap:8px;justify-content:flex-end">
                        <button class="btn btn-ghost btn-sm" onclick="document.getElementById('editOrgModal').remove()">Hủy</button>
                        <button class="btn btn-primary btn-sm" onclick="submitEditOrg(${id})">Lưu</button>
                    </div>
                </div>
            </div>
        </div>`;
    document.body.insertAdjacentHTML('beforeend', html);
}

async function submitEditOrg(id) {
    const active = document.getElementById('eoActive').checked;
    const res = await fetchAPI(`/api/superadmin/orgs/${id}`, 'PUT', {
        name: document.getElementById('eoName').value.trim(),
        domain: document.getElementById('eoDomain').value.trim(),
        plan_tier: document.getElementById('eoPlan').value,
        max_accounts: parseInt(document.getElementById('eoMax').value) || 0,
        active,
    });
    if (res) {
        document.getElementById('editOrgModal')?.remove();
        loadOrgsTable();
        showToast('Đã cập nhật tổ chức', 'success');
    }
}

async function saveProfile() {
    const name = document.getElementById('profileName').value.trim();
    if (!name) return showToast('Vui lòng nhập tên', 'error');
    const res = await fetchAPI('/api/auth/me', 'PUT', { name });
    if (res) {
        const user = JSON.parse(localStorage.getItem('thg_user') || '{}');
        const updated = { ...user, name };
        localStorage.setItem('thg_user', JSON.stringify(updated));
        updateSidebarUser(updated);
        showToast('Đã lưu thay đổi', 'success');
    }
}

async function changePassword() {
    const current = document.getElementById('currentPassword').value;
    const newPw = document.getElementById('newPassword').value;
    const confirm = document.getElementById('confirmPassword').value;
    if (!current || !newPw || !confirm) return showToast('Vui lòng điền đầy đủ', 'error');
    if (newPw !== confirm) return showToast('Mật khẩu mới không khớp', 'error');
    const res = await fetchAPI('/api/auth/me/password', 'PUT', { current_password: current, new_password: newPw, confirm_password: confirm });
    if (res) {
        showToast('Đổi mật khẩu thành công! Vui lòng đăng nhập lại', 'success');
        document.getElementById('currentPassword').value = '';
        document.getElementById('newPassword').value = '';
        document.getElementById('confirmPassword').value = '';
        setTimeout(doLogout, 2000);
    }
}

async function loadUsersTable() {
    const res = await fetchAPI('/api/auth/users');
    if (!res) return;
    const currentUser = JSON.parse(localStorage.getItem('thg_user') || '{}');
    renderTable('usersTable', res.users || [], u => `
        <td>${esc(u.name)}</td>
        <td>${esc(u.email)}</td>
        <td><span style="padding:2px 8px;border-radius:4px;font-size:12px;background:${u.role === 'admin' ? 'rgba(139,92,246,0.2)' : 'rgba(16,185,129,0.2)'};color:${u.role === 'admin' ? '#a78bfa' : '#6ee7b7'}">${u.role}</span></td>
        <td><span style="color:${u.active ? '#6ee7b7' : '#f87171'}">${u.active ? 'Hoạt động' : 'Vô hiệu'}</span></td>
        <td>${timeAgo(u.created_at)}</td>
        <td style="display:flex;gap:4px">
            <button class="btn btn-sm btn-ghost" onclick="showEditUserModal(${u.id},'${esc(u.name)}','${esc(u.email)}','${u.role}',${u.active})">✏️</button>
            ${u.id !== currentUser.id ? `<button class="btn btn-sm btn-danger" onclick="deleteUser(${u.id},'${esc(u.name)}')">🗑️</button>` : ''}
        </td>
    `);
}

function showCreateUserModal() {
    document.getElementById('createUserModal').style.display = 'flex';
    document.getElementById('newUserName').value = '';
    document.getElementById('newUserEmail').value = '';
    document.getElementById('newUserPassword').value = '';
    document.getElementById('newUserRole').value = 'sales';
}
function closeCreateUserModal() { document.getElementById('createUserModal').style.display = 'none'; }

async function submitCreateUser(e) {
    e.preventDefault();
    const res = await fetchAPI('/api/auth/users', 'POST', {
        name: document.getElementById('newUserName').value,
        email: document.getElementById('newUserEmail').value,
        password: document.getElementById('newUserPassword').value,
        role: document.getElementById('newUserRole').value,
    });
    if (res) {
        showToast(`Đã tạo tài khoản: ${res.email}`, 'success');
        closeCreateUserModal();
        loadUsersTable();
    }
}

function showEditUserModal(id, name, email, role, active) {
    document.getElementById('editUserModal').style.display = 'flex';
    document.getElementById('editUserId').value = id;
    document.getElementById('editUserName').value = name;
    document.getElementById('editUserEmail').value = email;
    document.getElementById('editUserRole').value = role;
    document.getElementById('editUserActive').value = String(active);
    document.getElementById('editUserPassword').value = '';
}
function closeEditUserModal() { document.getElementById('editUserModal').style.display = 'none'; }

async function submitEditUser(e) {
    e.preventDefault();
    const id = document.getElementById('editUserId').value;
    const newPw = document.getElementById('editUserPassword').value;
    const payload = {
        name: document.getElementById('editUserName').value,
        role: document.getElementById('editUserRole').value,
        active: document.getElementById('editUserActive').value === 'true',
    };
    if (newPw) payload.new_password = newPw;
    const res = await fetchAPI(`/api/auth/users/${id}`, 'PUT', payload);
    if (res) { showToast('Đã cập nhật tài khoản', 'success'); closeEditUserModal(); loadUsersTable(); }
}

async function deleteUser(id, name) {
    if (!confirm(`Xóa tài khoản "${name}"? Hành động này không thể hoàn tác.`)) return;
    const res = await fetchAPI(`/api/auth/users/${id}`, 'DELETE');
    if (res) { showToast(`Đã xóa tài khoản ${name}`, 'success'); loadUsersTable(); }
}

// ===== Logs Page =====

let logsSSE = null;

function loadLogsPage() {
    stopLogsStream();
    const container = document.getElementById('logsContainer');
    if (!container) return;

    const token = accessToken;
    logsSSE = new EventSource(`/api/logs/stream?token=${encodeURIComponent(token)}`);

    logsSSE.onmessage = e => {
        const line = document.createElement('div');
        const text = e.data || '';
        let color = '#94a3b8'; // default gray
        if (/error|❌|fatal/i.test(text)) color = '#f87171';
        else if (/warn|⚠/i.test(text)) color = '#fbbf24';
        else if (/✅/.test(text)) color = '#6ee7b7';
        line.style.cssText = `color:${color};white-space:pre-wrap;word-break:break-all`;
        line.textContent = text;
        container.appendChild(line);

        // Keep at most 500 lines to avoid memory bloat
        while (container.children.length > 500) container.removeChild(container.firstChild);

        const autoScroll = document.getElementById('logsAutoScroll');
        if (!autoScroll || autoScroll.checked) container.scrollTop = container.scrollHeight;
    };
    logsSSE.onerror = () => { /* browser will auto-reconnect */ };
}

function stopLogsStream() {
    if (logsSSE) { logsSSE.close(); logsSSE = null; }
}

function clearLogsDisplay() {
    const container = document.getElementById('logsContainer');
    if (container) container.innerHTML = '';
}

// ===== Sentiment / Analytics Page =====

async function loadSentimentPage() {
    const res = await fetchAPI('/api/analytics/sentiment');
    if (!res) return;

    const scores = res.score_breakdown || {};
    const outbound = res.outbound || {};

    // Stat cards
    const hot = scores.hot || 0, warm = scores.warm || 0, cold = scores.cold || 0;
    const total = hot + warm + cold || 1;
    setText('sentHot', `${hot} (${Math.round(hot/total*100)}%)`);
    setText('sentWarm', `${warm} (${Math.round(warm/total*100)}%)`);
    setText('sentCold', `${cold} (${Math.round(cold/total*100)}%)`);
    setText('sentCommentsSent', outbound.sent || 0);
    setText('sentInboxSent', outbound.inbox_sent || 0);
    setText('sentFailed', outbound.failed || 0);

    // Score breakdown bar chart
    renderBarChart('sentScoreChart', [
        { label: '🔥 Hot', value: hot, color: '#ef4444' },
        { label: '🌡 Warm', value: warm, color: '#f59e0b' },
        { label: '❄️ Cold', value: cold, color: '#60a5fa' },
    ], total);

    // Niche chart
    const niches = res.top_niches || [];
    const nicheMax = niches[0]?.count || 1;
    renderBarChart('sentNicheChart', niches.map(n => ({
        label: n.niche,
        value: n.count,
        color: '#8b5cf6',
    })), nicheMax);

    // Outbound performance
    const obTotal = Object.values(outbound).reduce((s, v) => s + (v || 0), 0) || 1;
    const obRows = Object.entries(outbound).map(([k, v]) => ({ label: k, value: v || 0, color: '#10b981' }));
    renderBarChart('sentOutboundChart', obRows, obTotal);
}

function setText(id, val) {
    const el = document.getElementById(id);
    if (el) el.textContent = val;
}

function renderBarChart(containerId, items, maxValue) {
    const container = document.getElementById(containerId);
    if (!container) return;
    if (!items || items.length === 0) {
        container.innerHTML = '<div style="color:var(--text-muted);font-size:13px;text-align:center;padding:20px">Chưa có dữ liệu</div>';
        return;
    }
    container.innerHTML = items.map(item => {
        const pct = Math.max(2, Math.round((item.value / maxValue) * 100));
        return `
            <div style="margin-bottom:10px">
                <div style="display:flex;justify-content:space-between;font-size:12px;margin-bottom:4px">
                    <span style="color:var(--text-secondary)">${esc(String(item.label))}</span>
                    <span style="color:var(--text-muted)">${item.value}</span>
                </div>
                <div style="background:rgba(255,255,255,0.08);border-radius:4px;height:8px;overflow:hidden">
                    <div style="width:${pct}%;height:100%;background:${item.color};border-radius:4px;transition:width 0.4s ease"></div>
                </div>
            </div>`;
    }).join('');
}

// ===== Page Navigation =====

function switchPage(page) {
    currentPage = page;
    document.querySelectorAll('.nav-item').forEach(el => el.classList.remove('active'));
    document.querySelector(`[data-page="${page}"]`).classList.add('active');
    document.querySelectorAll('.page').forEach(el => el.classList.remove('active'));
    document.getElementById(`page-${page}`).classList.add('active');

    const titles = {
        dashboard: ['Dashboard', 'Real-time overview'],
        browser:   ['Browser', 'Live Facebook browser — đăng nhập và điều khiển trực tiếp'],
        logs:      ['Logs', 'Real-time system log stream'],
        sentiment: ['Analytics', 'Lead sentiment & comment performance'],
        leads: ['Leads', 'AI-classified leads theo từng lĩnh vực'],
        posts: ['Posts', 'Scraped social media posts'],
        groups: ['Groups', 'Managed social media groups'],
        jobs: ['Jobs', 'Scraping job queue'],
        accounts: ['Accounts', 'Facebook account management'],
        aichat: ['AI Chat', 'Gửi prompt để AI agents thực thi'],
        outbox: ['Outbox', 'Auto-comment & auto-inbox queue'],
        settings: ['Settings', 'Tài khoản và cài đặt hệ thống'],
    };
    document.getElementById('pageTitle').textContent = titles[page]?.[0] || page;
    document.getElementById('pageSubtitle').textContent = titles[page]?.[1] || '';
    if (page === 'leads') loadNicheTabs();
    if (page === 'settings') loadSettingsPage();
    if (page === 'browser') loadBrowserPage();
    if (page === 'logs') loadLogsPage();
    if (page === 'sentiment') loadSentimentPage();
    // Stop log stream when navigating away
    if (page !== 'logs') stopLogsStream();
    refreshData();
}

// ===== Data Loading =====

async function refreshData() {
    try {
        switch (currentPage) {
            case 'dashboard': await loadDashboard(); break;
            case 'browser':   await loadBrowserWorkspaces(); break;
            case 'leads':     await loadLeads(); break;
            case 'posts':     await loadPosts(); break;
            case 'groups':    await loadGroups(); break;
            case 'jobs':      await loadJobs(); break;
            case 'accounts':  await loadAccounts(); break;
            case 'aichat':    await loadPromptHistory(); break;
            case 'outbox':    await loadOutbox(); break;
            case 'sentiment': await loadSentimentPage(); break;
        }
    } catch (e) { console.error('Refresh error:', e); }
}

async function loadDashboard() {
    const [stats, leadsRes, orgRes] = await Promise.all([
        fetchAPI('/api/stats'),
        fetchAPI('/api/leads?limit=5&score=hot'),
        fetchAPI('/api/org'),
    ]);
    if (stats) {
        document.getElementById('statGroups').textContent = stats.active_groups || 0;
        document.getElementById('statLeads').textContent = stats.total_leads || 0;
        document.getElementById('statHotLeads').textContent = stats.hot_leads || 0;
        document.getElementById('statRunning').textContent = stats.running_jobs || 0;
    }
    if (orgRes && orgRes.org) {
        const org = orgRes.org;
        const planLabels = { free: 'Free', pro: 'Pro ✨', enterprise: 'Enterprise ⚡' };
        const nameEl = document.getElementById('dashboardOrgName');
        const subEl = document.getElementById('dashboardOrgSub');
        if (nameEl) nameEl.textContent = org.name || '—';
        if (subEl) {
            const accCount = orgRes.account_count || 0;
            const accLimit = org.max_accounts === 0 ? '∞' : (org.max_accounts || '?');
            const planName = planLabels[org.plan_tier] || org.plan_tier || 'Free';
            subEl.textContent = `${planName} · ${accCount}/${accLimit} tài khoản Facebook`;
        }
        // Show getting-started guide when no Facebook accounts
        const gsEl = document.getElementById('dashboardGetStarted');
        if (gsEl) gsEl.style.display = (orgRes.account_count || 0) === 0 ? '' : 'none';
    }
    if (leadsRes && leadsRes.leads) {
        renderTable('dashboardLeadsTable', leadsRes.leads, lead => `
            <td><span class="badge badge-${lead.score}">${scoreIcon(lead.score)} ${lead.score}</span></td>
            <td>${lead.author_url ? `<a href="${esc(lead.author_url)}" target="_blank" style="color:var(--accent-alt)">${esc(lead.author || 'Unknown')}</a>` : esc(lead.author || 'Unknown')}</td>
            <td title="${esc(lead.content)}">${esc(trunc(lead.content, 80))}</td>
            <td>${esc(lead.service_match)}</td>
            <td>${timeAgo(lead.created_at)}</td>
            <td>${lead.source_url ? `<a href="${esc(lead.source_url)}" target="_blank" class="btn btn-sm btn-ghost">🔗 Xem</a>` : '-'}</td>
            <td><button class="btn btn-sm btn-danger" onclick="deleteLead(${lead.id})">🗑️</button></td>
        `);
    }
}

async function loadLeads() {
    const score = document.getElementById('leadScoreFilter').value;
    const nicheParam = activeNicheFilter ? `&niche=${activeNicheFilter}` : '';
    const res = await fetchAPI(`/api/leads?limit=100&score=${score}${nicheParam}`);
    if (!res) return;
    const leadsData = res.leads || [];

    // Update page title with niche info
    const titleEl = document.getElementById('leadsTitle');
    if (activeNicheFilter) {
        const tabs = document.querySelectorAll('.niche-tab[data-niche]');
        let nicheName = activeNicheFilter;
        tabs.forEach(t => { if (t.dataset.niche === activeNicheFilter) nicheName = t.textContent; });
        titleEl.textContent = `🎯 Leads — ${nicheName}`;
    } else {
        titleEl.textContent = '🎯 Classified Leads';
    }

    renderTable('leadsTable', leadsData, lead => `
        <td><span class="badge badge-${lead.score}">${scoreIcon(lead.score)} ${lead.score}</span></td>
        <td><span style="font-size:11px;padding:2px 8px;border-radius:10px;background:rgba(139,92,246,0.15);color:#a78bfa">${esc(lead.niche || 'logistics')}</span></td>
        <td>${lead.author_url ? `<a href="${esc(lead.author_url)}" target="_blank" style="color:var(--accent-alt)">${esc(lead.author || 'Unknown')}</a>` : esc(lead.author || 'Unknown')}</td>
        <td title="${esc(lead.content)}">${esc(trunc(lead.content, 60))}</td>
        <td>${esc(lead.service_match)}</td>
        <td>${roleTag(lead.author_role, lead.niche)}</td>
        <td>${esc(lead.pain_point || '-')}</td>
        <td>${lead.commented ? '<span class="badge badge-commented">✅ Đã comment</span>' : '<span style="color:var(--text-muted)">—</span>'}</td>
        <td>${timeAgo(lead.created_at)}</td>
        <td>${lead.source_url ? `<a href="${esc(lead.source_url)}" target="_blank" class="btn btn-sm btn-ghost">🔗 Xem</a>` : '-'}</td>
        <td><button class="btn btn-sm btn-danger" onclick="deleteLead(${lead.id})">🗑️</button></td>
    `);
}

// ===== Niche Tabs =====

async function loadNicheTabs() {
    const res = await fetchAPI('/api/niches');
    if (!res || !res.niches) return;
    const container = document.getElementById('nicheTabsContainer');
    container.innerHTML = res.niches.map(n =>
        `<button class="niche-tab ${activeNicheFilter === n.slug ? 'active' : ''}" data-niche="${esc(n.slug)}" onclick="setActiveNiche(this,'${esc(n.slug)}')">${esc(n.emoji)} ${esc(n.name)}</button>`
    ).join('');

    // Ensure "Tất cả" tab has active if no specific niche is selected
    if (activeNicheFilter === '') {
        const allBtn = document.querySelector('.niche-tab[data-niche=""]');
        if (allBtn) allBtn.classList.add('active');
    }
}

function setActiveNiche(el, niche) {
    activeNicheFilter = niche;
    document.querySelectorAll('.niche-tab').forEach(t => t.classList.remove('active'));
    if (el) el.classList.add('active');
    // Show/hide recruitment-specific controls
    const jobPostBtn = document.getElementById('btnCreateJobPost');
    if (jobPostBtn) jobPostBtn.style.display = niche === 'tuyen_dung' ? 'inline-flex' : 'none';
    const recruitBtn = document.getElementById('btnRecruitCandidates');
    if (recruitBtn) recruitBtn.style.display = niche === 'tuyen_dung' ? 'inline-flex' : 'none';
    loadLeads();
}

async function recruitAllCandidates() {
    if (!confirm('Comment outreach tất cả ứng viên hot đang tìm việc? AI sẽ soạn comment dựa trên JD hiện có.')) return;
    showToast('Đang xếp hàng outreach...', 'info');
    const res = await fetchAPI('/api/ai/prompt', 'POST', { prompt: 'comment ứng viên hot tìm việc', source: 'dashboard' });
    if (res) showToast('Đã xếp hàng — theo dõi kết quả trong Outbox', 'success');
}

function showJobPostModal() { document.getElementById('jobPostModal').classList.add('active'); }
function closeJobPostModal() { document.getElementById('jobPostModal').classList.remove('active'); }

async function submitJobPost(e) {
    e.preventDefault();
    const title = document.getElementById('jpTitle').value.trim();
    if (!title) { showToast('Nhập tên vị trí tuyển dụng', 'error'); return; }
    const prompt = `Tạo bài đăng tuyển dụng vị trí: ${title}. Mô tả: ${document.getElementById('jpDesc').value}. Yêu cầu: ${document.getElementById('jpReqs').value}. Quyền lợi: ${document.getElementById('jpBenefits').value}. Lương: ${document.getElementById('jpSalary').value}. Email: ${document.getElementById('jpEmail').value || 'career@thgfulfill.com'}`;
    closeJobPostModal();
    showToast('Đang tạo bài tuyển dụng...', 'info');
    const res = await fetchAPI('/api/ai/prompt', 'POST', { prompt, source: 'dashboard' });
    if (res) showToast('Đã tạo bài tuyển dụng — xem tại Outbox', 'success');
    ['jpTitle', 'jpDesc', 'jpReqs', 'jpBenefits', 'jpSalary', 'jpEmail'].forEach(id => { const el = document.getElementById(id); if (el) el.value = ''; });
}

function showAddNicheModal() { document.getElementById('addNicheModal').classList.add('active'); }
function closeNicheModal() { document.getElementById('addNicheModal').classList.remove('active'); }

async function submitAddNiche(e) {
    e.preventDefault();
    const data = {
        slug: document.getElementById('nicheSlug').value.trim().toLowerCase().replace(/\s+/g, '_'),
        name: document.getElementById('nicheName').value.trim(),
        emoji: document.getElementById('nicheEmoji').value.trim() || '🎯',
    };
    const res = await fetchAPI('/api/niches', 'POST', data);
    if (res) {
        showToast(`✅ Đã thêm lĩnh vực: ${data.name}`, 'success');
        closeNicheModal();
        ['nicheSlug', 'nicheName', 'nicheEmoji'].forEach(id => document.getElementById(id).value = '');
        await loadNicheTabs();
    }
}

async function deleteLead(id) {
    if (!confirm('Xóa lead này?')) return;
    const res = await fetch('/api/leads/' + id, { method: 'DELETE', headers: { 'Authorization': `Bearer ${accessToken}` } });
    if (res.ok) { loadLeads(); loadDashboard(); }
}

async function loadPosts() {
    const res = await fetchAPI('/api/posts?limit=50');
    if (!res) return;
    renderTable('postsTable', res.posts || [], post => `
        <td>${esc(post.group_name || '-')}</td>
        <td>${esc(post.author || 'Unknown')}</td>
        <td title="${esc(post.content)}">${esc(trunc(post.content, 80))}</td>
        <td>${post.reactions || 0}</td>
        <td>${timeAgo(post.scraped_at)}</td>
        <td>${post.url ? `<a href="${esc(post.url)}" target="_blank" class="btn btn-sm btn-ghost">🔗 Xem</a>` : '-'}</td>
        <td><button class="btn btn-sm btn-danger" onclick="deletePost(${post.id})">🗑️</button></td>
    `);
}

async function deletePost(id) {
    if (!confirm('Xóa post này?')) return;
    const res = await fetch('/api/posts/' + id, { method: 'DELETE', headers: { 'Authorization': `Bearer ${accessToken}` } });
    if (res.ok) loadPosts();
}

async function deleteAllPosts() {
    if (!confirm('⚠️ Xóa TẤT CẢ posts? (Groups sẽ được giữ lại)')) return;
    const res = await fetch('/api/posts/all', { method: 'DELETE', headers: { 'Authorization': `Bearer ${accessToken}` } });
    if (res.ok) {
        const data = await res.json();
        showToast(`Đã xóa ${data.deleted} posts`, 'success');
        loadPosts();
        loadDashboard();
    }
}

async function deleteAllLeads() {
    const niche = activeNicheFilter;
    const scopeLabel = niche ? `lĩnh vực "${niche}"` : 'TẤT CẢ lĩnh vực';
    if (!confirm(`⚠️ Xóa leads của ${scopeLabel}? (Posts và groups sẽ được giữ lại)`)) return;
    const url = '/api/leads/all' + (niche ? `?niche=${encodeURIComponent(niche)}` : '');
    const res = await fetch(url, { method: 'DELETE', headers: { 'Authorization': `Bearer ${accessToken}` } });
    if (res.ok) {
        const data = await res.json();
        showToast(`Đã xóa ${data.deleted} leads (${data.scope})`, 'success');
        loadLeads();
        loadDashboard();
    }
}

async function loadGroups() {
    const res = await fetchAPI('/api/groups');
    if (!res) return;
    renderTable('groupsTable', res.groups || [], group => `
        <td><span class="badge ${group.active ? 'badge-done' : 'badge-failed'}">${group.active ? '✅ Active' : '❌ Off'}</span></td>
        <td>${esc(group.name)}</td>
        <td>${esc(group.platform)}</td>
        <td><a href="${esc(group.url)}" target="_blank" style="color:var(--accent-alt)">${esc(trunc(group.url, 40))}</a></td>
        <td>${group.last_scan ? timeAgo(group.last_scan) : 'Never'}</td>
        <td>
            <button class="btn btn-sm btn-ghost" onclick="toggleGroup(${group.id}, ${!group.active})">${group.active ? '⏸' : '▶'}</button>
            <button class="btn btn-sm btn-danger" onclick="deleteGroup(${group.id})">🗑</button>
        </td>
    `);
}

async function loadJobs() {
    const status = document.getElementById('jobStatusFilter').value;
    const res = await fetchAPI(`/api/jobs?limit=50&status=${status}`);
    if (!res) return;
    renderTable('jobsTable', res.jobs || [], job => `
        <td>#${job.id}</td>
        <td>${esc(job.type)}</td>
        <td>${esc(job.platform)}</td>
        <td title="${esc(job.target)}">${esc(trunc(job.target, 40))}</td>
        <td><span class="badge badge-${job.status}">${statusIcon(job.status)} ${job.status}</span></td>
        <td>${timeAgo(job.created_at)}</td>
        <td>${job.status === 'running' ? `<button class="btn btn-sm btn-danger" onclick="cancelJob(${job.id})">⏹</button>` : ''}</td>
    `);
}

// ===== Accounts =====

async function loadAccounts() {
    const res = await fetchAPI('/api/accounts');
    if (!res || !res.accounts) return;
    const accounts = res.accounts;

    // Update summary stats
    const counts = { active: 0, cooldown: 0, banned: 0, inactive: 0 };
    accounts.forEach(a => { if (counts[a.status] !== undefined) counts[a.status]++; });
    document.getElementById('accStatActive').textContent = counts.active;
    document.getElementById('accStatCooldown').textContent = counts.cooldown;
    document.getElementById('accStatBanned').textContent = counts.banned;
    document.getElementById('accStatInactive').textContent = counts.inactive;

    const statusConfig = {
        active: { badge: 'background:rgba(16,185,129,0.15);color:#6ee7b7', icon: '✅' },
        cooldown: { badge: 'background:rgba(245,158,11,0.15);color:#fcd34d', icon: '⏳' },
        banned: { badge: 'background:rgba(239,68,68,0.15);color:#fca5a5', icon: '🚫' },
        inactive: { badge: 'background:rgba(100,116,139,0.15);color:#94a3b8', icon: '💤' },
    };

    const currentUser = JSON.parse(localStorage.getItem('thg_user') || '{}');
    const isAdmin = currentUser.role === 'admin';

    renderTable('accountsTable', accounts, acc => {
        const cfg = statusConfig[acc.status] || statusConfig.inactive;
        const cookieAge = acc.last_used ? Math.floor((Date.now() - new Date(acc.last_used)) / 86400000) : null;
        const cookieWarning = cookieAge !== null && cookieAge > 14;
        const staffBadge = acc.assigned_user_name
            ? `<span style="font-size:11px;padding:2px 8px;border-radius:10px;background:rgba(139,92,246,0.12);color:#a78bfa">${esc(acc.assigned_user_name)}</span>`
            : '<span style="color:var(--text-muted);font-size:12px">—</span>';
        return `
            <td><span style="padding:3px 10px;border-radius:20px;font-size:12px;font-weight:500;${cfg.badge}">${cfg.icon} ${acc.status}</span></td>
            <td style="font-weight:500">${esc(acc.name)}</td>
            <td>${staffBadge}</td>
            <td style="color:var(--text-muted);font-size:13px">${esc(acc.email || '—')}</td>
            <td><span style="font-size:12px;padding:2px 8px;border-radius:4px;background:rgba(59,130,246,0.1);color:#93c5fd">${esc(acc.platform)}</span></td>
            <td>${cookieWarning
                ? `<span style="color:#fcd34d" title="Cookie có thể đã hết hạn (${cookieAge} ngày)">⚠️ ${cookieAge}d</span>`
                : acc.last_used ? `<span style="color:#6ee7b7">🟢 OK</span>` : '<span style="color:var(--text-muted)">Chưa login</span>'
            }</td>
            <td style="font-size:13px">${acc.last_used ? timeAgo(acc.last_used) : '<span style="color:var(--text-muted)">—</span>'}</td>
            <td style="font-size:12px;color:var(--text-muted);max-width:120px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap" title="${esc(acc.notes || '')}">${esc(acc.notes || '—')}</td>
            <td style="display:flex;gap:4px;flex-wrap:wrap">
                <button class="btn btn-sm" style="background:rgba(139,92,246,0.15);color:#a78bfa;border:1px solid rgba(139,92,246,0.3)" title="Kết nối Facebook (mở Chrome tự động)" onclick="openDirectLogin(${acc.id})">🔑 Login</button>
                ${isAdmin ? `<button class="btn btn-sm btn-ghost" title="Cập nhật cookie thủ công" onclick="showUpdateCookieModal(${acc.id})">🔄</button>` : ''}
                ${isAdmin ? `<button class="btn btn-sm btn-ghost" title="${acc.status === 'active' ? 'Tạm dừng' : 'Kích hoạt'}" onclick="toggleAccountStatus(${acc.id}, '${acc.status === 'active' ? 'inactive' : 'active'}')">${acc.status === 'active' ? '⏸' : '▶️'}</button>` : ''}
                ${isAdmin ? `<button class="btn btn-sm btn-danger" title="Xóa tài khoản" onclick="deleteAccount(${acc.id})">🗑</button>` : ''}
            </td>
        `;
    });
}

function showAddAccountModal() {
    document.getElementById('addAccountModal').classList.add('active');
    ['accName', 'accEmail', 'accCookies', 'accProxy', 'accNotes'].forEach(id => {
        const el = document.getElementById(id);
        if (el) el.value = '';
    });
    document.getElementById('accError').style.display = 'none';
    document.getElementById('cookieValidation').style.display = 'none';
}
function closeAccountModal() { document.getElementById('addAccountModal').classList.remove('active'); }

function validateCookieJSON() {
    const raw = document.getElementById('accCookies').value.trim();
    const el = document.getElementById('cookieValidation');
    try {
        const parsed = JSON.parse(raw);
        if (!Array.isArray(parsed)) throw new Error('Phải là array JSON');
        const hasCUser = parsed.some(c => c.name === 'c_user');
        const hasXS = parsed.some(c => c.name === 'xs');
        if (!hasCUser || !hasXS) {
            el.style.display = 'block';
            el.style.color = '#fcd34d';
            el.textContent = `⚠️ Tìm thấy ${parsed.length} cookies nhưng thiếu trường quan trọng (c_user: ${hasCUser ? '✅' : '❌'}, xs: ${hasXS ? '✅' : '❌'}). Vẫn có thể thử nhưng có thể không hoạt động.`;
        } else {
            el.style.display = 'block';
            el.style.color = '#6ee7b7';
            el.textContent = `✅ JSON hợp lệ — ${parsed.length} cookies (c_user: ✅, xs: ✅)`;
        }
    } catch (err) {
        el.style.display = 'block';
        el.style.color = '#fca5a5';
        el.textContent = `❌ JSON không hợp lệ: ${err.message}`;
    }
}

async function submitAddAccount(e) {
    if (e) e.preventDefault();
    const cookieRaw = document.getElementById('accCookies').value.trim();
    if (cookieRaw) {
        try { JSON.parse(cookieRaw); } catch {
            document.getElementById('accError').style.display = 'block';
            document.getElementById('accError').textContent = 'Cookies JSON không hợp lệ. Kiểm tra lại format.';
            return;
        }
    }
    const data = {
        platform: document.getElementById('accPlatform').value || 'facebook',
        name: document.getElementById('accName').value,
        email: (document.getElementById('accEmail')?.value || ''),
        cookies_json: cookieRaw,
        proxy_url: (document.getElementById('accProxy')?.value || ''),
        notes: (document.getElementById('accNotes')?.value || ''),
    };
    const res = await fetchAPI('/api/accounts', 'POST', data);
    if (res) {
        showToast('Đã thêm tài khoản!', 'success');
        closeAccountModal();
        loadAccounts();
    }
}

async function submitAddAccountAndLogin() {
    const nameEl = document.getElementById('accName');
    if (!nameEl.value.trim()) {
        document.getElementById('accError').style.display = 'block';
        document.getElementById('accError').textContent = 'Vui lòng nhập Tên tài khoản FB';
        return;
    }

    const cookieRaw = document.getElementById('accCookies').value.trim();
    if (cookieRaw) {
        try { JSON.parse(cookieRaw); } catch {
            document.getElementById('accError').style.display = 'block';
            document.getElementById('accError').textContent = 'Cookies JSON không hợp lệ. Kiểm tra lại format.';
            return;
        }
    }

    const data = {
        platform: document.getElementById('accPlatform').value || 'facebook',
        name: nameEl.value.trim(),
        email: (document.getElementById('accEmail')?.value || '').trim(),
        cookies_json: cookieRaw,
        proxy_url: (document.getElementById('accProxy')?.value || '').trim(),
        notes: (document.getElementById('accNotes')?.value || '').trim(),
    };

    const r = await fetch('/api/accounts', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${accessToken}` },
        body: JSON.stringify(data)
    }).catch(e => { showToast('Lỗi kết nối: ' + e.message, 'error'); return null; });

    if (!r) return;

    const res = await r.json().catch(() => ({}));
    if (!r.ok) {
        const errEl = document.getElementById('accError');
        errEl.textContent = res.error || `Lỗi ${r.status} — thử đăng xuất và đăng nhập lại`;
        errEl.style.display = 'block';
        return;
    }

    if (res.account_id) {
        closeAccountModal();
        loadAccounts();
        openDirectLogin(res.account_id);
    }
}

function showUpdateCookieModal(id) {
    document.getElementById('updateCookieAccId').value = id;
    document.getElementById('updateCookieJSON').value = '';
    document.getElementById('updateCookieModal').classList.add('active');
}
function closeUpdateCookieModal() { document.getElementById('updateCookieModal').classList.remove('active'); }

async function submitUpdateCookie(e) {
    e.preventDefault();
    const id = document.getElementById('updateCookieAccId').value;
    const cookieRaw = document.getElementById('updateCookieJSON').value.trim();
    try { JSON.parse(cookieRaw); } catch {
        showToast('Cookies JSON không hợp lệ', 'error'); return;
    }
    const res = await fetchAPI(`/api/accounts/${id}/cookies`, 'PUT', { cookies_json: cookieRaw });
    if (res) { showToast('Đã cập nhật cookie!', 'success'); closeUpdateCookieModal(); loadAccounts(); }
}

async function toggleAccountStatus(id, status) {
    await fetchAPI(`/api/accounts/${id}/status`, 'PUT', { status });
    showToast(`Tài khoản đã ${status === 'active' ? 'kích hoạt' : 'tạm dừng'}`, 'info');
    loadAccounts();
}

async function deleteAccount(id) {
    if (!confirm('Delete this account?')) return;
    await fetchAPI(`/api/accounts/${id}`, 'DELETE');
    showToast('Account deleted', 'info');
    loadAccounts();
}

// ===== Direct Chrome Login =====
let dlAccountId = null;
let dlPoll = null;

async function openDirectLogin(accountId) {
    dlAccountId = accountId;
    document.getElementById('chromeLoginModal').classList.add('active');
    document.getElementById('dlSuccess').style.display = 'none';
    document.getElementById('dlDoneBtn').style.display = 'none';
    document.getElementById('dlCancelBtn').style.display = '';
    setDlStatus('starting', 'Đang khởi động Chrome...', 'Vui lòng chờ trong giây lát');
    setDlStep(1, 'active'); setDlStep(2, 'inactive'); setDlStep(3, 'inactive');

    // Use raw fetch so we can read the error body on failure
    let startData;
    try {
        const r = await fetch(`/api/accounts/${accountId}/start-login`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${accessToken}` }
        });
        startData = await r.json().catch(() => ({}));
        if (!r.ok) {
            const hint = startData.hint || 'Kiểm tra Chrome đã cài đặt, hoặc set CHROME_PATH trong .env';
            setDlStatus('error', startData.error || `Lỗi ${r.status}`, hint);
            return;
        }
    } catch (e) {
        setDlStatus('error', 'Lỗi kết nối server', e.message);
        return;
    }

    setDlStatus('waiting', 'Chrome đã mở! Hãy đăng nhập Facebook', 'Đăng nhập bình thường trong cửa sổ Chrome vừa mở');
    setDlStep(1, 'done'); setDlStep(2, 'active');

    if (dlPoll) clearInterval(dlPoll);
    dlPoll = setInterval(pollChromeLogin, 2500);
}

async function pollChromeLogin() {
    if (!dlAccountId) return;
    const res = await fetchAPI(`/api/accounts/${dlAccountId}/login-status`).catch(() => null);
    if (!res) return;

    if (res.logged_in) {
        clearInterval(dlPoll); dlPoll = null;
        setDlStatus('saving', 'Phát hiện đăng nhập! Đang lưu phiên...', '');
        setDlStep(2, 'done'); setDlStep(3, 'active');

        const cap = await fetchAPI(`/api/accounts/${dlAccountId}/capture-session`, 'POST');
        if (cap && cap.status === 'saved') {
            setDlStatus('success', 'Kết nối thành công!', `Đã lưu ${cap.cookies_count} cookies`);
            setDlStep(3, 'done');
            document.getElementById('dlSuccess').style.display = '';
            document.getElementById('dlCancelBtn').style.display = 'none';
            document.getElementById('dlDoneBtn').style.display = '';
            showToast('✅ Tài khoản Facebook đã được kết nối!', 'success');
            loadAccounts();
        } else {
            setDlStatus('error', 'Lưu phiên thất bại', cap?.error || 'Thử lại');
        }
    }
}

function setDlStatus(type, text, sub) {
    const banner = document.getElementById('dlStatusBanner');
    const cfgs = {
        starting: { bg: 'rgba(59,130,246,0.08)',  border: 'rgba(59,130,246,0.3)',  color: '#93c5fd', icon: '🔄' },
        waiting:  { bg: 'rgba(245,158,11,0.08)',  border: 'rgba(245,158,11,0.3)',  color: '#fcd34d', icon: '⏳' },
        saving:   { bg: 'rgba(139,92,246,0.08)', border: 'rgba(139,92,246,0.3)', color: '#c4b5fd', icon: '💾' },
        success:  { bg: 'rgba(16,185,129,0.08)', border: 'rgba(16,185,129,0.3)', color: '#6ee7b7', icon: '✅' },
        error:    { bg: 'rgba(239,68,68,0.08)',  border: 'rgba(239,68,68,0.3)',  color: '#fca5a5', icon: '❌' },
    };
    const c = cfgs[type] || cfgs.waiting;
    banner.style.cssText = `padding:14px 16px;border-radius:8px;margin-bottom:20px;font-size:14px;display:flex;align-items:center;gap:12px;background:${c.bg};border:1px solid ${c.border};color:${c.color}`;
    document.getElementById('dlStatusIcon').textContent = c.icon;
    document.getElementById('dlStatusText').textContent = text;
    document.getElementById('dlStatusSub').textContent = sub || '';
}

const DL_STEP_TEXT = ['', 'Chrome đang mở trang đăng nhập Facebook', 'Đăng nhập Facebook trong cửa sổ Chrome', 'Hệ thống tự động lưu và kích hoạt tài khoản 🎉'];
function setDlStep(n, state) {
    const el = document.getElementById(`dlStep${n}`);
    if (!el) return;
    const colors = { active: '#c4b5fd', done: '#6ee7b7', inactive: 'var(--text-muted)' };
    const prefix = { active: '▶ ', done: '✅ ', inactive: '' }[state] || '';
    el.style.color = colors[state] || colors.inactive;
    el.textContent = prefix + DL_STEP_TEXT[n];
}

function closeDirectLogin() {
    if (dlPoll) { clearInterval(dlPoll); dlPoll = null; }
    if (dlAccountId) fetchAPI(`/api/accounts/${dlAccountId}/stop-login`, 'POST').catch(() => {});
    dlAccountId = null;
    document.getElementById('chromeLoginModal').classList.remove('active');
}

// ===== AI Chat =====

async function sendAIPrompt(e) {
    e.preventDefault();
    const input = document.getElementById('chatInput');
    const prompt = input.value.trim();
    if (!prompt) return;

    // Add user message
    addChatMsg('user', prompt);
    input.value = '';
    document.getElementById('chatSendBtn').disabled = true;
    document.getElementById('chatSendBtn').textContent = '⏳...';

    try {
        const res = await fetchAPI('/api/ai/prompt', 'POST', { prompt });
        if (res && res.response) {
            addChatMsg('ai', res.response);
        } else if (res && res.error) {
            addChatMsg('error', res.error);
        } else {
            addChatMsg('error', 'Không nhận được phản hồi từ AI');
        }
        loadPromptHistory();
    } catch (e) {
        addChatMsg('error', 'Lỗi kết nối: ' + e.message);
    } finally {
        document.getElementById('chatSendBtn').disabled = false;
        document.getElementById('chatSendBtn').textContent = '🚀 Gửi';
    }
}

function addChatMsg(type, text) {
    const container = document.getElementById('chatMessages');
    const div = document.createElement('div');
    div.className = `chat-msg ${type}`;
    div.innerHTML = `<div class="chat-bubble">${type === 'user' ? '👤 ' : type === 'ai' ? '🤖 ' : '❌ '}${escHtml(text)}</div>`;
    container.appendChild(div);
    container.scrollTop = container.scrollHeight;
}

async function loadPromptHistory() {
    const res = await fetchAPI('/api/ai/history?limit=10');
    if (!res || !res.history) return;
    renderTable('promptHistoryTable', res.history, p => `
        <td>${timeAgo(p.created_at)}</td>
        <td title="${esc(p.user_prompt)}">${esc(trunc(p.user_prompt, 50))}</td>
        <td>${esc(p.action_taken || 'chat')}</td>
        <td>${p.success ? '<span class="badge badge-done">✅</span>' : '<span class="badge badge-failed">❌</span>'}</td>
    `);
}

// ===== Outbox =====

async function loadOutbox() {
    const status = document.getElementById('outboxStatusFilter').value;
    const res = await fetchAPI(`/api/outbox?limit=50&status=${status}`);
    if (!res) return;
    // Update stat cards
    const c = res.counts || {};
    document.getElementById('statDraft').textContent = c.draft || 0;
    document.getElementById('statApproved').textContent = c.approved || 0;
    document.getElementById('statSentOut').textContent = c.sent || 0;
    document.getElementById('statRejected').textContent = c.rejected || 0;

    if (!res.messages) return;
    renderTable('outboxTable', res.messages, m => {
        const typeBadge = m.type === 'comment' ? '💬 Comment' : '📬 Inbox';
        const statusBadge = {
            draft: 'badge-pending', approved: 'badge-done',
            sent: 'badge-running', rejected: 'badge-failed', failed: 'badge-failed'
        };
        const statusIcon = { draft: '✏️', approved: '✅', sent: '🚀', rejected: '❌', failed: '⚠️' };
        let actions = '';
        if (m.status === 'draft') {
            actions = `
                <button class="btn btn-sm btn-primary" onclick="approveOutbound(${m.id})">✅ Duyệt</button>
                <button class="btn btn-sm btn-danger" onclick="rejectOutbound(${m.id})">❌</button>
                <button class="btn btn-sm btn-ghost" onclick="deleteOutbound(${m.id})">🗑</button>
            `;
        } else if (m.status === 'approved') {
            actions = `<span class="badge badge-done">⏳ Đang chờ gửi...</span>`;
        }
        return `
            <td>${typeBadge}</td>
            <td title="${esc(m.target_url)}">${esc(m.target_name || trunc(m.target_url, 30))}</td>
            <td title="${esc(m.content)}">${esc(trunc(m.content, 60))}</td>
            <td><span class="badge ${statusBadge[m.status] || ''}">${statusIcon[m.status] || ''} ${m.status}</span></td>
            <td>${m.sent_at && m.status === 'sent' ? timeAgo(m.sent_at) : timeAgo(m.created_at)}</td>
            <td>${actions}</td>
        `;
    });
}

async function approveOutbound(id) {
    const res = await fetchAPI(`/api/outbox/${id}/approve`, 'PUT');
    if (res) { showToast('✅ Đã duyệt!', 'success'); loadOutbox(); }
}

async function rejectOutbound(id) {
    if (!confirm('Từ chối tin nhắn này?')) return;
    await fetchAPI(`/api/outbox/${id}/reject`, 'PUT');
    showToast('Đã từ chối', 'info');
    loadOutbox();
}

async function deleteOutbound(id) {
    if (!confirm('Xóa?')) return;
    await fetchAPI(`/api/outbox/${id}`, 'DELETE');
    showToast('Đã xóa', 'info');
    loadOutbox();
}

async function resetAllComments() {
    if (!confirm('⚠️ Xóa TẤT CẢ outbound comments (kể cả đã sent/failed)?\nLeads sẽ hiển thị "chưa comment" và có thể chạy lại.')) return;
    const res = await fetch('/api/outbox/comments/all', { method: 'DELETE' });
    if (res.ok) {
        const data = await res.json();
        showToast(`✅ Đã reset ${data.deleted} comments`, 'success');
        loadOutbox();
        loadLeads();
    }
}

// ===== Actions =====

async function triggerScanAll() {
    const btn = document.getElementById('scanAllBtn');
    btn.disabled = true; btn.textContent = '⏳...';
    try {
        const groupsRes = await fetchAPI('/api/groups');
        if (!groupsRes || !groupsRes.groups) { showToast('No groups', 'error'); return; }
        let count = 0;
        for (const g of groupsRes.groups) {
            if (!g.active) continue;
            const r = await fetchAPI('/api/jobs', 'POST', { type: 'SCRAPE_POSTS', platform: g.platform, target: g.url });
            if (r) count++;
        }
        showToast(`🚀 Created ${count} jobs!`, 'success');
        refreshData();
    } finally { btn.disabled = false; btn.innerHTML = '<span>🚀</span> Scan All'; }
}

async function cancelJob(id) { await fetchAPI(`/api/jobs/${id}`, 'DELETE'); showToast(`Job #${id} canceled`); loadJobs(); }
async function toggleGroup(id, active) { await fetchAPI(`/api/groups/${id}/toggle`, 'PUT', { active }); showToast(`Group ${active ? 'activated' : 'paused'}`); loadGroups(); }
async function deleteGroup(id) { if (!confirm('Delete?')) return; await fetchAPI(`/api/groups/${id}`, 'DELETE'); showToast('Deleted'); loadGroups(); }

function showAddGroupModal() { document.getElementById('addGroupModal').classList.add('active'); }
function closeModal() { document.getElementById('addGroupModal').classList.remove('active'); }

async function submitAddGroup(e) {
    e.preventDefault();
    const data = { platform: document.getElementById('groupPlatform').value, name: document.getElementById('groupName').value, url: document.getElementById('groupURL').value };
    const res = await fetchAPI('/api/groups', 'POST', data);
    if (res) { showToast('Group added!', 'success'); closeModal(); loadGroups(); }
}

// ===== Utilities =====

let isRefreshing = false;
let refreshSubscribers = [];

function subscribeTokenRefresh(cb) { refreshSubscribers.push(cb); }
function onRefreshed(token) { refreshSubscribers.forEach(cb => cb(token)); refreshSubscribers = []; }

async function fetchAPI(url, method = 'GET', body = null, retryCount = 0) {
    try {
        const opts = { method, headers: { 'Content-Type': 'application/json' } };
        if (accessToken) opts.headers['Authorization'] = `Bearer ${accessToken}`;
        if (body) opts.body = JSON.stringify(body);

        const res = await fetch(API + url, opts);

        if (res.status === 401) {
            if (retryCount >= 1) {
                showLogin();
                return null;
            }
            if (url === '/api/auth/refresh') {
                showLogin();
                return null;
            }

            if (!isRefreshing) {
                isRefreshing = true;
                try {
                    const storedRefresh = localStorage.getItem('thg_refresh');
                    const refreshHeaders = storedRefresh ? { 'X-Refresh-Token': storedRefresh } : {};
                    const refreshRes = await fetch(API + '/api/auth/refresh', { method: 'POST', headers: refreshHeaders });
                    if (refreshRes.status === 429) {
                        console.warn('Refresh rate limited (429). Waiting for next interval.');
                        isRefreshing = false;
                        onRefreshed(null);
                        return null;
                    }
                    if (!refreshRes.ok) throw new Error('Refresh failed');
                    const data = await refreshRes.json();
                    accessToken = data.access_token;
                    localStorage.setItem('thg_token', accessToken);
                    if (data.refresh_token) localStorage.setItem('thg_refresh', data.refresh_token);
                    isRefreshing = false;
                    onRefreshed(accessToken);
                } catch (e) {
                    isRefreshing = false;
                    accessToken = '';
                    localStorage.removeItem('thg_token');
                    localStorage.removeItem('thg_refresh');
                    localStorage.removeItem('thg_user');
                    onRefreshed(null);
                    if (refreshInterval) { clearInterval(refreshInterval); refreshInterval = null; }
                    showLogin();
                    return null;
                }
            }

            // Wait for refresh to finish if another request triggered it
            return new Promise(resolve => {
                subscribeTokenRefresh(newToken => {
                    if (newToken) {
                        resolve(fetchAPI(url, method, body, retryCount + 1));
                    } else {
                        resolve(null);
                    }
                });
            });
        }

        if (!res.ok) {
            try { lastApiError = await res.json(); } catch { lastApiError = null; }
            throw new Error((lastApiError && lastApiError.error) || `HTTP ${res.status}`);
        }
        return await res.json();
    } catch (e) {
        if (retryCount === 0) console.error(`API [${method} ${url}]:`, e);
        return null;
    }
}

function renderTable(id, data, renderer) {
    const tbody = document.querySelector(`#${id} tbody`);
    if (!data || data.length === 0) {
        tbody.innerHTML = `<tr><td colspan="10" style="text-align:center;color:var(--text-muted);padding:40px">No data yet</td></tr>`;
        return;
    }
    tbody.innerHTML = data.map(item => `<tr>${renderer(item)}</tr>`).join('');
}

function esc(s) { if (!s) return ''; const d = document.createElement('div'); d.textContent = s; return d.innerHTML; }
function escHtml(s) { return s.replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/\n/g, '<br>'); }
function trunc(s, n) { if (!s) return ''; return s.length > n ? s.substring(0, n) + '...' : s; }

function timeAgo(d) {
    if (!d) return '-';
    const diff = (Date.now() - new Date(d).getTime()) / 1000;
    if (diff < 60) return 'Just now';
    if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
    if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
    return `${Math.floor(diff / 86400)}d ago`;
}

function scoreIcon(s) { return ({ hot: '🔥', warm: '🟡', cold: '🔵', rejected: '⚪' })[s] || '⚪'; }

// roleTag renders author_role with niche-aware label and color
function roleTag(role, niche) {
    if (!role) return '<span style="color:var(--text-muted)">—</span>';
    const isHR = niche === 'tuyen_dung';
    const styles = {
        candidate: 'background:rgba(34,197,94,0.15);color:#4ade80',
        recruiter: 'background:rgba(239,68,68,0.15);color:#f87171',
        buyer: 'background:rgba(59,130,246,0.15);color:#60a5fa',
        seller: 'background:rgba(239,68,68,0.15);color:#f87171',
        provider: 'background:rgba(168,85,247,0.15);color:#c084fc',
        unknown: 'background:rgba(107,114,128,0.15);color:#9ca3af',
    };
    const labels = {
        candidate: '👤 Ứng viên',
        recruiter: '🏢 Nhà tuyển dụng',
        buyer: '🛒 Buyer',
        seller: '📢 Seller',
        provider: '🔧 Provider',
        unknown: '❓ Unknown',
    };
    const style = styles[role] || styles.unknown;
    const label = labels[role] || role;
    return `<span style="font-size:11px;padding:2px 8px;border-radius:10px;${style}">${label}</span>`;
}
function statusIcon(s) { return ({ running: '🔄', done: '✅', failed: '❌', pending: '⏳', canceled: '🛑' })[s] || ''; }
function accStatusIcon(s) { return ({ active: '🟢', cooldown: '🟡', banned: '🔴', inactive: '⚫' })[s] || '⚪'; }

function showToast(msg, type = 'info') {
    const t = document.createElement('div');
    t.className = `toast toast-${type}`;
    t.textContent = msg;
    document.getElementById('toastContainer').appendChild(t);
    setTimeout(() => t.remove(), 3000);
}

// ===== System Info (headless mode detection) =====
let serverHeadless = false;

async function loadSystemInfo() {
    try {
        const res = await fetch('/api/system/info');
        if (res.ok) {
            const data = await res.json();
            serverHeadless = !!data.headless;
        }
    } catch { /* non-critical */ }
}

// ===== Session Restore =====

// Always calls /api/auth/refresh on page load to get a server-verified fresh token.
// Sends the refresh token via X-Refresh-Token header (works even when Cookie header
// is stripped by a reverse proxy like nginx). Cookie is also sent as a fallback.
// Returns true if a valid session was established.
async function tryRestoreSession() {
    const storedRefresh = localStorage.getItem('thg_refresh');
    const headers = storedRefresh ? { 'X-Refresh-Token': storedRefresh } : {};
    try {
        const res = await fetch('/api/auth/refresh', { method: 'POST', headers });
        if (!res.ok) {
            accessToken = '';
            localStorage.removeItem('thg_token');
            localStorage.removeItem('thg_refresh');
            localStorage.removeItem('thg_user');
            return false;
        }
        const data = await res.json().catch(() => null);
        if (!data?.access_token) return false;
        accessToken = data.access_token;
        localStorage.setItem('thg_token', accessToken);
        if (data.refresh_token) localStorage.setItem('thg_refresh', data.refresh_token);
        const savedUser = localStorage.getItem('thg_user');
        if (savedUser) updateSidebarUser(JSON.parse(savedUser));
        return true;
    } catch {
        // Network error: fall back to localStorage token if still valid
        if (accessToken) {
            try {
                const payload = JSON.parse(atob(accessToken.split('.')[1]));
                if (payload.exp * 1000 > Date.now() + 10000) {
                    const savedUser = localStorage.getItem('thg_user');
                    if (savedUser) updateSidebarUser(JSON.parse(savedUser));
                    return true;
                }
            } catch { }
        }
        accessToken = '';
        localStorage.removeItem('thg_token');
        localStorage.removeItem('thg_refresh');
        localStorage.removeItem('thg_user');
        return false;
    }
}

// ===== Init =====
document.addEventListener('DOMContentLoaded', async () => {
    // Show login immediately — hide only after session is confirmed
    showLogin();
    await loadSystemInfo();

    const restored = await tryRestoreSession();
    if (restored) {
        hideLogin();
        loadDashboard();
        loadNicheTabs();
        if (refreshInterval) clearInterval(refreshInterval);
        refreshInterval = setInterval(() => { if (!document.hidden) refreshData(); }, 15000);
    }
    // If restore failed, login form stays visible — user must log in manually
});
