'use client';

import { Users } from 'lucide-react';
import { ActorVerdictChip } from '../ActorVerdictChip';
import type { CommentEligibility, LeadEngagementEntry } from '../../types';
import { eligibilityLine } from './eligibilityCopy';

// "Tương tác Facebook" — which Facebook account(s) already commented this SHARED
// lead. Execution is OWNED (per account) even though the lead is shared; multiple
// accounts commenting one lead is valid amplification. Observability only — NOT a
// lock/ownership signal. One row per distinct account (latest interaction).

interface Props {
  entries: LeadEngagementEntry[];
  eligibility?: CommentEligibility;
}

const ELIGIBILITY_TONE: Record<string, { color: string; bg: string }> = {
  ok: { color: 'var(--success, #15803d)', bg: 'var(--bg-elev-2)' },
  warn: { color: 'var(--warning, #b45309)', bg: 'var(--bg-elev-2)' },
  mute: { color: 'var(--text-mute)', bg: 'var(--bg-elev-2)' },
};

const ACTION_LABEL: Record<string, string> = {
  comment: 'Comment', inbox: 'Nhắn tin', group_post: 'Đăng bài', profile_post: 'Đăng bài',
};

const MAX_ROWS = 6;

function timeAgoVi(iso: string): string {
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return '';
  const sec = Math.max(0, Math.floor((Date.now() - t) / 1000));
  if (sec < 60) return 'vừa xong';
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min} phút trước`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr} giờ trước`;
  return `${Math.floor(hr / 24)} ngày trước`;
}

export function LeadFacebookInteractions({ entries, eligibility }: Props) {
  const seen = new Set<number>();
  const fb = (entries || [])
    .filter(e => e.account_id > 0 && (e.channel ?? 'facebook') === 'facebook')
    .filter(e => (seen.has(e.account_id) ? false : (seen.add(e.account_id), true)));
  const nameOf = (e: LeadEngagementEntry) => e.fb_display_name || e.account_name || `Account #${e.account_id}`;
  const line = eligibilityLine(eligibility);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
        <span style={{ fontSize: 11, fontWeight: 600, letterSpacing: 0.5, color: 'var(--text-faint)', textTransform: 'uppercase' }}>
          Tương tác Facebook
        </span>
        {fb.length > 1 && (
          <span className="tag tag-mute" title={fb.map(nameOf).join('\n')} style={{ display: 'inline-flex', alignItems: 'center', gap: 4, fontSize: 10.5 }}>
            <Users size={11} /> {fb.length} account
          </span>
        )}
      </div>

      {/* §6: precise comment-eligibility state (same gates as comment_all_leads),
          replacing the old "Lead vẫn được chia sẻ…" copy that implied any account
          could always comment. */}
      {line && (
        <div style={{ fontSize: 12.5, color: ELIGIBILITY_TONE[line.tone].color, background: ELIGIBILITY_TONE[line.tone].bg, borderRadius: 8, padding: '8px 12px' }}>
          {line.text}
        </div>
      )}

      {fb.length === 0 ? (
        !line && (
          <div style={{ fontSize: 12.5, color: 'var(--text-mute)', background: 'var(--bg-elev-2)', borderRadius: 8, padding: '10px 12px' }}>
            Chưa có tài khoản Facebook nào comment lead này.
          </div>
        )
      ) : (
        <>
          {fb.slice(0, MAX_ROWS).map(e => (
            <div key={e.account_id} style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap', fontSize: 12.5 }}>
              <span style={{ color: 'var(--text)', fontWeight: 500 }}>{nameOf(e)}</span>
              <span style={{ color: 'var(--text-faint)' }}>· Account #{e.account_id}</span>
              <ActorVerdictChip actorVerdict={e.actor_verdict} actorBlocked={e.actor_blocked} />
              <span style={{ color: 'var(--text-mute)' }}>· {ACTION_LABEL[e.action] || e.action} {timeAgoVi(e.performed_at)}</span>
            </div>
          ))}
          {fb.length > MAX_ROWS && (
            <div style={{ fontSize: 12, color: 'var(--text-faint)' }} title={fb.slice(MAX_ROWS).map(nameOf).join('\n')}>
              và {fb.length - MAX_ROWS} account khác nữa
            </div>
          )}
        </>
      )}
    </div>
  );
}
