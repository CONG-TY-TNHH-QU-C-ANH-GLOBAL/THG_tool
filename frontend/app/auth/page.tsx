'use client';
import { Suspense, useEffect, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import Auth from '@/src/modules/autoflow/components/Auth';
import { useAuth } from '@/src/modules/autoflow/hooks/useAuth';
import { initAuthSync } from '@/src/modules/autoflow/services/authSync';
import { isPlatformRole } from '@/src/modules/autoflow/services/authService';
import { usePlatformService } from '@/src/platform/services/usePlatformServices';
import '@/src/modules/autoflow/autoflow.css';

type Mode = 'login' | 'register' | 'forgot' | 'success';

const Spinner = () => (
  <div style={{ minHeight: '100vh', display: 'grid', placeItems: 'center' }}>
    <div className="skeleton" style={{ width: 240, height: 14 }} />
  </div>
);

function AuthInner() {
  const router = useRouter();
  const search = useSearchParams();
  const initialMode = (search?.get('mode') as Mode) || 'login';
  const [mode, setMode] = useState<Mode>(initialMode);
  const { user, hydrated } = useAuth();
  const fb = usePlatformService('facebook');

  useEffect(() => { initAuthSync(); }, []);

  useEffect(() => {
    if (!hydrated || !user) return;
    if (isPlatformRole(user.role)) { router.replace('/superadmin'); return; }
    if (fb?.workspaceState === 'ready' && fb.workspaceId) {
      router.replace(`/services/facebook/workspaces/${fb.workspaceId}`);
    } else {
      router.replace('/services');
    }
  }, [hydrated, user, fb, router]);

  return (
    <Auth
      mode={mode}
      setMode={setMode}
      onSuccess={() => { /* redirect handled by auth effect above */ }}
      goBack={() => router.push('/')}
    />
  );
}

export default function AuthPage() {
  return (
    <Suspense fallback={<Spinner />}>
      <AuthInner />
    </Suspense>
  );
}
