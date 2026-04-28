import { useState, useEffect, lazy, Suspense } from 'react';
import type { Organization } from '../types';
import { Avatar, Badge, Row } from './ui';
import { theme, rootStyle } from '../constants/styles';
import { get } from '../services/api';
import SettingsPage from './SettingsPage';
import {
  Users, Globe, MessageSquare, FileText, MessageCircle,
  Trophy, Database, Settings, Zap, Bell,
} from 'lucide-react';

const LeadsView      = lazy(() => import('./views/LeadsView'));
const BrowserView    = lazy(() => import('./views/BrowserView'));
const InboxView      = lazy(() => import('./views/InboxView'));
const PostingView    = lazy(() => import('./views/PostingView'));
const CommentingView = lazy(() => import('./views/CommentingView'));
const LeaderboardView = lazy(() => import('./views/LeaderboardView'));
const DataPrivateView = lazy(() => import('./views/DataPrivateView'));

type Tab = 'leads' | 'browser' | 'inbox' | 'posting' | 'commenting' | 'leaderboard' | 'data' | 'settings';

interface MainAppProps {
  role: 'admin' | 'staff';
  goLanding: () => void;
}

interface NavItem { id: Tab; l: string; I: React.ComponentType<{ size?: number | string }>; badge?: number; }

const ADMIN_TABS: NavItem[] = [
  { id: 'leads',       l: 'Leads',        I: Users },
  { id: 'browser',     l: 'Browser',      I: Globe },
  { id: 'inbox',       l: 'Inbox',        I: MessageSquare, badge: 8 },
  { id: 'posting',     l: 'Posting',      I: FileText },
  { id: 'commenting',  l: 'Commenting',   I: MessageCircle },
  { id: 'leaderboard', l: 'Leaderboard',  I: Trophy },
  { id: 'data',        l: 'Data Private', I: Database },
];

const STAFF_TABS: NavItem[] = [
  { id: 'leads',       l: 'My Leads',     I: Users },
  { id: 'inbox',       l: 'Inbox',        I: MessageSquare, badge: 3 },
  { id: 'leaderboard', l: 'Leaderboard',  I: Trophy },
  { id: 'data',        l: 'Data Private', I: Database },
];

const TAB_LABELS: Record<Tab, string> = {
  leads: 'Leads', browser: 'Browser', inbox: 'Inbox', posting: 'Posting',
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

const ORG_COLORS = ['#4f46e5', '#0ea5e9', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6'];

export default function MainApp({ role, goLanding }: MainAppProps) {
  const [tab, setTab] = useState<Tab>('leads');
  const [org, setOrg] = useState<Organization>({ id: 0, name: '...', abbr: '..', plan: 'Starter', color: '#4f46e5' });

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
  const tabs = isAdmin ? ADMIN_TABS : STAFF_TABS;

  const renderView = () => {
    switch (tab) {
      case 'leads':       return <LeadsView orgId={orgId} isAdmin={isAdmin} />;
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
      <aside style={{ width: 192, background: theme.surfaceAlt, borderRight: `1px solid ${theme.border}`, display: 'flex', flexDirection: 'column', flexShrink: 0 }}>
        {/* Logo */}
        <div style={{ padding: '16px 14px', borderBottom: `1px solid ${theme.border}` }}>
          <Row style={{ gap: 8 }}>
            <div style={{ width: 28, height: 28, background: theme.primary, borderRadius: 7, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
              <Zap size={14} color="#fff" />
            </div>
            <span style={{ fontWeight: 800, fontSize: 14, color: '#fff' }}>AutoFlow</span>
          </Row>
        </div>

        {/* Org switcher */}
        <div style={{ padding: '10px 10px 4px' }}>
          <p style={{ color: theme.textFaint, fontSize: 10, fontWeight: 600, letterSpacing: '0.07em', marginBottom: 6, paddingLeft: 4 }}>TỔ CHỨC</p>
          <div style={{ display: 'flex', alignItems: 'center', gap: 7, padding: '8px 10px' }}>
            <div style={{ width: 24, height: 24, background: org.color, borderRadius: 6, display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#fff', fontSize: 10, fontWeight: 800, flexShrink: 0 }}>{org.abbr}</div>
            <span style={{ color: theme.text, fontSize: 12, fontWeight: 500, flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{org.name}</span>
            <Badge label={org.plan} />
          </div>
        </div>

        {/* Nav */}
        <nav style={{ flex: 1, padding: '8px 10px', overflowY: 'auto' }}>
          <p style={{ color: theme.textFaint, fontSize: 10, fontWeight: 600, letterSpacing: '0.07em', marginBottom: 6, paddingLeft: 4 }}>MENU</p>
          {tabs.map(({ id, l, I, badge }) => (
            <button key={id} onClick={() => setTab(id)} style={{
              width: '100%', display: 'flex', alignItems: 'center', gap: 8,
              padding: '8px 10px', borderRadius: 8, border: 'none', cursor: 'pointer', marginBottom: 2,
              background: tab === id ? theme.primary : 'transparent',
              color: tab === id ? '#fff' : theme.textMuted,
            }}>
              <I size={14} />
              <span style={{ fontSize: 13, flex: 1, textAlign: 'left' }}>{l}</span>
              {badge != null && badge > 0 && (
                <span style={{ background: tab === id ? '#ffffff33' : theme.primary, color: '#fff', fontSize: 10, fontWeight: 700, minWidth: 17, height: 17, borderRadius: '50%', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: '0 3px' }}>{badge}</span>
              )}
            </button>
          ))}
        </nav>

        {/* Settings + user */}
        <div style={{ padding: '8px 10px', borderTop: `1px solid ${theme.border}` }}>
          <button onClick={() => setTab('settings')} style={{ width: '100%', display: 'flex', alignItems: 'center', gap: 8, padding: '8px 10px', borderRadius: 8, border: 'none', cursor: 'pointer', marginBottom: 8, background: tab === 'settings' ? theme.primary : 'transparent', color: tab === 'settings' ? '#fff' : theme.textMuted }}>
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
        <header style={{ display: 'flex', alignItems: 'center', padding: '11px 20px', borderBottom: `1px solid ${theme.border}`, background: theme.surfaceAlt, flexShrink: 0 }}>
          <div>
            <p style={{ color: theme.text, fontWeight: 600, fontSize: 15 }}>{TAB_LABELS[tab]}</p>
            <p style={{ color: theme.textFaint, fontSize: 11 }}>{org.name} · {isAdmin ? 'Admin' : 'Staff'}</p>
          </div>
          <Row style={{ gap: 10, marginLeft: 'auto' }}>
            <button style={{ background: 'none', border: `1px solid ${theme.border}`, borderRadius: 8, padding: '7px 9px', cursor: 'pointer', position: 'relative' }}>
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
