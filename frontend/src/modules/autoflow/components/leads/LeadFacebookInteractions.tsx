'use client';

import { ActorVerdictChip } from '../ActorVerdictChip';
import type { LeadEngagementEntry } from '../../types';

// "Tương tác Facebook" — which Facebook account(s) already commented this SHARED
// lead. Execution is OWNED (per account) even though the lead is shared; multiple
// accounts commenting one lead is valid amplification. Observability only — this is
// NOT a lock/ownership signal. One row per distinct account (latest interaction).

interface Props {
  entries: LeadEngagementEntry[];
  relativeTime: (s: string) => string;
}

const ACTION_LABEL: Record<string, string> = {
  comment: 'comment', inbox: 'nhắn tin', group_post: 'đăng bài', profile_post: 'đăng bài',
};

export function LeadFacebookInteractions({ entries, relativeTime }: Props) {
  const seen = new Set<number>();
  const fb = (entries || [])
    .filter(e => e.account_id > 0 && (e.channel ?? 'facebook') === 'facebook')
    .filter(e => (seen.has(e.account_id) ? false : (seen.add(e.account_id), true)));

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      <div style={{ fontSize: 11, fontWeight: 600, letterSpacing: 0.5, color: 'var(--text-faint)', textTransform: 'uppercase' }}>
        Tương tác Facebook
      </div>
      {fb.length === 0 ? (
        <div style={{ fontSize: 12.5, color: 'var(--text-mute)' }}>Chưa có account Facebook nào comment lead này.</div>
      ) : (
        fb.map(e => {
          const name = e.fb_display_name || e.account_name || `Account #${e.account_id}`;
          return (
            <div key={e.account_id} style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap', fontSize: 12.5 }}>
              <span style={{ color: 'var(--text)', fontWeight: 500 }}>{name}</span>
              <span style={{ color: 'var(--text-faint)' }}>· Account #{e.account_id}</span>
              <ActorVerdictChip actorVerdict={e.actor_verdict} actorBlocked={e.actor_blocked} />
              <span style={{ color: 'var(--text-mute)' }}>· Đã {ACTION_LABEL[e.action] || e.action} {relativeTime(e.performed_at)}</span>
            </div>
          );
        })
      )}
    </div>
  );
}
