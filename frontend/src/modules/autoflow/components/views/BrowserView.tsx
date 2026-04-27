import { theme, cardStyle } from '../../constants/styles';
import { Row } from '../ui';
import { useFacebookSession } from '../../hooks/useFacebookSession';
import { LogIn, RefreshCw, Check } from 'lucide-react';
import '../../autoflow.css';

interface BrowserViewProps { orgId: string; }

export default function BrowserView({ orgId }: BrowserViewProps) {
  const { status, isConnecting, connect, disconnect } = useFacebookSession(orgId);
  const conn = status.connected;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
      <div style={{
        display: 'flex', alignItems: 'center', gap: 10, padding: '11px 14px',
        borderRadius: 11, border: `1px solid ${conn ? '#16a34a55' : theme.border}`,
        background: conn ? '#052e1611' : theme.surface,
      }}>
        <div style={{ width: 7, height: 7, borderRadius: '50%', background: conn ? '#4ade80' : theme.textFaint }} />
        <p style={{ fontSize: 13, color: conn ? '#4ade80' : theme.textMuted, fontWeight: 500 }}>
          {conn ? 'Session đang hoạt động — Facebook kết nối thành công' : 'Chưa có session — Vui lòng kết nối Facebook'}
        </p>
        {conn && (
          <span style={{ marginLeft: 'auto', fontSize: 11, color: '#4ade80', background: '#052e1633', border: '1px solid #16a34a44', padding: '3px 9px', borderRadius: 7 }}>
            Expires: {status.expiresLabel}
          </span>
        )}
      </div>

      <div style={{ background: theme.surface, border: `1px solid ${theme.border}`, borderRadius: 12, overflow: 'hidden' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 7, padding: '9px 14px', background: theme.surfaceAlt, borderBottom: `1px solid ${theme.border}` }}>
          <Row style={{ gap: 5 }}>
            {['#ef4444','#f59e0b','#22c55e'].map(c => <div key={c} style={{ width: 10, height: 10, borderRadius: '50%', background: c }} />)}
          </Row>
          <div style={{ flex: 1, display: 'flex', alignItems: 'center', gap: 7, background: theme.surface, borderRadius: 7, padding: '4px 11px', margin: '0 10px' }}>
            <span style={{ fontSize: 12 }}>🔒</span>
            <span style={{ color: theme.textFaint, fontSize: 12 }}>https://www.facebook.com</span>
          </div>
          {conn && (
            <button onClick={disconnect} style={{ background: 'none', border: 'none', color: theme.textFaint, fontSize: 11, cursor: 'pointer' }}>Reset</button>
          )}
        </div>

        <div style={{ minHeight: 240, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 28 }}>
          {!conn ? (
            <div style={{ textAlign: 'center' }}>
              <div style={{ width: 60, height: 60, background: theme.facebook, borderRadius: 16, display: 'flex', alignItems: 'center', justifyContent: 'center', margin: '0 auto 14px' }}>
                <span style={{ color: '#fff', fontSize: 28, fontWeight: 900 }}>f</span>
              </div>
              <p style={{ color: '#f9fafb', fontWeight: 600, fontSize: 16, marginBottom: 7 }}>Kết nối tài khoản Facebook</p>
              <p style={{ color: theme.textMuted, fontSize: 13, marginBottom: 22, maxWidth: 260 }}>
                Đăng nhập để AutoFlow thu thập leads và chạy AI agents trong các nhóm của bạn
              </p>
              <button onClick={connect} disabled={isConnecting} style={{
                ...cardStyle(),
                border: 'none', cursor: isConnecting ? 'not-allowed' : 'pointer',
                background: isConnecting ? '#374151' : theme.facebook,
                display: 'inline-flex', alignItems: 'center', gap: 7,
                color: '#fff', fontSize: 14, padding: '10px 20px',
              }}>
                {isConnecting
                  ? <><RefreshCw size={14} className="spin" />Đang kết nối...</>
                  : <><LogIn size={14} />Đăng nhập Facebook</>
                }
              </button>
            </div>
          ) : (
            <div style={{ textAlign: 'center' }}>
              <div style={{ width: 60, height: 60, background: theme.greenDark, borderRadius: '50%', display: 'flex', alignItems: 'center', justifyContent: 'center', margin: '0 auto 14px' }}>
                <Check size={28} color="#fff" />
              </div>
              <p style={{ color: '#f9fafb', fontWeight: 600, fontSize: 16, marginBottom: 5 }}>Kết nối thành công!</p>
              <p style={{ color: theme.textMuted, fontSize: 13, marginBottom: 18 }}>
                Tài khoản: <strong style={{ color: '#fff' }}>{status.account}</strong>
              </p>
              <Row style={{ gap: 11, justifyContent: 'center' }}>
                {[
                  { l: 'Nhóm', v: String(status.groups ?? 0) },
                  { l: 'Leads hôm nay', v: String(status.leadsToday ?? 0) },
                  { l: 'Agents', v: String(status.agents ?? 0) },
                ].map(s => (
                  <div key={s.l} style={{ background: theme.border, borderRadius: 9, padding: '9px 16px', textAlign: 'center' }}>
                    <p style={{ color: '#fff', fontWeight: 700, fontSize: 17 }}>{s.v}</p>
                    <p style={{ color: theme.textMuted, fontSize: 10 }}>{s.l}</p>
                  </div>
                ))}
              </Row>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
