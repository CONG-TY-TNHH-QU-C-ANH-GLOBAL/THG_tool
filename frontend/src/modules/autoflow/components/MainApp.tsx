'use client';

import { useState, useEffect, lazy, Suspense } from 'react';
import type { Organization } from '../types';
import { get } from '../services/api';
import SettingsPage from './SettingsPage';
import { LangSwitch } from './ds/LangSwitch';
import { DensitySwitch } from './ds/DensitySwitch';
import { useLang } from '../i18n/useLang';
import {
  Bell,
  Bot,
  ChevronDown,
  Database,
  FileText,
  Globe,
  LogOut,
  MessageCircle,
  MessageSquare,
  Search,
  Settings as SettingsIcon,
  Trophy,
  Users,
} from 'lucide-react';

const LeadsView = lazy(() => import('./views/LeadsView'));
const WorkspaceChatView = lazy(() => import('./views/WorkspaceChatView'));
const BrowserView = lazy(() => import('./views/BrowserView'));
const InboxView = lazy(() => import('./views/InboxView'));
const PostingView = lazy(() => import('./views/PostingView'));
const CommentingView = lazy(() => import('./views/CommentingView'));
const LeaderboardView = lazy(() => import('./views/LeaderboardView'));
const DataPrivateView = lazy(() => import('./views/DataPrivateView'));

type Tab = 'leads' | 'chat' | 'browser' | 'inbox' | 'posting' | 'commenting' | 'leaderboard' | 'data' | 'settings';

interface MainAppProps {
  role: 'admin' | 'staff';
  goLanding: () => void;
}

interface NavItem {
  id: Tab;
  Icon: React.ComponentType<{ size?: number | string }>;
  badge?: number;
}

interface ThreadsBadgeResponse {
  threads?: Array<{ unread_count?: number }>;
  unread_count?: number;
}

const ADMIN_TABS: NavItem[] = [
  { id: 'leads', Icon: Users },
  { id: 'chat', Icon: Bot },
  { id: 'browser', Icon: Globe },
  { id: 'inbox', Icon: MessageSquare },
  { id: 'posting', Icon: FileText },
  { id: 'commenting', Icon: MessageCircle },
];

const STAFF_TABS: NavItem[] = [
  { id: 'leads', Icon: Users },
  { id: 'chat', Icon: Bot },
  { id: 'inbox', Icon: MessageSquare },
];

const ANALYTICS_TABS: NavItem[] = [
  { id: 'leaderboard', Icon: Trophy },
  { id: 'data', Icon: Database },
];

function makeAbbr(name: string): string {
  const words = name.trim().split(/\s+/);
  if (words.length >= 2) return (words[0][0] + words[1][0]).toUpperCase();
  return name.slice(0, 2).toUpperCase();
}

const Spinner = () => (
  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: 200 }}>
    <div className="skeleton" style={{ width: 220, height: 14 }} />
  </div>
);

export default function MainApp({ role, goLanding }: MainAppProps) {
  const { t } = useLang();
  const [tab, setTab] = useState<Tab>('leads');
  const [org, setOrg] = useState<Organization>({ id: 0, name: '...', abbr: '..', plan: 'Starter', color: '' });
  const [inboxBadge, setInboxBadge] = useState(0);

  useEffect(() => {
    get<{ org: { id: number; name: string; plan_tier: string } }>('/org')
      .then(response => {
        if (!response.org) return;
        const { id, name, plan_tier: planTier } = response.org;
        const planMap: Record<string, Organization['plan']> = {
          free: 'Starter',
          pro: 'Pro',
          enterprise: 'Enterprise',
        };
        setOrg({
          id,
          name,
          abbr: makeAbbr(name),
          plan: planMap[planTier] ?? 'Starter',
          color: '',
        });
      })
      .catch(() => {});
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
        const response = await get<ThreadsBadgeResponse>('/threads');
        const unread = typeof response.unread_count === 'number'
          ? response.unread_count
          : (response.threads ?? []).reduce((sum, thread) => sum + Math.max(0, Number(thread.unread_count ?? 0) || 0), 0);
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

  const mainTabs = (isAdmin ? ADMIN_TABS : STAFF_TABS).map(item => (
    item.id === 'inbox' ? { ...item, badge: inboxBadge } : item
  ));

  const navLabel = (id: Tab) => {
    const map: Record<Tab, string> = {
      leads: t.nav.leads,
      chat: t.nav.chat,
      browser: t.nav.browser,
      inbox: t.nav.inbox,
      posting: t.nav.posting,
      commenting: t.nav.commenting,
      leaderboard: t.nav.leaderboard,
      data: t.nav.dataPrivate,
      settings: t.nav.settings,
    };
    return map[id];
  };

  const renderView = () => {
    switch (tab) {
      case 'leads':
        return <LeadsView orgId={orgId} isAdmin={isAdmin} />;
      case 'chat':
        return <WorkspaceChatView orgId={orgId} />;
      case 'browser':
        return <BrowserView orgId={orgId} />;
      case 'inbox':
        return <InboxView orgId={orgId} />;
      case 'posting':
        return <PostingView orgId={orgId} />;
      case 'commenting':
        return <CommentingView orgId={orgId} />;
      case 'leaderboard':
        return <LeaderboardView orgId={orgId} isAdmin={isAdmin} />;
      case 'data':
        return <DataPrivateView orgId={orgId} />;
      case 'settings':
        return <SettingsPage org={org} orgId={orgId} isAdmin={isAdmin} />;
      default:
        return null;
    }
  };

  const renderNavItem = (item: NavItem) => (
    <button
      key={item.id}
      type="button"
      className={`nav-item ${tab === item.id ? 'is-active' : ''}`}
      onClick={() => setTab(item.id)}
      style={{ width: '100%', background: 'transparent', border: 0, textAlign: 'left' }}
    >
      <span className="icon">
        <item.Icon size={16} />
      </span>
      <span>{navLabel(item.id)}</span>
      {item.badge != null && item.badge > 0 && <span className="badge-num badge">{item.badge}</span>}
    </button>
  );

  return (
    <div className="app-shell">
      <header className="app-topbar">
        <div className="brand">
          <div className="brand-mark">A</div>
          <span className="brand-name">
            AutoFlow
            <span className="dim">.thg</span>
          </span>
        </div>

        <button className="btn btn-ghost btn-sm btn-square" style={{ marginLeft: 8 }} type="button">
          <span className="avatar avatar-sm">{org.abbr}</span>
          <span style={{ maxWidth: 180, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {org.name}
          </span>
          <ChevronDown size={13} style={{ color: 'var(--text-faint)' }} />
        </button>

        <div style={{ flex: 1, maxWidth: 480, margin: '0 auto', position: 'relative' }}>
          <Search
            size={14}
            style={{
              position: 'absolute',
              left: 12,
              top: '50%',
              transform: 'translateY(-50%)',
              color: 'var(--text-faint)',
              pointerEvents: 'none',
            }}
          />
          <input
            className="input"
            placeholder={`${t.topbar.search}  Ctrl/Cmd + K`}
            style={{ paddingLeft: 32, background: 'var(--bg-elev-2)' }}
          />
        </div>

        <DensitySwitch />
        <LangSwitch />

        <button className="btn btn-ghost btn-icon" type="button" aria-label="Notifications" style={{ position: 'relative' }}>
          <Bell size={15} />
          <span style={{ position: 'absolute', top: 6, right: 6, width: 6, height: 6, background: 'var(--hot)', borderRadius: '50%' }} />
        </button>

        <button className="btn btn-ghost btn-sm" type="button" onClick={goLanding}>
          <span className="avatar avatar-sm">{isAdmin ? 'A' : 'S'}</span>
          <span>{isAdmin ? 'Admin' : 'Staff'}</span>
          <LogOut size={13} style={{ color: 'var(--text-faint)' }} />
        </button>
      </header>

      <aside className="app-sidebar">
        <div className="sidebar-section">{t.nav.main}</div>
        {mainTabs.map(renderNavItem)}

        <div className="sidebar-section">{t.nav.analytics}</div>
        {ANALYTICS_TABS.map(renderNavItem)}

        <div className="sidebar-section">{t.nav.system}</div>
        {renderNavItem({ id: 'settings', Icon: SettingsIcon })}
      </aside>

      <main className="app-content">
        <Suspense fallback={<Spinner />}>{renderView()}</Suspense>
      </main>
    </div>
  );
}
