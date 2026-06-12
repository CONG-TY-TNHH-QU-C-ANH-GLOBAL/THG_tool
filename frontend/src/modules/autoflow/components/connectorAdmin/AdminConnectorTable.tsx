'use client';
import { useCallback, useEffect, useState } from 'react';
import { Pause, Play, RefreshCw, Copy } from 'lucide-react';
import {
  getConnectorOverview,
  pauseAccountAssignment,
  resumeAccountAssignment,
  versionStateLabel,
  UPDATE_INSTRUCTIONS_VI,
  type ConnectorOverviewRow,
} from '../../services/connectorAdminService';
import { mapReason } from '../accountHealth/reasonMessages';

const toneColor = { ok: 'var(--ok)', warn: 'var(--info)', blocked: 'var(--hot)' } as const;

function lastSeenLabel(iso: string): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString('vi-VN', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' });
}

/**
 * Admin-only workspace connector table (PR-3): staff → Facebook account
 * → connector/version/readiness, with the pause/resume assignment
 * safety switch. View-only over devices — no pair/unpair, no
 * impersonation, no cookies/session data (backend guarantees).
 */
export default function AdminConnectorTable() {
  const [rows, setRows] = useState<ConnectorOverviewRow[]>([]);
  const [busyId, setBusyId] = useState<number | null>(null);
  const [error, setError] = useState('');

  const refresh = useCallback(() => {
    getConnectorOverview().then(setRows).catch(e => setError(e instanceof Error ? e.message : 'load_failed'));
  }, []);

  useEffect(() => { refresh(); }, [refresh]);

  async function togglePause(row: ConnectorOverviewRow) {
    setBusyId(row.account_id);
    setError('');
    try {
      if (row.assignment_paused) await resumeAccountAssignment(row.account_id);
      else await pauseAccountAssignment(row.account_id);
      refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'action_failed');
    } finally {
      setBusyId(null);
    }
  }

  const th: React.CSSProperties = { textAlign: 'left', padding: '8px 10px', fontSize: 11, color: 'var(--text-faint)', borderBottom: '1px solid var(--line)', whiteSpace: 'nowrap' };
  const td: React.CSSProperties = { padding: '8px 10px', fontSize: 12.5, color: 'var(--text)', borderBottom: '1px solid var(--line)', verticalAlign: 'top' };

  return (
    <div className="card" style={{ padding: 0, overflowX: 'auto' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '10px 12px' }}>
        <strong style={{ fontSize: 13 }}>Facebook accounts / connectors trong workspace</strong>
        <div style={{ display: 'flex', gap: 8 }}>
          <button type="button" className="btn btn-ghost btn-sm" onClick={() => void navigator.clipboard?.writeText(UPDATE_INSTRUCTIONS_VI)}>
            <Copy size={12} /> Copy hướng dẫn cập nhật
          </button>
          <button type="button" className="btn btn-ghost btn-sm" onClick={refresh}><RefreshCw size={12} /> Làm mới</button>
        </div>
      </div>
      {error && <p style={{ color: 'var(--hot)', fontSize: 12, padding: '0 12px 8px' }}>{error}</p>}
      <table style={{ width: '100%', borderCollapse: 'collapse' }}>
        <thead>
          <tr>
            <th style={th}>Nhân viên</th>
            <th style={th}>Facebook</th>
            <th style={th}>Connector</th>
            <th style={th}>Last seen</th>
            <th style={th}>Extension</th>
            <th style={th}>Trạng thái</th>
            <th style={th}>Automation</th>
            <th style={th}></th>
          </tr>
        </thead>
        <tbody>
          {rows.map(r => {
            const vs = versionStateLabel(r.extension_version_state);
            const primaryBlock = r.block_reasons[0] ? mapReason(r.block_reasons[0]) : null;
            return (
              <tr key={r.account_id}>
                <td style={td}>
                  <div>{r.staff_name || '— chưa gán —'}</div>
                  <div style={{ fontSize: 11, color: 'var(--text-faint)' }}>{r.staff_email}{r.staff_role ? ` · ${r.staff_role}` : ''}</div>
                </td>
                <td style={td}>
                  <div>{r.fb_display_name || r.account_name}</div>
                  <div style={{ fontSize: 11, color: 'var(--text-faint)' }}>#{r.account_id}</div>
                </td>
                <td style={td}>
                  <span style={{ color: r.connector_online ? 'var(--ok)' : 'var(--text-faint)' }}>
                    {r.connector_online ? '● online' : '○ offline'}
                  </span>
                </td>
                <td style={td}>{lastSeenLabel(r.last_seen)}</td>
                <td style={td}>
                  <div>{r.extension_version || '—'}</div>
                  <div style={{ fontSize: 11, color: toneColor[vs.tone] }}>{vs.label}</div>
                </td>
                <td style={td}>
                  {primaryBlock
                    ? <span title={r.block_reasons.join(', ')} style={{ color: toneColor.blocked, fontSize: 12 }}>{primaryBlock.title}</span>
                    : <span style={{ color: 'var(--ok)', fontSize: 12 }}>Sẵn sàng</span>}
                  {(r.extension_version_state === 'update_required' || r.extension_version_state === 'unsupported') && (
                    <div style={{ fontSize: 11, color: 'var(--text-faint)' }}>Automation paused until staff updates extension.</div>
                  )}
                </td>
                <td style={td}>{r.automation_eligible ? '✓' : '—'}</td>
                <td style={td}>
                  <button
                    type="button"
                    className="btn btn-ghost btn-sm"
                    disabled={busyId === r.account_id}
                    onClick={() => void togglePause(r)}
                    title={r.assignment_paused ? 'Mở lại giao task' : 'Tạm dừng giao task (an toàn)'}
                  >
                    {r.assignment_paused ? <><Play size={12} /> Resume</> : <><Pause size={12} /> Pause</>}
                  </button>
                </td>
              </tr>
            );
          })}
          {rows.length === 0 && (
            <tr><td style={{ ...td, color: 'var(--text-faint)' }} colSpan={8}>Chưa có Facebook account nào trong workspace.</td></tr>
          )}
        </tbody>
      </table>
    </div>
  );
}
