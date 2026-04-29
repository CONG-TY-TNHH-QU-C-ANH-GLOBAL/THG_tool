import { useState, useEffect } from 'react';
import { theme, cardStyle, secondaryBtn } from '../constants/styles';
import { Avatar, Badge, Row } from './ui';
import { Shield, RefreshCw, Database, Trash2, LogOut } from 'lucide-react';
import {
  getOrgs, getAccounts, getUsers, getSessions, runQuery,
  deleteOrg, deleteAccount, deleteUser, terminateSession,
  SAOrg, SAAccount, SAUser, SASession, QueryResult,
} from '../services/superadminService';
import { useAuthStore } from '../stores/authStore';
import { post } from '../services/api';

interface SuperAdminProps { goBack: () => void; }

type AdminTab = 'orgs' | 'accounts' | 'sessions' | 'users' | 'query';

const TAB_LABELS: Record<AdminTab, string> = {
  orgs: 'Tổ chức',
  accounts: 'Accounts',
  sessions: 'Sessions',
  users: 'Users',
  query: 'SQL Query',
};

export default function SuperAdmin({ goBack }: SuperAdminProps) {
  const [tab, setTab] = useState<AdminTab>('orgs');
  const [orgs, setOrgs] = useState<SAOrg[]>([]);
  const [accounts, setAccounts] = useState<SAAccount[]>([]);
  const [users, setUsers] = useState<SAUser[]>([]);
  const [sessions, setSessions] = useState<SASession[]>([]);
  const [loading, setLoading] = useState(true);
  const [sql, setSql] = useState('');
  const [queryResult, setQueryResult] = useState<QueryResult | null>(null);
  const [queryLoading, setQueryLoading] = useState(false);
  const [queryError, setQueryError] = useState('');
  const [deleting, setDeleting] = useState<number | null>(null);

  useEffect(() => {
    Promise.all([getOrgs(), getAccounts(), getSessions(), getUsers()])
      .then(([o, a, s, u]) => { setOrgs(o); setAccounts(a); setSessions(s); setUsers(u); })
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  const handleLogout = async () => {
    try { await post('/auth/logout', {}); } catch {}
    useAuthStore.getState().setToken(null);
    useAuthStore.getState().setUser(null);
    goBack();
  };

  const handleQuery = async () => {
    setQueryLoading(true); setQueryError('');
    try { setQueryResult(await runQuery(sql)); }
    catch (e) { setQueryError(String(e)); }
    finally { setQueryLoading(false); }
  };

  const handleDeleteOrg = async (id: number) => {
    if (!confirm('Xoá tổ chức này? Hành động không thể hoàn tác.')) return;
    setDeleting(id);
    try { await deleteOrg(id); setOrgs(prev => prev.filter(o => o.id !== id)); } catch {}
    setDeleting(null);
  };
  const handleDeleteAccount = async (id: number) => {
    if (!confirm('Xoá tài khoản Facebook này?')) return;
    setDeleting(id);
    try { await deleteAccount(id); setAccounts(prev => prev.filter(a => a.id !== id)); } catch {}
    setDeleting(null);
  };
  const handleDeleteUser = async (id: number) => {
    if (!confirm('Xoá user này?')) return;
    setDeleting(id);
    try { await deleteUser(id); setUsers(prev => prev.filter(u => u.id !== id)); } catch {}
    setDeleting(null);
  };
  const handleTerminateSession = async (accountId: number) => {
    if (!confirm('Terminate session này?')) return;
    setDeleting(accountId);
    try { await terminateSession(accountId); setSessions(prev => prev.filter(s => s.account_id !== accountId)); } catch {}
    setDeleting(null);
  };

  const stats = [
    { label: 'Tổ chức', value: orgs.length, color: '#818cf8' },
    { label: 'FB Accounts', value: accounts.length, color: '#4ade80' },
    { label: 'Sessions live', value: sessions.filter(s => s.status === 'active').length, color: '#38bdf8' },
    { label: 'Users', value: users.length, color: '#fbbf24' },
  ];

  const dot = (active: boolean) => (
    <span style={{ display: 'inline-block', width: 8, height: 8, borderRadius: '50%', background: active ? '#4ade80' : '#ef4444' }} />
  );

  const tw = { background: theme.surface, border: `1px solid ${theme.border}`, borderRadius: 12, overflow: 'hidden' };
  const th: React.CSSProperties = { padding: '10px 14px', textAlign: 'left', color: theme.textFaint, fontWeight: 500, fontSize: 12 };
  const td: React.CSSProperties = { padding: '10px 14px', color: theme.text, fontSize: 13, borderBottom: `1px solid ${theme.border}` };
  const delBtn: React.CSSProperties = { background: 'none', border: 'none', cursor: 'pointer', color: '#f87171', padding: '2px 6px', borderRadius: 4, display: 'flex', alignItems: 'center' };

  return (
    <div style={{ background: theme.bg, color: theme.text, fontFamily: 'system-ui, sans-serif', minHeight: '100vh' }}>
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'center', padding: '13px 24px', background: theme.surfaceAlt, borderBottom: `1px solid ${theme.border}` }}>
        <Row style={{ gap: 10 }}>
          <div style={{ width: 30, height: 30, background: '#dc2626', borderRadius: 8, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <Shield size={14} color="#fff" />
          </div>
          <span style={{ fontWeight: 700, fontSize: 14, color: '#fff' }}>AutoFlow Admin Portal</span>
          <span style={{ background: '#dc262622', color: '#f87171', border: '1px solid #dc262644', fontSize: 10, padding: '2px 8px', borderRadius: 99 }}>Super Admin</span>
        </Row>
        <button
          onClick={() => void handleLogout()}
          style={{ ...secondaryBtn({ padding: '6px 14px', fontSize: 12 }), marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 5, color: '#f87171' }}
        >
          <LogOut size={13} /> Đăng xuất
        </button>
        <button onClick={goBack} style={{ ...secondaryBtn({ padding: '6px 14px', fontSize: 12 }), marginLeft: 8 }}>← Trang chủ</button>
      </div>

      <div style={{ padding: 22 }}>
        {/* Stats */}
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4,1fr)', gap: 12, marginBottom: 22 }}>
          {stats.map(s => (
            <div key={s.label} style={cardStyle()}>
              <p style={{ color: theme.textFaint, fontSize: 11 }}>{s.label}</p>
              <p style={{ fontSize: 24, fontWeight: 800, color: s.color, marginTop: 4 }}>{s.value}</p>
            </div>
          ))}
        </div>

        {/* Tab bar */}
        <Row style={{ gap: 8, marginBottom: 18 }}>
          {(Object.keys(TAB_LABELS) as AdminTab[]).map(t => (
            <button key={t} onClick={() => setTab(t)} style={{
              padding: '7px 16px', borderRadius: 8, border: 'none', cursor: 'pointer', fontSize: 13,
              background: tab === t ? theme.primary : theme.surface,
              color: tab === t ? '#fff' : theme.textMuted,
            }}>
              {TAB_LABELS[t]}
            </button>
          ))}
        </Row>

        {/* Loading */}
        {loading && (
          <div style={{ width: 24, height: 24, border: `3px solid ${theme.border}`, borderTopColor: theme.primary, borderRadius: '50%', animation: 'spin 0.7s linear infinite', margin: '40px auto' }} />
        )}

        {!loading && (
          <>
            {/* ORGS */}
            {tab === 'orgs' && (
              <div style={tw}>
                <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                  <thead>
                    <tr style={{ borderBottom: `1px solid ${theme.border}` }}>
                      {['ID', 'Tên', 'Domain', 'Gói', 'Max Accounts', 'Active', 'Ngày tạo', ''].map(h => (
                        <th key={h} style={th}>{h}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {orgs.map(o => (
                      <tr key={o.id}>
                        <td style={td}>{o.id}</td>
                        <td style={td}><Row style={{ gap: 8 }}><Avatar text={o.name[0] ?? '?'} size={24} /><span>{o.name}</span></Row></td>
                        <td style={td}>{o.domain}</td>
                        <td style={td}><Badge label={o.plan_tier} /></td>
                        <td style={td}>{o.max_accounts}</td>
                        <td style={td}>{dot(o.active)}</td>
                        <td style={{ ...td, color: theme.textFaint }}>{o.created_at.slice(0, 10)}</td>
                        <td style={td}>
                          <button style={delBtn} disabled={deleting === o.id} onClick={() => void handleDeleteOrg(o.id)}>
                            <Trash2 size={13} />
                          </button>
                        </td>
                      </tr>
                    ))}
                    {orgs.length === 0 && <tr><td colSpan={8} style={{ ...td, textAlign: 'center', color: theme.textFaint }}>Không có dữ liệu</td></tr>}
                  </tbody>
                </table>
              </div>
            )}

            {/* ACCOUNTS */}
            {tab === 'accounts' && (
              <div style={tw}>
                <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                  <thead>
                    <tr style={{ borderBottom: `1px solid ${theme.border}` }}>
                      {['ID', 'Org ID', 'Tên', 'Platform', 'Email', 'Status', 'Đăng nhập FB', 'Ngày tạo', ''].map(h => (
                        <th key={h} style={th}>{h}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {accounts.map(a => (
                      <tr key={a.id}>
                        <td style={td}>{a.id}</td>
                        <td style={td}>{a.org_id}</td>
                        <td style={td}>{a.name}</td>
                        <td style={td}>{a.platform}</td>
                        <td style={td}>{a.email}</td>
                        <td style={td}><Badge label={a.status} /></td>
                        <td style={td}>{a.browser_logged_in ? <span style={{ color: '#4ade80' }}>✓</span> : <span style={{ color: theme.textFaint }}>-</span>}</td>
                        <td style={{ ...td, color: theme.textFaint }}>{a.created_at.slice(0, 10)}</td>
                        <td style={td}>
                          <button style={delBtn} disabled={deleting === a.id} onClick={() => void handleDeleteAccount(a.id)}>
                            <Trash2 size={13} />
                          </button>
                        </td>
                      </tr>
                    ))}
                    {accounts.length === 0 && <tr><td colSpan={9} style={{ ...td, textAlign: 'center', color: theme.textFaint }}>Không có dữ liệu</td></tr>}
                  </tbody>
                </table>
              </div>
            )}

            {/* SESSIONS */}
            {tab === 'sessions' && (
              <div style={tw}>
                <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                  <thead>
                    <tr style={{ borderBottom: `1px solid ${theme.border}` }}>
                      {['Account ID', 'Org ID', 'Status', 'CDP Port', 'VNC Port', 'Bắt đầu', 'Hoạt động', ''].map(h => (
                        <th key={h} style={th}>{h}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {sessions.map((s, i) => (
                      <tr key={i}>
                        <td style={td}>{s.account_id}</td>
                        <td style={td}>{s.org_id}</td>
                        <td style={td}><Badge label={s.status} /></td>
                        <td style={td}>{s.cdp_port === 0 ? '-' : s.cdp_port}</td>
                        <td style={td}>{s.vnc_port === 0 ? '-' : s.vnc_port}</td>
                        <td style={{ ...td, color: theme.textFaint }}>{s.started_at.slice(0, 16)}</td>
                        <td style={{ ...td, color: theme.textFaint }}>{s.last_active_at.slice(0, 16)}</td>
                        <td style={td}>
                          <button style={delBtn} disabled={deleting === s.account_id} onClick={() => void handleTerminateSession(s.account_id)}>
                            <Trash2 size={13} />
                          </button>
                        </td>
                      </tr>
                    ))}
                    {sessions.length === 0 && <tr><td colSpan={8} style={{ ...td, textAlign: 'center', color: theme.textFaint }}>Không có phiên đang chạy</td></tr>}
                  </tbody>
                </table>
              </div>
            )}

            {/* USERS */}
            {tab === 'users' && (
              <div style={tw}>
                <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                  <thead>
                    <tr style={{ borderBottom: `1px solid ${theme.border}` }}>
                      {['ID', 'Org ID', 'Tên', 'Email', 'Role', 'Active', ''].map(h => (
                        <th key={h} style={th}>{h}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {users.map(u => (
                      <tr key={u.id}>
                        <td style={td}>{u.id}</td>
                        <td style={td}>{u.org_id}</td>
                        <td style={td}><Row style={{ gap: 8 }}><Avatar text={u.name[0] ?? '?'} size={24} /><span>{u.name}</span></Row></td>
                        <td style={td}>{u.email}</td>
                        <td style={td}><Badge label={u.role} /></td>
                        <td style={td}>{dot(u.active)}</td>
                        <td style={td}>
                          <button style={delBtn} disabled={deleting === u.id} onClick={() => void handleDeleteUser(u.id)}>
                            <Trash2 size={13} />
                          </button>
                        </td>
                      </tr>
                    ))}
                    {users.length === 0 && <tr><td colSpan={7} style={{ ...td, textAlign: 'center', color: theme.textFaint }}>Không có dữ liệu</td></tr>}
                  </tbody>
                </table>
              </div>
            )}

            {/* QUERY */}
            {tab === 'query' && (
              <div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
                  <Database size={14} color={theme.textFaint} />
                  <span style={{ color: theme.textFaint, fontSize: 12 }}>Chỉ cho phép câu lệnh SELECT</span>
                </div>
                <textarea
                  value={sql}
                  onChange={e => setSql(e.target.value)}
                  placeholder="SELECT * FROM users LIMIT 10"
                  style={{ width: '100%', height: 100, background: theme.surfaceAlt, border: `1px solid ${theme.border}`, borderRadius: 8, color: theme.text, padding: 10, fontSize: 13, fontFamily: 'monospace', resize: 'vertical', boxSizing: 'border-box' }}
                />
                <button
                  onClick={() => void handleQuery()}
                  disabled={queryLoading}
                  style={{ marginTop: 10, padding: '8px 20px', background: theme.primary, border: 'none', borderRadius: 8, color: '#fff', fontSize: 13, cursor: queryLoading ? 'wait' : 'pointer', opacity: queryLoading ? 0.6 : 1, display: 'flex', alignItems: 'center', gap: 6 }}
                >
                  {queryLoading && <RefreshCw size={13} />}
                  {queryLoading ? 'Đang chạy...' : 'Chạy Query'}
                </button>
                {queryError && <p style={{ color: '#f87171', fontSize: 13, marginTop: 10 }}>{queryError}</p>}
                {queryResult && (
                  <div style={{ marginTop: 14 }}>
                    <p style={{ color: theme.textFaint, fontSize: 12, marginBottom: 8 }}>{queryResult.count} dòng kết quả</p>
                    <div style={{ overflowX: 'auto' }}>
                      <table style={{ width: '100%', borderCollapse: 'collapse', background: theme.surface, border: `1px solid ${theme.border}`, borderRadius: 12 }}>
                        <thead>
                          <tr style={{ borderBottom: `1px solid ${theme.border}` }}>
                            {queryResult.columns.map(col => <th key={col} style={th}>{col}</th>)}
                          </tr>
                        </thead>
                        <tbody>
                          {queryResult.rows.map((row, i) => (
                            <tr key={i}>
                              {queryResult.columns.map(col => <td key={col} style={td}>{String(row[col] ?? '')}</td>)}
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </div>
                )}
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}
