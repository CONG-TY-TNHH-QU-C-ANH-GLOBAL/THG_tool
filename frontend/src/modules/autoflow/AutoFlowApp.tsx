import { useState, useEffect } from 'react';
import './autoflow.css';
import Landing from './components/Landing';
import Auth from './components/Auth';
import MainApp from './components/MainApp';
import SuperAdmin from './components/SuperAdmin';
import Onboarding from './components/Onboarding';
import JoinWorkspace from './components/JoinWorkspace';
import { useAuth } from './hooks/useAuth';
import { useRoleStore } from './stores/roleStore';
import { initAuthSync } from './services/authSync';
import { isPlatformRole } from './services/authService';

type Screen = 'landing' | 'auth' | 'onboarding' | 'app' | 'superadmin' | 'join';
type AuthMode = 'login' | 'register' | 'forgot' | 'success';

export default function AutoFlowApp() {
  const inviteToken = window.location.pathname.startsWith('/join/')
    ? decodeURIComponent(window.location.pathname.replace('/join/', '').split('/')[0] ?? '')
    : '';
  const [screen, setScreen] = useState<Screen>(inviteToken ? 'join' : 'landing');
  const [authMode, setAuthMode] = useState<AuthMode>('login');
  // True while we're exchanging the g_at cookie from a Google OAuth redirect.
  // Suppresses the org-based routing until the exchange completes.
  const [googleAuthPending, setGoogleAuthPending] = useState(
    () => new URLSearchParams(window.location.search).has('google_auth'),
  );
  const { user, logout } = useAuth();
  const { role } = useRoleStore();

  // On mount: schedule silent pre-expiry refresh + multi-tab sync
  useEffect(() => {
    initAuthSync();
  }, []);

  // Handle Google OAuth redirect (/?google_auth=1) at the top level so it works
  // regardless of which screen the app starts on.
  useEffect(() => {
    if (!googleAuthPending) return;
    history.replaceState(null, '', window.location.pathname);
    fetch('/api/auth/google/token', { method: 'POST', credentials: 'include' })
      .then(r => (r.ok ? r.json() : Promise.reject()))
      .then(async (data) => {
        const { useAuthStore } = await import('./stores/authStore');
        useAuthStore.getState().setAuth(data.access_token, data.user);
        setGoogleAuthPending(false);
        if (isPlatformRole(data.user?.role)) {
          setScreen('superadmin');
        } else if (data.needs_onboarding) {
          setScreen('onboarding');
        } else {
          setScreen('app');
        }
      })
      .catch(() => setGoogleAuthPending(false));
  }, []);

  // Route authenticated users to the correct screen.
  // Suppressed while googleAuthPending to avoid racing with the OAuth exchange.
  useEffect(() => {
    if (googleAuthPending) return;
    if (user && screen !== 'app' && screen !== 'superadmin' && screen !== 'onboarding' && screen !== 'join') {
      if (isPlatformRole(user.role)) {
        setScreen('superadmin');
      } else if ((user as { org_id?: number }).org_id === 0) {
        // Only send to onboarding from the auth screen (fresh login/signup).
        // Prevents a stale org_id=0 token in localStorage from wrongly
        // redirecting an already-onboarded user who did a Google re-login.
        if (screen === 'auth') setScreen('onboarding');
      } else {
        setScreen('app');
      }
    }
  }, [user, screen, googleAuthPending]);

  const mainRole: 'admin' | 'staff' = role === 'admin' || isPlatformRole(role) ? 'admin' : 'staff';

  if (screen === 'superadmin') {
    return <SuperAdmin goBack={() => setScreen('landing')} />;
  }

  if (screen === 'onboarding') {
    return <Onboarding onComplete={(r) => setScreen(isPlatformRole(r) ? 'superadmin' : 'app')} />;
  }

  if (screen === 'join' && inviteToken) {
    return (
      <JoinWorkspace
        token={inviteToken}
        onJoined={(r) => {
          history.replaceState(null, '', '/');
          setScreen(isPlatformRole(r) ? 'superadmin' : 'app');
        }}
        goBack={() => { history.replaceState(null, '', '/'); setScreen('landing'); }}
      />
    );
  }

  if (screen === 'app') {
    return <MainApp role={mainRole} goLanding={async () => { await logout(); setScreen('landing'); }} />;
  }

  if (screen === 'auth') {
    return (
      <Auth
        mode={authMode}
        setMode={setAuthMode}
        onSuccess={(r) => {
          const nextScreen = isPlatformRole(r) ? 'superadmin' : 'app';
          setScreen(nextScreen);
        }}
        onNeedsOnboarding={() => setScreen('onboarding')}
        goBack={() => { setScreen('landing'); setAuthMode('login'); }}
      />
    );
  }

  return (
    <Landing
      onLogin={() => { setAuthMode('login'); setScreen('auth'); }}
      onRegister={() => { setAuthMode('register'); setScreen('auth'); }}
      onAdmin={() => { setAuthMode('login'); setScreen('auth'); }}
    />
  );
}
