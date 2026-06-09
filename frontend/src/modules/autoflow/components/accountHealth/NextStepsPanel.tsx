'use client';

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

function accountReasons(a: AccountReadiness): string[] {
  return Array.from(new Set(a.capabilities.flatMap(c => c.reasons ?? [])));
}

export function NextStepsPanel({ accounts, onNavigate, defaultAccountId, onSetDefault, settingDefault }: Props) {
  const ready = accounts.filter(a => overallStatus(accountReasons(a)).severity === 'ready');
  const needsWork = accounts.filter(a => overallStatus(accountReasons(a)).severity !== 'ready');

  return (
    <div className="card" style={{ padding: 'var(--s-4)', display: 'flex', flexDirection: 'column', gap: 14 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <ListChecks size={16} color="var(--accent, var(--ok))" />
        <span style={{ fontSize: 14, fontWeight: 600 }}>Bước tiếp theo</span>
      </div>

      {ready.length > 0 ? (
        <>
          <div style={{ display: 'flex', alignItems: 'flex-start', gap: 9 }}>
            <CheckCircle2 size={18} color="var(--ok)" style={{ flexShrink: 0, marginTop: 1 }} />
            <div>
              <div style={{ fontSize: 13.5, fontWeight: 600 }}>Facebook đã sẵn sàng</div>
              <div style={{ fontSize: 12.5, color: 'var(--text-mute)', marginTop: 2 }}>
                Bạn có thể bắt đầu tạo nhiệm vụ tìm lead, hoặc dùng tài khoản này cho comment / inbox / đăng bài.
              </div>
            </div>
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            <button type="button" className="btn btn-primary btn-sm" onClick={() => onNavigate?.('missions')}>
              <Search size={13} /> Tạo nhiệm vụ tìm khách
            </button>
            <button type="button" className="btn btn-ghost btn-sm" onClick={() => onNavigate?.('leads')}>
              Đi tới Leads <ArrowRight size={13} />
            </button>
            {defaultAccountId === 0 && (
              <button type="button" className="btn btn-ghost btn-sm" disabled={settingDefault} onClick={() => onSetDefault(ready[0].account_id)}>
                <Star size={13} /> {settingDefault ? 'Đang đặt...' : 'Đặt làm tài khoản mặc định'}
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
            const primaryCode = overallStatus(accountReasons(a)).primary?.technical_code;
            const action = primaryCode ? mapReason(primaryCode).action : a.required_action;
            const name = a.fb_display_name || a.account_name || `Tài khoản #${a.account_id}`;
            return (
              <div key={a.account_id} style={{ fontSize: 12, color: 'var(--text-mute)' }}>
                <strong style={{ color: 'var(--text)' }}>{name}:</strong> {action || 'Kiểm tra tài khoản.'}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
