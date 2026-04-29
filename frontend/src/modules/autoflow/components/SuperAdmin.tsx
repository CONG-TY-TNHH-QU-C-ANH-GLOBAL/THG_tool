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
const TAB_LABELS: Record<AdminTab, string> = { orgs: 'Tổ chức', accounts: 'Accounts', sessions: 'Sessions', users: 'Users', query: 'SQL Query' };

const SuperAdmin = ({ goBack }: SuperAdminProps) => {
  const [deleting, setDeleting] = useState<number | null>(null);
  const [orgs, setOrgs] = useState<SAOrg[]>([]);
  const [accounts, setAccounts] = useState<SAAccount[]>([]);
  const [users, setUsers] = useState<SAUser[]>([]);
  const [sessions, setSessions] = useState<SASession[]>([]);

  const handleLogout = async () => {
    try { await post('/auth/logout', {}); } catch {}
    useAuthStore.getState().setToken(null);
    useAuthStore.getState().setUser(null);
    goBack();
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

  return (
    <div>
      <header>
        <button onClick={() => void handleLogout()} style={{ ...secondaryBtn({ padding: '6px 14px', fontSize: 12 }), display: 'flex', alignItems: 'center', gap: 5, color: '#f87171', marginLeft: 'auto' }}>
          <LogOut size={13} /> Đăng xuất
        </button>
        <button onClick={goBack} style={{ ...secondaryBtn({ padding: '6px 14px', fontSize: 12 }), marginLeft: 8 }}>← Trang chủ</button>
      </header>
      {/* Other components and logic for rendering tables and data */}
    </div>
  );
};

export default SuperAdmin;