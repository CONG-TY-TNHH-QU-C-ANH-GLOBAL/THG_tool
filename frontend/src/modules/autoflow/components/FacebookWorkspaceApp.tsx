'use client';

import { useState, useEffect, lazy, Suspense } from 'react';
import type { Organization } from '../types';
import { get } from '../services/api';
import { useRoleStore } from '../stores/roleStore';
import SettingsPage from './SettingsPage';
import { useLang } from '../i18n/useLang';
import { isPlatformRole } from '../services/authService';
import {
  Activity,
  Bot,
  Clapperboard,
  FileText,
  Globe,
  MessageCircle,
  MessageSquare,
  Settings as SettingsIcon,
  Target,
  Trophy,
  Users,
} from 'lucide-react';

const LeadsView = lazy(() => import('./views/LeadsView'));
const WorkspaceChatView = lazy(() => import('./views/WorkspaceChatView'));
const BrowserView = lazy(() => import('./views/BrowserView'));
const AccountHealthBoard = lazy(() => import('./accountHealth/AccountHealthBoard'));
const InboxView = lazy(() => import('./views/InboxView'));
const PostingView = lazy(() => import('./views/PostingView'));
const CommentingView = lazy(() => import('./views/CommentingView'));
const ReelView = lazy(() => import('./views/ReelView'));
const LeaderboardView = lazy(() => import('./views/LeaderboardView'));
const MissionsView = lazy(() => import('./views/MissionsView'));

type Tab = 'leads' | 'chat' | 'browser' | 'health' | 'inbox' | 'posting' | 'commenting' | 'reel' | 'leaderboard' | 'missions' | 'settings';

interface FacebookWorkspaceAppProps {
  workspaceId: string;
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
  { id: 'health', Icon: Activity },
  { id: 'inbox', Icon: MessageSquare },
  { id: 'posting', Icon: FileText },
  { id: 'commenting', Icon: MessageCircle },
  { id: 'reel', Icon: Clapperboard },
];

const STAFF_TABS: NavItem[] = [
  { id: 'leads', Icon: Users },
  { id: 'chat', Icon: Bot },
  // Sales/staff users own their FB accounts (see AccountOwnerAllowed in
  // internal/server/agent/account_guard). They need browser access to
  // initialize/manage their own automation session. Backend already
  // enforces ownership on action endpoints — sales can only start/stop
  // sessions on accounts they own — so the panel is safe to expose.
  // Shared-battlefield: sales see all accounts as context, action only
  // their own. See feedback_shared_battlefield_not_crm.
  { id: 'health', Icon: Activity },
  { id: 'inbox', Icon: MessageSquare },
];

const ANALYTICS_TABS: NavItem[] = [
  { id: 'missions', Icon: Target },
  { id: 'leaderboard', Icon: Trophy },
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

export default function FacebookWorkspaceApp({ workspaceId }: Readonly<FacebookWorkspaceAppProps>) {
  const { t } = useLang();
  const { role } = useRoleStore();
  const isAdmin = role === 'admin' || isPlatformRole(role);

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
  }, [workspaceId]);

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

  // PR-E.2: the legacy Browser tab (old LocalConnectorPanel / stream viewer with
  // dev-style stream/pairing wording) is SUPERADMIN-only. Customers/members use the
  // "Kết nối Facebook" tab as the single official setup flow — no second connection
  // surface. The Browser view code is kept (not deleted) for platform debug.
  const isPlatform = isPlatformRole(role);
  const baseTabs = isAdmin ? ADMIN_TABS : STAFF_TABS;
  const visibleTabs: NavItem[] = isPlatform ? [...baseTabs, { id: 'browser', Icon: Globe }] : baseTabs;
  const mainTabs = visibleTabs.map(item => (
    item.id === 'inbox' ? { ...item, badge: inboxBadge } : item
  ));

  const navLabel = (id: Tab) => {
    const map: Record<Tab, string> = {
      leads: t.nav.leads,
      chat: t.nav.chat,
      browser: t.nav.browser,
      // Inline label (PR-E) — kept out of the large i18n strings.ts god file per
      // the Engineering Guardrails (do not grow legacy large files).
      health: 'Kết nối Facebook',
      inbox: t.nav.inbox,
      posting: t.nav.posting,
      commenting: t.nav.commenting,
      // Inline label (like `health`) to avoid growing the i18n strings.ts god file; the
      // view's own copy is localized via i18n/reelStrings.ts.
      reel: 'Reel',
      leaderboard: t.nav.leaderboard,
      missions: t.nav.missions,
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
        // Superadmin-only legacy debug surface (PR-E.2). A customer/member can
        // never reach it via nav; this guard keeps them out even on a stale tab.
        return isPlatform ? <BrowserView orgId={orgId} /> : <AccountHealthBoard orgId={orgId} isAdmin={isAdmin} onNavigate={(t) => setTab(t as Tab)} />;
      case 'health':
        return <AccountHealthBoard orgId={orgId} isAdmin={isAdmin} onNavigate={(t) => setTab(t as Tab)} />;
      case 'inbox':
        return <InboxView orgId={orgId} />;
      case 'posting':
        return <PostingView orgId={orgId} isAdmin={isAdmin} />;
      case 'commenting':
        return <CommentingView orgId={orgId} isAdmin={isAdmin} />;
      case 'reel':
        return <ReelView orgId={orgId} isAdmin={isAdmin} />;
      case 'leaderboard':
        return <LeaderboardView orgId={orgId} isAdmin={isAdmin} />;
      case 'missions':
        return <MissionsView orgId={orgId} isAdmin={isAdmin} />;
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
    <div
      className="workspace-shell"
      style={{
        display: 'grid',
        gridTemplateColumns: '220px 1fr',
        // `.app-sidebar` / `.app-content` are defined for the legacy `.app-shell`
        // grid which used named areas. PlatformShell now wraps this view in
        // a flex column with no template-areas — we have to declare them here
        // or both children collapse into the same cell (the bug that surfaced
        // as overlapping sidebar + content).
        gridTemplateAreas: '"sidebar content"',
        height: '100%',
        minHeight: 0,
      }}
    >
      <aside className="app-sidebar">
        <div style={{ padding: '8px 8px 16px' }}>
          <div className="brand">
            <div className="brand-mark" style={{ background: 'var(--accent)', color: 'var(--accent-ink)', fontFamily: 'var(--font-mono)', borderRadius: 4 }}>F</div>
            <div className="brand-name">{org.name || 'Facebook Automation'}</div>
          </div>
        </div>

        <div className="sidebar-section">{t.nav.main}</div>
        {mainTabs.map(renderNavItem)}

        <div className="sidebar-section">{t.nav.analytics}</div>
        {ANALYTICS_TABS.map(renderNavItem)}

        <div className="sidebar-section">{t.nav.system}</div>
        {renderNavItem({ id: 'settings', Icon: SettingsIcon })}
      </aside>

      <main className="app-content" style={{ minHeight: 0, overflow: 'auto' }}>
        <Suspense fallback={<Spinner />}>{renderView()}</Suspense>
      </main>
    </div>
  );
}
