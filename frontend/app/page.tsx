'use client';
import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import PlatformLanding from '@/src/marketing/PlatformLanding';
import { useAuth } from '@/src/modules/autoflow/hooks/useAuth';
import { useAuthStore } from '@/src/modules/autoflow/stores/authStore';
import { initAuthSync } from '@/src/modules/autoflow/services/authSync';
import { isPlatformRole } from '@/src/modules/autoflow/services/authService';
import { usePlatformService } from '@/src/platform/services/usePlatformServices';
import '@/src/modules/autoflow/autoflow.css';

const Spinner = () => (
  <div style={{ minHeight: '100vh', display: 'grid', placeItems: 'center' }}>
    <div className="skeleton" style={{ width: 240, height: 14 }} />
  </div>
);

export default function HomePage() {
  const router = useRouter();
  const { user, hydrated } = useAuth();
  const fb = usePlatformService('facebook');
  const [googleAuthPending, setGoogleAuthPending] = useState(false);

  useEffect(() => { initAuthSync(); }, []);

  useEffect(() => {
    const pending = new URLSearchParams(window.location.search).has('google_auth');
    if (!pending) return;
    setGoogleAuthPending(true);
    history.replaceState(null, '', window.location.pathname);
    fetch('/api/auth/google/token', { method: 'POST', credentials: 'include' })
      .then(r => (r.ok ? r.json() : Promise.reject()))
      .then((data) => {
        useAuthStore.getState().setAuth(data.access_token, data.user);
        setGoogleAuthPending(false);
      })
      .catch(() => setGoogleAuthPending(false));
  }, []);

  useEffect(() => {
    if (googleAuthPending) return;
    if (!hydrated || !user) return;
    if (isPlatformRole(user.role)) { router.replace('/superadmin'); return; }
    if (fb?.workspaceState === 'ready' && fb.workspaceId) {
      router.replace(`/services/facebook/workspaces/${fb.workspaceId}`);
    } else {
      router.replace('/services');
    }
  }, [hydrated, user, fb, googleAuthPending, router]);

  if (googleAuthPending || (hydrated && user)) return <Spinner />;

  return (
    <PlatformLanding
      onLogin={() => router.push('/auth?mode=login')}
      onRegister={() => router.push('/auth?mode=register')}
    />
  );
}
