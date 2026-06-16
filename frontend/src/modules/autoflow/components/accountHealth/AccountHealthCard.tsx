'use client';

import { useState } from 'react';
import { AlertTriangle, CheckCircle2, ChevronDown, Clock, Globe, Monitor, ShieldAlert, User, Wrench } from 'lucide-react';
import type { AccountReadiness } from './types';
import { CAPABILITY_LABELS } from './types';
import { accountExecState, mapReason, overallStatus, type Severity } from './reasonMessages';

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
  // P1.3E: the account headline is driven STRICTLY by requester-scoped executability — green
  // "Sẵn sàng" only when the current user's OWN live connector can run THIS account now. The
  // org-wide per-capability health (cooldown/daily) still surfaces on the per-task badges below.
  const exec = accountExecState(account);
  const health = overallStatus(allReasons); // per-cap pacing detail (cooldown/daily) when executable
  const status = exec;
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
          // A task is runnable ONLY when the account is executable AND the capability is allowed.
          // If the account is not executable, every task badge shows the account-level block —
          // never green — so the card can't say "not connected" while a task stays green.
          const reasonCode = (cap.reasons ?? [])[0];
          const runnable = !!account.executable && cap.can;
          const sev: Severity = runnable ? 'ready' : (!account.executable ? exec.severity : mapReason(reasonCode || '').severity);
          const title = runnable ? 'Sẵn sàng' : (!account.executable ? exec.label : mapReason(reasonCode || '').title);
          const c = SEVERITY_COLOR[sev];
          return (
            <span key={cap.capability} title={title}
              style={{ display: 'inline-flex', alignItems: 'center', gap: 5, fontSize: 12, color: c, background: `color-mix(in srgb, ${c} 10%, transparent)`, padding: '3px 9px', borderRadius: 8, opacity: runnable ? 1 : 0.85 }}>
              {runnable ? <CheckCircle2 size={11} /> : severityIcon(sev, 11)}
              {CAPABILITY_LABELS[cap.capability] || cap.capability}
            </span>
          );
        })}
      </div>

      {(() => {
        // Detail block: when not executable → the typed connector/control reason; when
        // executable but a capability is paced (cooldown/daily) → that health detail.
        const detail = !account.executable
          ? { title: exec.title, description: exec.description, action: exec.action, severity: exec.severity }
          : (health.primary ? { title: health.primary.title, description: health.primary.description, action: health.primary.action, severity: health.severity } : null);
        if (!detail) return null;
        const dColor = SEVERITY_COLOR[detail.severity];
        return (
          <div style={{ display: 'flex', gap: 10, alignItems: 'flex-start', background: 'var(--bg-elev-2)', borderRadius: 8, padding: '10px 12px' }}>
            <span style={{ color: dColor, flexShrink: 0, marginTop: 1 }}>{severityIcon(detail.severity, 16)}</span>
            <div style={{ minWidth: 0 }}>
              <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text)' }}>{detail.title}</div>
              <div style={{ fontSize: 12.5, color: 'var(--text-mute)', marginTop: 2 }}>{detail.description}</div>
              {detail.action && (
                <div style={{ fontSize: 12.5, color: 'var(--text)', marginTop: 6, display: 'flex', gap: 6, alignItems: 'flex-start' }}>
                  <Wrench size={13} style={{ flexShrink: 0, marginTop: 2, color: 'var(--text-mute)' }} />
                  <span><strong>Cần làm:</strong> {detail.action}</span>
                </div>
              )}
            </div>
          </div>
        );
      })()}

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
              <div>executable: {String(account.executable ?? false)} ({account.exec_reason_code || '—'})</div>
              <div>control_allowed: {String(account.control_allowed ?? false)} · paired: {String(account.paired ?? false)} · online: {String(account.connector_online ?? false)} · identity_matched: {String(account.live_identity_matched ?? false)}</div>
              <div>reason_codes: {allReasons.length ? allReasons.join(', ') : 'ready'}</div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
