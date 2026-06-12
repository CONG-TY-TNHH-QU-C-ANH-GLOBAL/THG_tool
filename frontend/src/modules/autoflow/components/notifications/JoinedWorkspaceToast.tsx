'use client';
import { useEffect, useState } from 'react';
import { Check } from 'lucide-react';
import { consumeJoinedWorkspace } from '../../services/membershipService';

/**
 * One-shot toast after an invite accept: «Bạn đã tham gia workspace
 * <name>.» The accept flow navigates immediately, so the name crosses
 * the route via sessionStorage (membershipService) and renders once
 * here, in the shell layer.
 */
export default function JoinedWorkspaceToast() {
  const [name, setName] = useState<string | null>(null);

  useEffect(() => {
    const joined = consumeJoinedWorkspace();
    if (!joined) return;
    setName(joined);
    const t = setTimeout(() => setName(null), 6000);
    return () => clearTimeout(t);
  }, []);

  if (!name) return null;
  return (
    <div
      role="status"
      style={{
        position: 'fixed',
        bottom: 24,
        right: 24,
        zIndex: 200,
        display: 'flex',
        alignItems: 'center',
        gap: 10,
        padding: '12px 16px',
        borderRadius: 8,
        background: 'var(--bg-raised, var(--bg))',
        border: '1px solid var(--line)',
        borderLeft: '3px solid var(--ok)',
        boxShadow: '0 10px 30px rgba(0,0,0,0.25)',
        fontSize: 13,
        color: 'var(--text)',
        maxWidth: 360,
      }}
    >
      <Check size={15} color="var(--ok)" />
      <span>
        Bạn đã tham gia workspace <strong>{name}</strong>.
      </span>
    </div>
  );
}
