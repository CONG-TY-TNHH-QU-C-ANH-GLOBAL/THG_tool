'use client';
import { useRouter, useParams } from 'next/navigation';
import { useEffect } from 'react';
import JoinWorkspace from '@/src/modules/autoflow/components/JoinWorkspace';
import { initAuthSync } from '@/src/modules/autoflow/services/authSync';
import { isPlatformRole } from '@/src/modules/autoflow/services/authService';
import { usePlatformService } from '@/src/platform/services/usePlatformServices';
import '@/src/modules/autoflow/autoflow.css';

export default function JoinPage() {
  const router = useRouter();
  const params = useParams<{ token: string }>();
  const token = decodeURIComponent(String(params?.token ?? ''));
  const fb = usePlatformService('facebook');

  useEffect(() => { initAuthSync(); }, []);

  return (
    <JoinWorkspace
      token={token}
      onJoined={(role) => {
        if (isPlatformRole(role)) { router.replace('/superadmin'); return; }
        if (fb?.workspaceState === 'ready' && fb.workspaceId) {
          router.replace(`/services/facebook/workspaces/${fb.workspaceId}`);
        } else {
          router.replace('/services');
        }
      }}
      goBack={() => router.push('/')}
    />
  );
}
