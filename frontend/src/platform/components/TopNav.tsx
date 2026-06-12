'use client';

import { useState, useRef, useEffect } from 'react';
import { useRouter, usePathname } from 'next/navigation';
import { ChevronDown, LogOut, User as UserIcon, LayoutGrid } from 'lucide-react';
import { useAuthStore } from '../../modules/autoflow/stores/authStore';
import NotificationBell from '../../modules/autoflow/components/notifications/NotificationBell';
import { LangSwitch } from '../../modules/autoflow/components/ds/LangSwitch';
import { DensitySwitch } from '../../modules/autoflow/components/ds/DensitySwitch';
import { ThemeToggle } from '../../modules/autoflow/components/ds/ThemeToggle';
import { useLang } from '../../modules/autoflow/i18n/useLang';
import { usePlatformServices } from '../services/usePlatformServices';

function makeAbbr(name: string): string {
  const trimmed = (name || '').trim();
  if (!trimmed) return '?';
  const words = trimmed.split(/\s+/);
  if (words.length >= 2) return (words[0][0] + words[1][0]).toUpperCase();
  return trimmed.slice(0, 2).toUpperCase();
}

export default function TopNav() {
  const router = useRouter();
  const pathname = usePathname();
  const { lang } = useLang();
  const user = useAuthStore(s => s.user);
  const logout = useAuthStore(s => s.logout);
  const services = usePlatformServices();
  const [menuOpen, setMenuOpen] = useState(false);
  const [switcherOpen, setSwitcherOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement | null>(null);
  const switcherRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) setMenuOpen(false);
      if (switcherRef.current && !switcherRef.current.contains(e.target as Node)) setSwitcherOpen(false);
    }
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, []);

  async function handleLogout() {
    await logout();
    setMenuOpen(false);
    router.push('/');
  }

  const activeServices = services.filter(s => s.status === 'available');
  const currentServiceSlug = pathname?.match(/^\/services\/([^/]+)/)?.[1] ?? null;
  const currentService = currentServiceSlug ? services.find(s => s.slug === currentServiceSlug) : null;

  return (
    <header
      className="platform-topbar"
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 12,
        padding: '10px 16px',
        borderBottom: '1px solid var(--line)',
        background: 'var(--bg)',
        minHeight: 48,
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
        <button
          type="button"
          className="brand"
          onClick={() => router.push('/services')}
          style={{ background: 'transparent', border: 0, cursor: 'pointer', padding: 0, display: 'flex', alignItems: 'center', gap: 8 }}
        >
          <div
            className="brand-mark"
            style={{ background: 'var(--accent)', color: 'var(--accent-ink)', fontFamily: 'var(--font-mono)', borderRadius: 4, width: 22, height: 22, display: 'grid', placeItems: 'center', fontSize: 12 }}
          >
            T
          </div>
          <span className="brand-name" style={{ fontSize: 13 }}>
            THG <span className="dim">/ Platform</span>
          </span>
        </button>

        {currentService && activeServices.length >= 1 && (
          <div ref={switcherRef} style={{ position: 'relative', marginLeft: 8 }}>
            <button
              type="button"
              className="btn btn-ghost btn-sm"
              onClick={() => setSwitcherOpen(o => !o)}
              style={{ display: 'flex', alignItems: 'center', gap: 6 }}
            >
              <LayoutGrid size={13} />
              <span>{currentService.label}</span>
              <ChevronDown size={12} />
            </button>
            {switcherOpen && (
              <div
                className="card"
                style={{
                  position: 'absolute',
                  top: '100%',
                  left: 0,
                  marginTop: 6,
                  minWidth: 220,
                  padding: 6,
                  zIndex: 50,
                  boxShadow: '0 10px 30px rgba(0,0,0,0.2)',
                }}
              >
                <button
                  type="button"
                  className="btn btn-ghost btn-sm"
                  style={{ width: '100%', justifyContent: 'flex-start' }}
                  onClick={() => { setSwitcherOpen(false); router.push('/services'); }}
                >
                  {lang === 'vi' ? 'Tất cả services' : 'All services'}
                </button>
                {services.map(svc => (
                  <button
                    key={svc.slug}
                    type="button"
                    className="btn btn-ghost btn-sm"
                    style={{ width: '100%', justifyContent: 'space-between', opacity: svc.status === 'available' ? 1 : 0.5 }}
                    disabled={svc.status !== 'available'}
                    onClick={() => {
                      setSwitcherOpen(false);
                      if (svc.workspaceState === 'ready' && svc.workspaceId) {
                        router.push(`/services/${svc.slug}/workspaces/${svc.workspaceId}`);
                      } else {
                        router.push(`/services/${svc.slug}/workspaces/new`);
                      }
                    }}
                  >
                    <span>{svc.label}</span>
                    {svc.status !== 'available' && (
                      <span style={{ fontSize: 10, color: 'var(--text-faint)' }}>
                        {svc.status === 'unavailable' ? (lang === 'vi' ? 'Sắp ra mắt' : 'Coming soon') : svc.status}
                      </span>
                    )}
                  </button>
                ))}
              </div>
            )}
          </div>
        )}
      </div>

      <div style={{ flex: 1 }} />

      <NotificationBell />
      <ThemeToggle />
      <DensitySwitch />
      <LangSwitch />

      {user && (
        <div ref={menuRef} style={{ position: 'relative' }}>
          <button
            type="button"
            className="btn btn-ghost btn-sm"
            onClick={() => setMenuOpen(o => !o)}
            style={{ display: 'flex', alignItems: 'center', gap: 8 }}
          >
            <span
              className="avatar avatar-sm"
              style={{ width: 24, height: 24, display: 'grid', placeItems: 'center', fontSize: 11, background: 'var(--accent-soft)', color: 'var(--accent)', borderRadius: '50%' }}
            >
              {makeAbbr(user.name)}
            </span>
            <span style={{ fontSize: 13, fontWeight: 500, color: 'var(--text)' }}>{user.name}</span>
            <ChevronDown size={12} />
          </button>
          {menuOpen && (
            <div
              className="card"
              style={{
                position: 'absolute',
                top: '100%',
                right: 0,
                marginTop: 6,
                minWidth: 220,
                padding: 6,
                zIndex: 50,
                boxShadow: '0 10px 30px rgba(0,0,0,0.2)',
              }}
            >
              <div style={{ padding: '8px 10px', borderBottom: '1px solid var(--line)' }}>
                <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text)' }}>{user.name}</div>
                <div style={{ fontSize: 11, color: 'var(--text-faint)' }}>{user.email}</div>
              </div>
              <button
                type="button"
                className="btn btn-ghost btn-sm"
                style={{ width: '100%', justifyContent: 'flex-start' }}
                onClick={() => { setMenuOpen(false); router.push('/services'); }}
              >
                <UserIcon size={13} /> {lang === 'vi' ? 'Services của tôi' : 'My services'}
              </button>
              <button
                type="button"
                className="btn btn-ghost btn-sm"
                style={{ width: '100%', justifyContent: 'flex-start', color: 'var(--text-mute)' }}
                onClick={() => void handleLogout()}
              >
                <LogOut size={13} /> {lang === 'vi' ? 'Đăng xuất' : 'Sign out'}
              </button>
            </div>
          )}
        </div>
      )}
    </header>
  );
}
