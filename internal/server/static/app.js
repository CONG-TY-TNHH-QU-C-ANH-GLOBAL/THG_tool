// ===== THG Agentic Scraper — Dashboard JS v2 =====

const API = '';
let currentPage = 'dashboard';
let refreshInterval = null;
let activeNicheFilter = '';
let accessToken = localStorage.getItem('thg_token') || '';

// ===== Auth =====

function showLogin() {
    document.getElementById('loginPage').style.display = 'flex';
    document.getElementById('loginEmail').focus();
}

function hideLogin() {
    document.getElementById('loginPage').style.display = 'none';
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
        if (data.user) {
            localStorage.setItem('thg_user', JSON.stringify(data.user));
            updateSidebarUser(data.user);
        }
        hideLogin();
        loadDashboard();
        loadNicheTabs();
    } catch {
        errEl.textContent = 'Lỗi kết nối server';
        errEl.style.display = 'block';
    } finally {
        btn.disabled = false;
        btn.textContent = 'Đăng nhập';
    }
}

function doLogout() {
    fetch('/api/auth/logout', { method: 'POST', headers: { 'Authorization': `Bearer ${accessToken}` } }).catch(() => { });
    accessToken = '';
    localStorage.removeItem('thg_token');
    localStorage.removeItem('thg_user');
    showLogin();
}

function updateSidebarUser(user) {
    const el = document.getElementById('sidebarUser');
    if (el && user) el.textContent = `${user.name || user.email} (${user.role})`;
}

// ===== Settings Page =====

async function loadSettingsPage() {
    const user = await fetchAPI('/api/auth/me');
    if (!user) return;
    localStorage.setItem('thg_user', JSON.stringify(user));
    updateSidebarUser(user);
    document.getElementById('profileName').value = user.name || '';
    document.getElementById('profileEmail').value = user.email || '';
    document.getElementById('profileRole').value = user.role === 'admin' ? 'Admin' : 'Sales';
    if (user.role === 'admin') {
        document.getElementById('userMgmtSection').style.display = '';
        loadUsersTable();
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

// ===== Page Navigation =====

function switchPage(page) {
    currentPage = page;
    document.querySelectorAll('.nav-item').forEach(el => el.classList.remove('active'));
    document.querySelector(`[data-page="${page}"]`).classList.add('active');
    document.querySelectorAll('.page').forEach(el => el.classList.remove('active'));
    document.getElementById(`page-${page}`).classList.add('active');

    const titles = {
        dashboard: ['Dashboard', 'Real-time overview'],
        leads: ['Leads', 'AI-classified leads theo từng lĩnh vực'],
        posts: ['Posts', 'Scraped social media posts'],
        groups: ['Groups', 'Managed social media groups'],
        jobs: ['Jobs', 'Scraping job queue'],
        accounts: ['Accounts', 'Facebook account management'],
        aichat: ['AI Chat', 'Gửi prompt để AI agents thực thi'],
        outbox: ['Outbox', 'Auto-comment & auto-inbox queue'],
        settings: ['Settings', 'Tài khoản và cài đặt hệ thống'],
    };
    document.getElementById('pageTitle').textContent = titles[page][0];
    document.getElementById('pageSubtitle').textContent = titles[page][1];
    if (page === 'leads') loadNicheTabs();
    if (page === 'settings') loadSettingsPage();
    refreshData();
}

// ===== Data Loading =====

async function refreshData() {
    try {
        switch (currentPage) {
            case 'dashboard': await loadDashboard(); break;
            case 'leads': await loadLeads(); break;
            case 'posts': await loadPosts(); break;
            case 'groups': await loadGroups(); break;
            case 'jobs': await loadJobs(); break;
            case 'accounts': await loadAccounts(); break;
            case 'aichat': await loadPromptHistory(); break;
            case 'outbox': await loadOutbox(); break;
        }
    } catch (e) { console.error('Refresh error:', e); }
}

async function loadDashboard() {
    const [stats, leadsRes] = await Promise.all([
        fetchAPI('/api/stats'),
        fetchAPI('/api/leads?limit=5&score=hot'),
    ]);
    if (stats) {
        document.getElementById('statGroups').textContent = stats.active_groups || 0;
        document.getElementById('statPosts').textContent = stats.total_posts || 0;
        document.getElementById('statLeads').textContent = stats.total_leads || 0;
        document.getElementById('statHotLeads').textContent = stats.hot_leads || 0;
        document.getElementById('statTodayPosts').textContent = stats.today_posts || 0;
        document.getElementById('statRunning').textContent = stats.running_jobs || 0;
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
        `<button class="niche-tab" data-niche="${esc(n.slug)}" onclick="setActiveNiche(this,'${esc(n.slug)}')">${esc(n.emoji)} ${esc(n.name)}</button>`
    ).join('');
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
                <button class="btn btn-sm" style="background:rgba(139,92,246,0.15);color:#a78bfa;border:1px solid rgba(139,92,246,0.3)" title="Đăng nhập Chrome Profile" onclick="startChromeLogin(${acc.id},'${esc(acc.name)}')">🖥️ Login</button>
                ${isAdmin ? `<button class="btn btn-sm btn-ghost" title="Cập nhật cookie thủ công" onclick="showUpdateCookieModal(${acc.id})">🔄</button>` : ''}
                ${isAdmin ? `<button class="btn btn-sm btn-ghost" title="${acc.status === 'active' ? 'Tạm dừng' : 'Kích hoạt'}" onclick="toggleAccountStatus(${acc.id}, '${acc.status === 'active' ? 'inactive' : 'active'}')">${acc.status === 'active' ? '⏸' : '▶️'}</button>` : ''}
                ${isAdmin ? `<button class="btn btn-sm btn-danger" title="Xóa tài khoản" onclick="deleteAccount(${acc.id})">🗑</button>` : ''}
            </td>
        `;
    });
}

function showAddAccountModal() {
    document.getElementById('addAccountModal').classList.add('active');
    ['accName', 'accEmail', 'accCookies', 'accProxy', 'accNotes'].forEach(id => document.getElementById(id).value = '');
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

    const res = await fetchAPI('/api/accounts', 'POST', data);
    if (res && res.account_id) {
        showToast('Đã tạo tài khoản, đang khởi động Chrome...', 'success');
        closeAccountModal();
        loadAccounts();
        startChromeLogin(res.account_id, data.name);
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

// ===== Chrome Profile Login =====

let chromeLoginPoll = null;
let chromeLoginAccId = null;

async function startChromeLogin(id, name) {
    chromeLoginAccId = id;
    document.getElementById('chromeAccName').textContent = name;
    document.getElementById('chromeTunnelCmd').textContent = 'Đang khởi động...';
    document.getElementById('captureSessionBtn').disabled = true;
    document.getElementById('fbUserIdRow').style.display = 'none';
    setChromeStatus('starting');
    document.getElementById('chromeLoginModal').classList.add('active');

    const res = await fetchAPI(`/api/accounts/${id}/start-login`, 'POST', {});
    if (!res) { closeChromeLoginModal(); return; }

    document.getElementById('chromeTunnelCmd').textContent = res.tunnel;

    // Start polling login status every 3 seconds
    if (chromeLoginPoll) clearInterval(chromeLoginPoll);
    chromeLoginPoll = setInterval(pollChromeLoginStatus, 3000);
    setTimeout(pollChromeLoginStatus, 2000); // first check after 2s
}

function setChromeStatus(status, fbUserId) {
    const el = document.getElementById('chromeLoginStatus');
    const textEl = document.getElementById('chromeStatusText');
    const cfgs = {
        starting: { text: 'Chrome đang khởi động...', bg: 'rgba(245,158,11,0.08)', border: 'rgba(245,158,11,0.3)', color: '#fcd34d', icon: '⏳' },
        checking: { text: 'Đang kiểm tra trạng thái...', bg: 'rgba(245,158,11,0.08)', border: 'rgba(245,158,11,0.3)', color: '#fcd34d', icon: '⏳' },
        waiting: { text: 'Chờ đăng nhập Facebook... (hãy làm theo hướng dẫn bên dưới)', bg: 'rgba(245,158,11,0.08)', border: 'rgba(245,158,11,0.3)', color: '#fcd34d', icon: '⏳' },
        logged_in: { text: `Đã đăng nhập thành công! Nhấn "Capture & Save Cookies" để lưu.`, bg: 'rgba(16,185,129,0.08)', border: 'rgba(16,185,129,0.3)', color: '#6ee7b7', icon: '✅' },
        no_session: { text: 'Không có phiên Chrome nào đang chạy.', bg: 'rgba(239,68,68,0.08)', border: 'rgba(239,68,68,0.3)', color: '#fca5a5', icon: '❌' },
    };
    const cfg = cfgs[status] || cfgs.checking;
    el.style.background = cfg.bg;
    el.style.borderColor = cfg.border;
    el.style.color = cfg.color;
    textEl.textContent = `${cfg.icon} ${cfg.text}`;
    if (fbUserId) {
        document.getElementById('fbUserIdRow').style.display = 'block';
        document.getElementById('fbUserIdText').textContent = fbUserId;
    }
}

async function pollChromeLoginStatus() {
    if (!chromeLoginAccId) return;
    const res = await fetchAPI(`/api/accounts/${chromeLoginAccId}/login-status`).catch(() => null);
    if (!res) return;
    setChromeStatus(res.status, res.fb_user_id);
    if (res.logged_in) {
        document.getElementById('captureSessionBtn').disabled = false;
        if (chromeLoginPoll) { clearInterval(chromeLoginPoll); chromeLoginPoll = null; }
    }
}

async function captureChromeSession() {
    if (!chromeLoginAccId) return;
    const btn = document.getElementById('captureSessionBtn');
    btn.disabled = true;
    btn.textContent = '⏳ Đang lưu...';
    const res = await fetchAPI(`/api/accounts/${chromeLoginAccId}/capture-session`, 'POST', {});
    if (res) {
        showToast(`✅ Đã lưu ${res.cookies_count} cookies cho tài khoản FB: ${res.fb_user_id}`, 'success');
        closeChromeLoginModal();
        loadAccounts();
    } else {
        btn.disabled = false;
        btn.textContent = '💾 Capture & Save Cookies';
    }
}

async function stopChromeSession() {
    if (chromeLoginPoll) { clearInterval(chromeLoginPoll); chromeLoginPoll = null; }
    if (chromeLoginAccId) {
        fetchAPI(`/api/accounts/${chromeLoginAccId}/stop-login`, 'POST', {}).catch(() => { });
    }
    closeChromeLoginModal();
}

function closeChromeLoginModal() {
    chromeLoginAccId = null;
    document.getElementById('chromeLoginModal').classList.remove('active');
}

function copyTunnelCmd() {
    const text = document.getElementById('chromeTunnelCmd').textContent;
    navigator.clipboard.writeText(text).then(() => showToast('Đã sao chép lệnh SSH tunnel', 'info'));
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

async function fetchAPI(url, method = 'GET', body = null) {
    try {
        const opts = { method, headers: { 'Content-Type': 'application/json' } };
        if (accessToken) opts.headers['Authorization'] = `Bearer ${accessToken}`;
        if (body) opts.body = JSON.stringify(body);
        const res = await fetch(API + url, opts);
        if (res.status === 401) { showLogin(); return null; }
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        return await res.json();
    } catch (e) { console.error(`API [${method} ${url}]:`, e); return null; }
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

// ===== Init =====
document.addEventListener('DOMContentLoaded', () => {
    if (!accessToken) {
        showLogin();
    } else {
        const savedUser = localStorage.getItem('thg_user');
        if (savedUser) updateSidebarUser(JSON.parse(savedUser));
        loadDashboard();
        loadNicheTabs();
    }
    refreshInterval = setInterval(refreshData, 15000);
});
