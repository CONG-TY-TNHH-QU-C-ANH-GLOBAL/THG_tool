'use client';
import { Suspense, type ReactNode, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { useAuth } from '../../modules/autoflow/hooks/useAuth';
import { initAuthSync } from '../../modules/autoflow/services/authSync';
import { isPlatformRole } from '../../modules/autoflow/services/authService';
import { useRoleStore } from '../../modules/autoflow/stores/roleStore';
import TopNav from './TopNav';
import JoinedWorkspaceToast from '../../modules/autoflow/components/notifications/JoinedWorkspaceToast';
import { PlatformErrorBoundary } from './PlatformErrorBoundary';
import '../../modules/autoflow/autoflow.css';

interface PlatformShellProps {
  children: ReactNode;
}

const ShellSpinner = () => (
  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '60vh' }}>
    <div className="skeleton" style={{ width: 240, height: 14 }} />
  </div>
);

export default function PlatformShell({ children }: PlatformShellProps) {
  const router = useRouter();
  const { user, hydrated } = useAuth();
  const { role, setRole } = useRoleStore();

  useEffect(() => { initAuthSync(); }, []);

  useEffect(() => {
    if (!hydrated) return;
    if (!user) {
      router.replace('/auth');
      return;
    }
    if (user.role) setRole(user.role);
    if (isPlatformRole(user.role)) {
      router.replace('/superadmin');
    }
  }, [hydrated, user, router, setRole]);

  if (!hydrated || !user || isPlatformRole(role)) {
    return (
      <div style={{ minHeight: '100vh', display: 'grid', placeItems: 'center' }}>
        <ShellSpinner />
      </div>
    );
  }

  return (
    <div className="platform-shell" style={{ display: 'flex', flexDirection: 'column', height: '100vh', minHeight: 0 }}>
      <TopNav />
      <JoinedWorkspaceToast />
      <div className="platform-outlet" style={{ flex: 1, minHeight: 0, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
        <PlatformErrorBoundary>
          <Suspense fallback={<ShellSpinner />}>{children}</Suspense>
        </PlatformErrorBoundary>
      </div>
    </div>
  );
}
