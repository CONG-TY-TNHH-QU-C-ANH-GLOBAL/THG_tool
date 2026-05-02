import { useState, useEffect, lazy, Suspense } from 'react';
import type { Organization } from '../types';
import { Avatar, Badge, Row } from './ui';
import { theme, rootStyle } from '../constants/styles';
import { get } from '../services/api';
import SettingsPage from './SettingsPage';
import {
  Users, Globe, MessageSquare, FileText, MessageCircle,
  Trophy, Database, Settings, Zap, Bell, Bot,
} from 'lucide-react';

const LeadsView      = lazy(() => import('./views/LeadsView'));
const WorkspaceChatView = lazy(() => import('./views/WorkspaceChatView'));
const BrowserView    = lazy(() => import('./views/BrowserView'));
const InboxView      = lazy(() => import('./views/InboxView'));
const PostingView    = lazy(() => import('./views/PostingView'));
const CommentingView = lazy(() => import('./views/CommentingView'));
const LeaderboardView = lazy(() => import('./views/LeaderboardView'));
const DataPrivateView = lazy(() => import('./views/DataPrivateView'));

type Tab = 'leads' | 'chat' | 'browser' | 'inbox' | 'posting' | 'commenting' | 'leaderboard' | 'data' | 'settings';

interface MainAppProps {
  role: 'admin' | 'staff';
  goLanding: () => void;
}

interface NavItem { id: Tab; l: string; I: React.ComponentType<{ size?: number | string }>; badge?: number; }
interface ThreadsBadgeResponse {
  threads?: Array<{ unread_count?: number }>;
  unread_count?: number;
}

const ADMIN_TABS: NavItem[] = [
  { id: 'leads',       l: 'Leads',        I: Users },
  { id: 'chat',        l: 'Chat',         I: Bot },
  { id: 'browser',     l: 'Browser',      I: Globe },
  { id: 'inbox',       l: 'Inbox',        I: MessageSquare },
  { id: 'posting',     l: 'Posting',      I: FileText },
  { id: 'commenting',  l: 'Commenting',   I: MessageCircle },
  { id: 'leaderboard', l: 'Leaderboard',  I: Trophy },
  { id: 'data',        l: 'Data Private', I: Database },
];

const STAFF_TABS: NavItem[] = [
  { id: 'leads',       l: 'My Leads',     I: Users },
  { id: 'chat',        l: 'Chat',         I: Bot },
  { id: 'inbox',       l: 'Inbox',        I: MessageSquare },
  { id: 'leaderboard', l: 'Leaderboard',  I: Trophy },
  { id: 'data',        l: 'Data Private', I: Database },
];

const TAB_LABELS: Record<Tab, string> = {
  leads: 'Leads', chat: 'Chat', browser: 'Browser', inbox: 'Inbox', posting: 'Posting',
  commenting: 'Commenting', leaderboard: 'Leaderboard', data: 'Data Private', settings: 'Settings',
};

const Spinner = () => (
  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: 200 }}>
    <div style={{ width: 24, height: 24, border: `3px solid ${theme.border}`, borderTopColor: theme.primary, borderRadius: '50%', animation: 'spin 0.7s linear infinite' }} />
  </div>
);

function makeAbbr(name: string): string {
  const words = name.trim().split(/\s+/);
  if (words.length >= 2) return (words[0][0] + words[1][0]).toUpperCase();
  return name.slice(0, 2).toUpperCase();
}

const ORG_COLORS = [theme.primary, theme.blue, theme.green, theme.yellow, theme.red, theme.primaryLight];

export default function MainApp({ role, goLanding }: MainAppProps) {
  const [tab, setTab] = useState<Tab>('leads');
  const [org, setOrg] = useState<Organization>({ id: 0, name: '...', abbr: '..', plan: 'Starter', color: theme.primary });
  const [inboxBadge, setInboxBadge] = useState(0);

  useEffect(() => {
    get<{ org: { id: number; name: string; plan_tier: string } }>('/org').then(res => {
      if (!res.org) return;
      const { id, name, plan_tier } = res.org;
      const planMap: Record<string, Organization['plan']> = { free: 'Starter', pro: 'Pro', enterprise: 'Enterprise' };
      setOrg({
        id,
        name,
        abbr: makeAbbr(name),
        plan: planMap[plan_tier] ?? 'Starter',
        color: ORG_COLORS[id % ORG_COLORS.length],
      });
    }).catch(() => {});
  }, []);

  const isAdmin = role === 'admin';
  const orgId = String(org.id);
  useEffect(() => {
    if (org.id <= 0) {
      setInboxBadge(0);
      return;
    }

    let cancelled = false;
    const loadInboxBadge = async () => {
      try {
        const res = await get<ThreadsBadgeResponse>('/threads');
        const unread = typeof res.unread_count === 'number'
          ? res.unread_count
          : (res.threads ?? []).reduce((sum, t) => sum + Math.max(0, Number(t.unread_count ?? 0) || 0), 0);
        if (!cancelled) setInboxBadge(unread);
      } catch {
        if (!cancelled) setInboxBadge(0);
      }
    };

    void loadInboxBadge();
    const timer = window.setInterval(() => void loadInboxBadge(), 30_000);
    const handleThreadsUpdated = () => void loadInboxBadge();
    window.addEventListener('autoflow:threads-updated', handleThreadsUpdated);
    return () => {
      cancelled = true;
      window.removeEventListener('autoflow:threads-updated', handleThreadsUpdated);
      window.clearInterval(timer);
    };
  }, [org.id]);

  const tabs = (isAdmin ? ADMIN_TABS : STAFF_TABS).map(item => (
    item.id === 'inbox' ? { ...item, badge: inboxBadge } : item
  ));

  const renderView = () => {
    switch (tab) {
      case 'leads':       return <LeadsView orgId={orgId} isAdmin={isAdmin} />;
      case 'chat':        return <WorkspaceChatView orgId={orgId} />;
      case 'browser':     return <BrowserView orgId={orgId} />;
      case 'inbox':       return <InboxView orgId={orgId} />;
      case 'posting':     return <PostingView orgId={orgId} />;
      case 'commenting':  return <CommentingView orgId={orgId} />;
      case 'leaderboard': return <LeaderboardView orgId={orgId} isAdmin={isAdmin} />;
      case 'data':        return <DataPrivateView orgId={orgId} />;
      case 'settings':    return <SettingsPage org={org} orgId={orgId} isAdmin={isAdmin} />;
      default:            return null;
    }
  };

  return (
    <div style={{ ...rootStyle, display: 'flex', height: '100vh', overflow: 'hidden' }}>
      {/* Sidebar */}
      <aside className="af-glass" style={{ width: 214, borderRight: `1px solid ${theme.border}`, borderTop: 0, borderLeft: 0, borderBottom: 0, borderRadius: 0, display: 'flex', flexDirection: 'column', flexShrink: 0, boxShadow: '12px 0 46px rgba(0,0,0,0.18)' }}>
        {/* Logo */}
        <div style={{ padding: '18px 16px', borderBottom: `1px solid ${theme.borderAlt}` }}>
          <Row style={{ gap: 8 }}>
            <div style={{ width: 32, height: 32, background: `linear-gradient(135deg, ${theme.primary}, ${theme.primaryLight})`, borderRadius: 8, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0, boxShadow: '0 14px 30px rgba(24, 86, 255, 0.28)' }}>
              <Zap size={14} color="#fff" />
            </div>
            <span style={{ fontWeight: 850, fontSize: 14, color: theme.textWhite }}>AutoFlow</span>
          </Row>
        </div>

        {/* Org switcher */}
        <div style={{ padding: '10px 10px 4px' }}>
          <p style={{ color: theme.textFaint, fontSize: 10, fontWeight: 800, letterSpacing: '0.07em', marginBottom: 6, paddingLeft: 4, fontFamily: '"JetBrains Mono", ui-monospace, monospace' }}>TỔ CHỨC</p>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '9px 10px', border: `1px solid ${theme.borderAlt}`, borderRadius: 8, background: theme.surfaceAlt }}>
            <div style={{ width: 26, height: 26, background: `linear-gradient(135deg, ${org.color}, ${theme.primaryLight})`, borderRadius: 8, display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#fff', fontSize: 10, fontWeight: 850, flexShrink: 0 }}>{org.abbr}</div>
            <span style={{ color: theme.text, fontSize: 12, fontWeight: 500, flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{org.name}</span>
            <Badge label={org.plan} />
          </div>
        </div>

        {/* Nav */}
        <nav style={{ flex: 1, padding: '8px 10px', overflowY: 'auto' }}>
          <p style={{ color: theme.textFaint, fontSize: 10, fontWeight: 800, letterSpacing: '0.07em', marginBottom: 6, paddingLeft: 4, fontFamily: '"JetBrains Mono", ui-monospace, monospace' }}>MENU</p>
          {tabs.map(({ id, l, I, badge }) => (
            <button key={id} onClick={() => setTab(id)} style={{
              width: '100%', display: 'flex', alignItems: 'center', gap: 8,
              padding: '9px 10px', borderRadius: 8, border: `1px solid ${tab === id ? 'rgba(255,255,255,0.22)' : 'transparent'}`, cursor: 'pointer', marginBottom: 4,
              background: tab === id ? `linear-gradient(135deg, ${theme.primary}, ${theme.primaryDark})` : 'transparent',
              color: tab === id ? '#fff' : theme.textMuted,
              boxShadow: tab === id ? '0 14px 30px rgba(24, 86, 255, 0.22)' : 'none',
            }}>
              <I size={14} />
              <span style={{ fontSize: 13, flex: 1, textAlign: 'left' }}>{l}</span>
              {badge != null && badge > 0 && (
                <span style={{ background: tab === id ? '#ffffff33' : theme.primary, color: '#fff', fontSize: 10, fontWeight: 800, minWidth: 17, height: 17, borderRadius: '50%', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: '0 3px' }}>{badge}</span>
              )}
            </button>
          ))}
        </nav>

        {/* Settings + user */}
        <div style={{ padding: '10px', borderTop: `1px solid ${theme.borderAlt}` }}>
          <button onClick={() => setTab('settings')} style={{ width: '100%', display: 'flex', alignItems: 'center', gap: 8, padding: '9px 10px', borderRadius: 8, border: `1px solid ${tab === 'settings' ? 'rgba(255,255,255,0.22)' : 'transparent'}`, cursor: 'pointer', marginBottom: 8, background: tab === 'settings' ? `linear-gradient(135deg, ${theme.primary}, ${theme.primaryDark})` : 'transparent', color: tab === 'settings' ? '#fff' : theme.textMuted }}>
            <Settings size={14} /><span style={{ fontSize: 13 }}>Settings</span>
          </button>
          <Row style={{ gap: 8, padding: '6px 4px' }}>
            <Avatar text={isAdmin ? 'A' : 'S'} size={26} />
            <div style={{ flex: 1, minWidth: 0 }}>
              <p style={{ color: theme.text, fontSize: 12, fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{isAdmin ? 'Admin' : 'Staff'}</p>
              <button onClick={goLanding} style={{ background: 'none', border: 'none', color: theme.textFaint, fontSize: 10, cursor: 'pointer', padding: 0 }}>Đăng xuất</button>
            </div>
          </Row>
        </div>
      </aside>

      {/* Main content */}
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
        {/* Topbar */}
        <header className="af-glass" style={{ display: 'flex', alignItems: 'center', padding: '13px 20px', borderTop: 0, borderLeft: 0, borderRight: 0, borderBottom: `1px solid ${theme.border}`, borderRadius: 0, flexShrink: 0, boxShadow: '0 12px 40px rgba(0,0,0,0.16)' }}>
          <div>
            <p style={{ color: theme.text, fontWeight: 800, fontSize: 15 }}>{TAB_LABELS[tab]}</p>
            <p style={{ color: theme.textFaint, fontSize: 11 }}>{org.name} · {isAdmin ? 'Admin' : 'Staff'}</p>
          </div>
          <Row style={{ gap: 10, marginLeft: 'auto' }}>
            <button style={{ background: theme.surfaceAlt, border: `1px solid ${theme.border}`, borderRadius: 8, padding: '7px 9px', cursor: 'pointer', position: 'relative' }}>
              <Bell size={15} color={theme.textMuted} />
              <span style={{ position: 'absolute', top: 4, right: 4, width: 7, height: 7, background: theme.red, borderRadius: '50%' }} />
            </button>
          </Row>
        </header>

        {/* View content */}
        <main style={{ flex: 1, overflowY: 'auto', padding: 18 }}>
          <Suspense fallback={<Spinner />}>
            {renderView()}
          </Suspense>
        </main>
      </div>
    </div>
  );
}
