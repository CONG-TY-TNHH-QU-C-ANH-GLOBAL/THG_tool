'use client';

import { useState } from 'react';
import { AlertTriangle, CheckCircle2, ChevronDown, Clock, Globe, Monitor, ShieldAlert, User, Wrench } from 'lucide-react';
import type { AccountReadiness } from './types';
import { CAPABILITY_LABELS } from './types';
import { mapReason, overallStatus, type Severity } from './reasonMessages';

const SEVERITY_COLOR: Record<Severity, string> = {
  ready: 'var(--ok)',
  warning: 'var(--warn)',
  blocked: 'var(--hot)',
  waiting: '#3b82f6',
};

function severityIcon(s: Severity, size = 14) {
  switch (s) {
    case 'ready': return <CheckCircle2 size={size} />;
    case 'warning': return <AlertTriangle size={size} />;
    case 'blocked': return <ShieldAlert size={size} />;
    case 'waiting': return <Clock size={size} />;
  }
}

interface Props {
  account: AccountReadiness;
  isAdmin: boolean;
  onClearBlock?: (accountId: number) => void;
  clearing?: boolean;
  multiAccount?: boolean;
}

export function AccountHealthCard({ account, isAdmin, onClearBlock, clearing, multiAccount }: Props) {
  const [showTech, setShowTech] = useState(false);
  const allReasons = Array.from(new Set(account.capabilities.flatMap(c => c.reasons ?? [])));
  const status = overallStatus(allReasons);
  const color = SEVERITY_COLOR[status.severity];
  const identity = account.fb_display_name || account.account_name
    || (account.fb_user_id ? `FB ${account.fb_user_id}` : `Tài khoản #${account.account_id}`);
  // Customer-friendly Chrome profile line — the human label (set at pairing) or a
  // plain "Chưa đặt tên". Raw ids/version live under "Chi tiết kỹ thuật".
  // TODO(PR-C pairing form): when a set-machine-label API exists, add a small
  // "Đặt tên profile" CTA here.
  const profileLabel = `Chrome profile: ${account.machine_label || 'Chưa đặt tên'}`;
  const isActorBlocked = allReasons.includes('actor_mismatch_blocked');

  return (
    <div className="card" style={{ display: 'flex', flexDirection: 'column', gap: 12, padding: 'var(--s-4)', borderLeft: `3px solid ${color}` }}>
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
        <div style={{ minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <Monitor size={15} color="var(--text-mute)" />
            <span style={{ fontWeight: 600, fontSize: 14, color: 'var(--text)' }}>{identity}</span>
          </div>
          <div style={{ fontSize: 12, color: 'var(--text-mute)', marginTop: 5, display: 'flex', flexDirection: 'column', gap: 3 }}>
            {account.assigned_user_name && (
              <span style={{ display: 'inline-flex', alignItems: 'center', gap: 5 }}><User size={12} /> Người quản lý: {account.assigned_user_name}</span>
            )}
            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 5 }}><Globe size={12} /> {profileLabel}</span>
            {!account.machine_label && multiAccount && (
              <span style={{ fontSize: 11, color: 'var(--text-faint)', paddingLeft: 17 }}>Đặt tên profile giúp dễ quản lý nhiều tài khoản.</span>
            )}
          </div>
        </div>
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, color, background: `color-mix(in srgb, ${color} 12%, transparent)`, padding: '4px 10px', borderRadius: 999, fontSize: 12, fontWeight: 600, whiteSpace: 'nowrap' }}>
          {severityIcon(status.severity)} {status.label}
        </span>
      </div>

      <div style={{ fontSize: 11.5, color: 'var(--text-faint)' }}>Agent có thể dùng tài khoản này cho các tác vụ:</div>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginTop: -4 }}>
        {account.capabilities.map(cap => {
          const reasonCode = (cap.reasons ?? [])[0];
          const sev: Severity = cap.can ? 'ready' : mapReason(reasonCode || '').severity;
          const c = SEVERITY_COLOR[sev];
          return (
            <span key={cap.capability} title={cap.can ? 'Sẵn sàng' : mapReason(reasonCode || '').title}
              style={{ display: 'inline-flex', alignItems: 'center', gap: 5, fontSize: 12, color: c, background: `color-mix(in srgb, ${c} 10%, transparent)`, padding: '3px 9px', borderRadius: 8 }}>
              {cap.can ? <CheckCircle2 size={11} /> : severityIcon(sev, 11)}
              {CAPABILITY_LABELS[cap.capability] || cap.capability}
            </span>
          );
        })}
      </div>

      {status.primary && (
        <div style={{ display: 'flex', gap: 10, alignItems: 'flex-start', background: 'var(--bg-elev-2)', borderRadius: 8, padding: '10px 12px' }}>
          <span style={{ color, flexShrink: 0, marginTop: 1 }}>{severityIcon(status.severity, 16)}</span>
          <div style={{ minWidth: 0 }}>
            <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text)' }}>{status.primary.title}</div>
            <div style={{ fontSize: 12.5, color: 'var(--text-mute)', marginTop: 2 }}>{status.primary.description}</div>
            <div style={{ fontSize: 12.5, color: 'var(--text)', marginTop: 6, display: 'flex', gap: 6, alignItems: 'flex-start' }}>
              <Wrench size={13} style={{ flexShrink: 0, marginTop: 2, color: 'var(--text-mute)' }} />
              <span><strong>Cần làm:</strong> {status.primary.action}</span>
            </div>
          </div>
        </div>
      )}

      {(isActorBlocked && isAdmin && onClearBlock) && (
        <div>
          <button type="button" className="btn btn-ghost btn-sm" disabled={clearing} onClick={() => onClearBlock(account.account_id)}>
            {clearing ? 'Đang gỡ...' : 'Gỡ chặn tài khoản (admin)'}
          </button>
        </div>
      )}

      {isAdmin && (
        <div style={{ borderTop: '1px solid var(--line)', paddingTop: 8 }}>
          <button type="button" onClick={() => setShowTech(v => !v)}
            style={{ background: 'transparent', border: 0, padding: 0, cursor: 'pointer', display: 'inline-flex', alignItems: 'center', gap: 4, color: 'var(--text-mute)', fontSize: 11.5 }}>
            <ChevronDown size={12} style={{ transform: showTech ? 'rotate(180deg)' : 'none', transition: 'transform .15s' }} />
            Chi tiết kỹ thuật
          </button>
          {showTech && (
            <div className="mono" style={{ fontSize: 11, color: 'var(--text-mute)', marginTop: 6, lineHeight: 1.7 }}>
              <div>account_id: {account.account_id}</div>
              <div>connector_id: {account.connector_id || '—'}</div>
              <div>fb_user_id: {account.fb_user_id || '—'}</div>
              <div>browser_profile_id: {account.browser_profile_id || '—'}</div>
              <div>extension_version: {account.extension_version || '—'}</div>
              <div>reason_codes: {allReasons.length ? allReasons.join(', ') : 'ready'}</div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
