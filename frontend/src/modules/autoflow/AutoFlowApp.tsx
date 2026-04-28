import { useState, useEffect } from 'react';
import './autoflow.css';
import Landing from './components/Landing';
import Auth from './components/Auth';
import MainApp from './components/MainApp';
import SuperAdmin from './components/SuperAdmin';
import { useAuth } from './hooks/useAuth';
import { useRoleStore } from './stores/roleStore';
import { initAuthSync } from './services/authSync';

type Screen = 'landing' | 'auth' | 'app' | 'superadmin';
type AuthMode = 'login' | 'register' | 'forgot' | 'success';

export default function AutoFlowApp() {
  const [screen, setScreen] = useState<Screen>('landing');
  const [authMode, setAuthMode] = useState<AuthMode>('login');
  const { user, logout } = useAuth();
  const { role, isSuperAdmin } = useRoleStore();

  // On mount: schedule silent pre-expiry refresh + multi-tab sync
  useEffect(() => {
    initAuthSync();
  }, []);

  useEffect(() => {
    if (user && screen === 'auth') {
      setScreen(isSuperAdmin ? 'superadmin' : 'app');
    }
  }, [user, isSuperAdmin]);

  const mainRole: 'admin' | 'staff' = role === 'admin' || role === 'superadmin' ? 'admin' : 'staff';

  if (screen === 'superadmin') {
    return <SuperAdmin goBack={() => setScreen('landing')} />;
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
          const nextScreen = r === 'superadmin' ? 'superadmin' : 'app';
          setScreen(nextScreen);
        }}
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
