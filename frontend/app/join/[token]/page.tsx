'use client';
import { useRouter, useParams } from 'next/navigation';
import { useEffect } from 'react';
import JoinWorkspace from '@/src/modules/autoflow/components/JoinWorkspace';
import { initAuthSync } from '@/src/modules/autoflow/services/authSync';
import '@/src/modules/autoflow/autoflow.css';

export default function JoinPage() {
  const router = useRouter();
  const params = useParams<{ token: string }>();
  const token = decodeURIComponent(String(params?.token ?? ''));

  useEffect(() => { initAuthSync(); }, []);

  // Post-accept navigation lives inside the shared accept flow
  // (useAcceptInvite) — it routes to the freshly joined workspace from
  // the accept response, never from cached service state.
  return <JoinWorkspace token={token} goBack={() => router.push('/')} />;
}
