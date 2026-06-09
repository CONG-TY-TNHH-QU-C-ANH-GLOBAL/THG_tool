'use client';

import type { CSSProperties } from 'react';
import { ArrowRight, CheckCircle2, ListChecks, Search, Star } from 'lucide-react';
import type { AccountReadiness } from './types';
import { mapReason, overallStatus } from './reasonMessages';

interface Props {
  accounts: AccountReadiness[];
  onNavigate?: (tab: string) => void;
  defaultAccountId: number;
  onSetDefault: (accountId: number) => void;
  settingDefault: boolean;
}

const linkBtn: CSSProperties = {
  background: 'transparent', border: 0, padding: 0, cursor: 'pointer',
  color: 'var(--text-mute)', fontSize: 12, display: 'inline-flex', alignItems: 'center', gap: 5,
};

function reasonsOf(a: AccountReadiness): string[] {
  return Array.from(new Set(a.capabilities.flatMap(c => c.reasons ?? [])));
}
function nameOf(a: AccountReadiness): string {
  return a.fb_display_name || a.account_name || `Tài khoản #${a.account_id}`;
}

export function NextStepsPanel({ accounts, onNavigate, defaultAccountId, onSetDefault, settingDefault }: Props) {
  const ready = accounts.filter(a => overallStatus(reasonsOf(a)).severity === 'ready');
  const needsWork = accounts.filter(a => overallStatus(reasonsOf(a)).severity !== 'ready');
  const defaultIsReady = ready.some(a => a.account_id === defaultAccountId);

  return (
    <div className="card" style={{ padding: 'var(--s-4)', display: 'flex', flexDirection: 'column', gap: 14 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <ListChecks size={16} color="var(--ok)" />
        <span style={{ fontSize: 14, fontWeight: 600 }}>Bước tiếp theo</span>
      </div>

      {ready.length > 0 ? (
        <>
          <div style={{ display: 'flex', alignItems: 'flex-start', gap: 9 }}>
            <CheckCircle2 size={18} color="var(--ok)" style={{ flexShrink: 0, marginTop: 1 }} />
            <div>
              <div style={{ fontSize: 13.5, fontWeight: 600 }}>
                Facebook đã sẵn sàng{ready.length === 1 ? `: ${nameOf(ready[0])}` : ` · ${ready.length} tài khoản`}
              </div>
              <div style={{ fontSize: 12.5, color: 'var(--text-mute)', marginTop: 2 }}>
                Bạn có thể bắt đầu tạo nhiệm vụ tìm lead, hoặc dùng tài khoản này cho comment / inbox / đăng bài.
              </div>
            </div>
          </div>

          <button type="button" className="btn btn-primary btn-sm" style={{ width: '100%', justifyContent: 'center' }} onClick={() => onNavigate?.('missions')}>
            <Search size={13} /> Tạo nhiệm vụ tìm khách
          </button>

          <div style={{ display: 'flex', flexDirection: 'column', gap: 9 }}>
            <button type="button" style={linkBtn} onClick={() => onNavigate?.('leads')}>Đi tới Leads <ArrowRight size={12} /></button>
            {defaultIsReady ? (
              <span style={{ fontSize: 12, color: 'var(--ok)', display: 'inline-flex', alignItems: 'center', gap: 5 }}><Star size={12} /> Đang là tài khoản mặc định</span>
            ) : (
              <button type="button" style={linkBtn} disabled={settingDefault} onClick={() => onSetDefault(ready[0].account_id)}>
                <Star size={12} /> {settingDefault ? 'Đang đặt...' : 'Đặt làm tài khoản mặc định'}
              </button>
            )}
          </div>
        </>
      ) : (
        <div style={{ fontSize: 12.5, color: 'var(--text-mute)' }}>
          {accounts.length === 0 ? 'Chưa có tài khoản. Bấm “Kết nối Facebook mới” để bắt đầu.' : 'Chưa có tài khoản nào sẵn sàng để tự động hoá.'}
        </div>
      )}

      {needsWork.length > 0 && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8, borderTop: ready.length > 0 ? '1px solid var(--line)' : 'none', paddingTop: ready.length > 0 ? 12 : 0 }}>
          <div style={{ fontSize: 12.5, fontWeight: 600, color: 'var(--text)' }}>Cần xử lý</div>
          {needsWork.slice(0, 4).map(a => {
            const primaryCode = overallStatus(reasonsOf(a)).primary?.technical_code;
            const action = primaryCode ? mapReason(primaryCode).action : a.required_action;
            return (
              <div key={a.account_id} style={{ fontSize: 12, color: 'var(--text-mute)' }}>
                <strong style={{ color: 'var(--text)' }}>{nameOf(a)}:</strong> {action || 'Kiểm tra tài khoản.'}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
