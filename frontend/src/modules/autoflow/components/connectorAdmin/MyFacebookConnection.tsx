'use client';
import { useEffect, useState } from 'react';
import { MonitorPlay, RefreshCw } from 'lucide-react';
import { get } from '../../services/api';
import { versionStateLabel, UPDATE_INSTRUCTIONS_VI } from '../../services/connectorAdminService';
import { mapReason } from '../accountHealth/reasonMessages';

interface ReadinessAccount {
  account_id: number;
  account_name: string;
  fb_display_name: string;
  connector_id: number;
  machine_label: string;
  extension_version: string;
  extension_version_state: string;
  required_action: string;
  capabilities: { capability: string; can: boolean; reasons: string[] }[];
}

const toneColor = { ok: 'var(--ok)', warn: 'var(--info)', blocked: 'var(--hot)' } as const;

/**
 * Staff "My Facebook connection" (PR-3): the member's OWN account(s) +
 * connector + version status + update/pair instructions. The backend
 * readiness matrix is already owner-scoped (CanViewAccountDevice), so
 * this view can never show a colleague's device.
 */
export default function MyFacebookConnection() {
  const [accounts, setAccounts] = useState<ReadinessAccount[]>([]);
  const [loading, setLoading] = useState(true);

  const refresh = () => {
    setLoading(true);
    get<{ accounts?: ReadinessAccount[] }>('/accounts/readiness')
      .then(r => setAccounts(r.accounts ?? []))
      .catch(() => {})
      .finally(() => setLoading(false));
  };

  useEffect(refresh, []);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <strong style={{ fontSize: 13 }}>Kết nối Facebook của tôi</strong>
        <button type="button" className="btn btn-ghost btn-sm" onClick={refresh}><RefreshCw size={12} /> Làm mới</button>
      </div>

      {loading && <div className="skeleton" style={{ height: 60 }} />}
      {!loading && accounts.length === 0 && (
        <div className="card" style={{ padding: 16 }}>
          <p style={{ fontSize: 12.5, color: 'var(--text-mute)', margin: 0 }}>
            Bạn chưa kết nối Facebook account nào. Vào tab <strong>Browser</strong>, tạo mã kết nối và pair THG Connector
            trên Chrome của bạn (Chrome profile riêng của bạn — admin không thao tác hộ).
          </p>
        </div>
      )}

      {accounts.map(acc => {
        const vs = versionStateLabel(acc.extension_version_state);
        const blockers = acc.capabilities.flatMap(c => c.reasons);
        const primary = blockers[0] ? mapReason(blockers[0]) : null;
        const needsUpdate = acc.extension_version_state === 'update_required' || acc.extension_version_state === 'unsupported';
        const softUpdate = acc.extension_version_state === 'update_available';
        return (
          <div key={acc.account_id} className="card" style={{ padding: 14 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
              <MonitorPlay size={14} color="var(--accent)" />
              <strong style={{ fontSize: 13 }}>{acc.fb_display_name || acc.account_name}</strong>
              <span style={{ fontSize: 11, color: acc.connector_id > 0 ? 'var(--ok)' : 'var(--text-faint)' }}>
                {acc.connector_id > 0 ? '● online' : '○ offline'}
              </span>
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(160px, 1fr))', gap: 8, fontSize: 12 }}>
              <div>
                <div style={{ color: 'var(--text-faint)', fontSize: 11 }}>Extension</div>
                <div>{acc.extension_version || '—'} <span style={{ color: toneColor[vs.tone] }}>({vs.label})</span></div>
              </div>
              <div>
                <div style={{ color: 'var(--text-faint)', fontSize: 11 }}>Thiết bị</div>
                <div>{acc.machine_label || 'Chrome của tôi'}</div>
              </div>
              <div>
                <div style={{ color: 'var(--text-faint)', fontSize: 11 }}>Trạng thái automation</div>
                <div style={{ color: primary ? toneColor.blocked : 'var(--ok)' }}>{primary ? primary.title : 'Sẵn sàng nhận task'}</div>
              </div>
            </div>
            {softUpdate && (
              <p style={{ fontSize: 12, color: 'var(--info)', margin: '10px 0 0' }}>
                Có bản cập nhật extension mới. Bạn vẫn có thể dùng, nhưng nên cập nhật để ổn định hơn.
              </p>
            )}
            {needsUpdate && (
              <p style={{ fontSize: 12, color: 'var(--hot)', margin: '10px 0 0' }}>
                {acc.extension_version_state === 'unsupported'
                  ? 'Phiên bản extension này không còn được hỗ trợ. Vui lòng cài phiên bản mới.'
                  : 'Automation đang tạm dừng vì extension của bạn đã cũ. Cập nhật extension để tiếp tục nhận task.'}
              </p>
            )}
            {(needsUpdate || softUpdate) && (
              <p style={{ fontSize: 11.5, color: 'var(--text-faint)', margin: '6px 0 0' }}>{UPDATE_INSTRUCTIONS_VI}</p>
            )}
            {primary && !needsUpdate && (
              <p style={{ fontSize: 11.5, color: 'var(--text-faint)', margin: '8px 0 0' }}>{primary.action}</p>
            )}
          </div>
        );
      })}
    </div>
  );
}
