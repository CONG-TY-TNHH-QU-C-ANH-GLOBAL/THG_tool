'use client';
import { useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { useAuth } from '@/src/modules/autoflow/hooks/useAuth';
import { initAuthSync } from '@/src/modules/autoflow/services/authSync';
import { isPlatformRole } from '@/src/modules/autoflow/services/authService';
import { usePlatformService } from '@/src/platform/services/usePlatformServices';
import '@/src/modules/autoflow/autoflow.css';

const Spinner = () => (
  <div style={{ minHeight: '100vh', display: 'grid', placeItems: 'center' }}>
    <div className="skeleton" style={{ width: 240, height: 14 }} />
  </div>
);

export default function AutoflowLegacyRedirect() {
  const router = useRouter();
  const { user, hydrated } = useAuth();
  const fb = usePlatformService('facebook');

  useEffect(() => { initAuthSync(); }, []);

  useEffect(() => {
    if (!hydrated) return;
    if (!user) { router.replace('/auth?mode=login'); return; }
    if (isPlatformRole(user.role)) { router.replace('/superadmin'); return; }
    if (fb?.workspaceState === 'ready' && fb.workspaceId) {
      router.replace(`/services/facebook/workspaces/${fb.workspaceId}`);
    } else {
      router.replace('/services');
    }
  }, [hydrated, user, fb, router]);

  return <Spinner />;
}
