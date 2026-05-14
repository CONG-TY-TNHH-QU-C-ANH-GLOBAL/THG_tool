'use client';
import { useEffect } from 'react';
import { useRouter } from 'next/navigation';
import SuperAdmin from '@/src/modules/autoflow/components/SuperAdmin';
import { useAuth } from '@/src/modules/autoflow/hooks/useAuth';
import { initAuthSync } from '@/src/modules/autoflow/services/authSync';
import { isPlatformRole } from '@/src/modules/autoflow/services/authService';
import '@/src/modules/autoflow/autoflow.css';

const Spinner = () => (
  <div style={{ minHeight: '100vh', display: 'grid', placeItems: 'center' }}>
    <div className="skeleton" style={{ width: 240, height: 14 }} />
  </div>
);

export default function SuperAdminPage() {
  const router = useRouter();
  const { user, hydrated } = useAuth();

  useEffect(() => { initAuthSync(); }, []);

  useEffect(() => {
    if (!hydrated) return;
    if (!user) { router.replace('/auth?mode=login'); return; }
    if (!isPlatformRole(user.role)) { router.replace('/services'); return; }
  }, [hydrated, user, router]);

  if (!hydrated || !user || !isPlatformRole(user.role)) return <Spinner />;

  return <SuperAdmin goBack={() => router.push('/')} />;
}
