import { useEffect, useState, useCallback } from 'react';
import { Users } from 'lucide-react';
import {
  getConnectorStatus,
  type ConnectorAccountStatus,
  type ConnectorAccountState,
} from '../../services/connectorService';

/**
 * PR-M2 Account Presence Board.
 *
 * The connector cards below show per-EXTENSION status. With multiple Facebook
 * accounts + members, the operator needs the inverse view: per-ACCOUNT, is it
 * REACHABLE (an online extension is logged into the right FB), and which member
 * owns it. This board answers "which of my accounts can run automation now"
 * before a command is issued. Self-fetching + polls every 15s.
 */
const STATE_META: Record<ConnectorAccountState, { label: string; tag: string }> = {
  online:        { label: 'Sẵn sàng',            tag: 'tag-ok' },
  logged_out:    { label: 'Chưa đăng nhập FB',    tag: 'tag-warm' },
  wrong_account: { label: 'Sai tài khoản FB',     tag: 'tag-hot' },
  offline:       { label: 'Extension offline',    tag: 'tag-mute' },
  no_connector:  { label: 'Chưa kết nối',         tag: 'tag-mute' },
  unassigned:    { label: 'Chưa gán account',     tag: 'tag-warm' },
};

export function AccountPresenceBoard() {
  const [rows, setRows] = useState<ConnectorAccountStatus[]>([]);
  const [reachable, setReachable] = useState(0);
  const [total, setTotal] = useState(0);
  const [err, setErr] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const res = await getConnectorStatus();
      setRows(res.accounts ?? []);
      setReachable(res.reachable_total ?? 0);
      setTotal(res.accounts_total ?? 0);
      setErr(null);
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Không tải được trạng thái connector');
    }
  }, []);

  useEffect(() => {
    void load();
    const t = window.setInterval(() => void load(), 15000);
    return () => window.clearInterval(t);
  }, [load]);

  if (err) return null; // fail quiet — the connector cards below still render
  if (rows.length === 0) return null;

  return (
    <div className="card" style={{ padding: 'var(--s-4) var(--s-5)' }}>
      <header style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 'var(--s-3)' }}>
        <Users size={15} style={{ color: 'var(--accent)' }} />
        <h3 style={{ margin: 0, fontSize: 14, fontWeight: 600, color: 'var(--text)' }}>
          Trạng thái tài khoản
        </h3>
        <span className={`tag ${reachable > 0 ? 'tag-ok' : 'tag-mute'}`} style={{ marginLeft: 'auto' }}>
          {reachable}/{total} sẵn sàng chạy
        </span>
      </header>
      <div style={{ display: 'grid', gap: 6 }}>
        {rows.map((r) => {
          const meta = STATE_META[r.state] ?? STATE_META.no_connector;
          return (
            <div
              key={r.account_id}
              style={{
                display: 'flex', alignItems: 'center', gap: 10,
                padding: '8px 10px', borderRadius: 'var(--radius-sm)',
                background: 'var(--bg-elev)', border: '1px solid var(--line)',
              }}
            >
              <span style={{
                width: 8, height: 8, borderRadius: '50%', flexShrink: 0,
                background: r.reachable ? 'var(--ok)' : r.state === 'wrong_account' ? 'var(--hot)' : 'var(--text-faint)',
              }} />
              <strong style={{ fontSize: 13, color: 'var(--text)', minWidth: 120 }}>
                {r.account_name || `Account #${r.account_id}`}
              </strong>
              <span style={{ fontSize: 12, color: 'var(--text-mute)', flex: 1 }}>
                {r.assigned_user_name ? `Chủ: ${r.assigned_user_name}` : 'Chưa gán chủ'}
                {r.connector_fb_user_id ? ` · FB ${r.connector_fb_user_id}` : ''}
              </span>
              <span className={`tag ${meta.tag}`}>{meta.label}</span>
            </div>
          );
        })}
      </div>
      {rows.some((r) => r.state === 'wrong_account') && (
        <p style={{ margin: '10px 0 0', fontSize: 11.5, color: 'var(--hot)', lineHeight: 1.5 }}>
          "Sai tài khoản FB": extension đang đăng nhập FB khác với account — mở Chrome profile đúng tài khoản đó rồi kết nối lại.
        </p>
      )}
    </div>
  );
}
