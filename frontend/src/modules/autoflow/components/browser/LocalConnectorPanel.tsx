import { useEffect, useState } from 'react';
import { CheckCircle, Copy, Eye, EyeOff, KeyRound, Laptop, Radio, RefreshCw, Shield, Unplug } from 'lucide-react';
import { theme } from '../../constants/styles';
import type { SystemInfo } from '../../services/systemService';
import type { LocalConnector } from '../../types';
import { connectorStatusLabel, facebookIdentityLabel, formatCountdown, formatLastSeen, isDashboardStreamConnector, RUNTIME_DOWNLOADS } from './browserHelpers';
export function LocalConnectorPanel({
  connectors,
  creating,
  pairingCode,
  pairingExpiresAt,
  systemInfo,
  currentUserId,
  currentUserRole,
  disconnectingId,
  onCreate,
  onDisconnect,
}: {
  connectors: LocalConnector[];
  creating: boolean;
  pairingCode: string;
  pairingExpiresAt: string;
  systemInfo: SystemInfo | null;
  currentUserId: number;
  currentUserRole: string;
  disconnectingId: number | null;
  onCreate: () => void;
  onDisconnect: (connector: LocalConnector) => void;
}) {
  const online = connectors.filter(c => c.online).length;
  const runtimeOnline = connectors.filter(c => c.online && isDashboardStreamConnector(c)).length;
  const [setupOpen, setSetupOpen] = useState(connectors.length === 0);
  const [pairingCodeVisible, setPairingCodeVisible] = useState(false);
  const [pairingRemainingMs, setPairingRemainingMs] = useState<number | null>(null);
  const [dashboardServer, setDashboardServer] = useState('');
  const pairingExpired = pairingCode !== '' && pairingRemainingMs !== null && pairingRemainingMs <= 0;

  useEffect(() => {
    setPairingCodeVisible(false);
  }, [pairingCode]);

  useEffect(() => {
    if (!pairingExpiresAt) {
      setPairingRemainingMs(null);
      return;
    }
    const expiresAt = new Date(pairingExpiresAt).getTime();
    if (Number.isNaN(expiresAt)) {
      setPairingRemainingMs(null);
      return;
    }
    const tick = () => setPairingRemainingMs(Math.max(0, expiresAt - Date.now()));
    tick();
    const timer = window.setInterval(tick, 1000);
    return () => window.clearInterval(timer);
  }, [pairingExpiresAt]);

  useEffect(() => {
    if (typeof window !== 'undefined') {
      setDashboardServer(window.location.origin);
    }
  }, []);

  return (
    <div className="af-runtime-panel" style={{ background: theme.surface, border: `1px solid ${online ? '#10b98166' : '#334155'}`, borderRadius: 10, overflow: 'hidden' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '11px 14px', borderBottom: `1px solid ${theme.border}` }}>
        <div style={{ width: 34, height: 34, borderRadius: 9, background: '#0f766e22', border: '1px solid #2dd4bf55', display: 'grid', placeItems: 'center' }}>
          <Laptop size={17} color="#5eead4" />
        </div>
        <div style={{ minWidth: 0 }}>
          <p style={{ color: theme.text, fontSize: 13, fontWeight: 800 }}>Chrome local Ä‘Äƒng nháº­p trÆ°á»›c, dashboard quan sÃ¡t sau</p>
          <p style={{ color: theme.textMuted, fontSize: 11 }}>Flow production chÃ­nh: user Ä‘Äƒng nháº­p Facebook trong Chrome local trÃªn device/IP tháº­t, THG tá»± Ä‘á»“ng bá»™ tráº¡ng thÃ¡i rá»“i stream automation vá» Browser.</p>
        </div>
        <span style={{ marginLeft: 'auto', color: online ? '#4ade80' : theme.textMuted, fontSize: 12, display: 'inline-flex', alignItems: 'center', gap: 5 }}>
          <Radio size={12} /> {runtimeOnline}/{connectors.length} runtime ready
        </span>
        <button onClick={() => setSetupOpen(v => !v)} style={{ display: 'inline-flex', alignItems: 'center', gap: 6, padding: '7px 12px', borderRadius: 8, border: '1px solid #2dd4bf66', background: '#0f766e33', color: '#ccfbf1', cursor: 'pointer', fontSize: 12, fontWeight: 700 }}>
          <Laptop size={13} />
          HÆ°á»›ng dáº«n káº¿t ná»‘i
        </button>
      </div>

      {setupOpen && (
        <div style={{ padding: 12, borderBottom: `1px solid ${theme.border}`, background: '#07131f' }}>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(210px, 1fr))', gap: 10 }}>
            <div style={{ border: `1px solid ${theme.border}`, borderRadius: 8, padding: 11, background: theme.surface }}>
              <p style={{ color: '#93c5fd', fontSize: 11, fontWeight: 800, marginBottom: 7 }}>1. CÃ i THG Local Kit</p>
              <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.45, minHeight: 50 }}>
                Má»™t gÃ³i theo há»‡ Ä‘iá»u hÃ nh, bÃªn trong cÃ³ THG Local Runtime Ä‘á»ƒ má»Ÿ Chrome local, giá»¯ session Facebook trÃªn mÃ¡y ngÆ°á»i dÃ¹ng vÃ  stream vá» dashboard.
              </p>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 7 }}>
                {RUNTIME_DOWNLOADS.map(item => (
                  <a
                    key={item.key}
                    href={item.href}
                    style={{ display: 'inline-flex', alignItems: 'center', gap: 6, padding: '7px 10px', borderRadius: 7, border: '1px solid #14b8a666', background: '#0f766e33', color: '#ccfbf1', textDecoration: 'none', fontSize: 12, fontWeight: 700, opacity: systemInfo?.agent_builds?.[item.key] === false ? 0.5 : 1 }}
                  >
                    <Laptop size={13} /> Táº£i Kit {item.label}
                  </a>
                ))}
              </div>
              <p style={{ color: theme.textFaint, fontSize: 10, marginTop: 7, lineHeight: 1.45 }}>
                Sau khi giáº£i nÃ©n, cháº¡y file Start trong kit. Windows dÃ¹ng Start-THG-Local-Runtime.cmd Ä‘á»ƒ cá»­a sá»• luÃ´n má»Ÿ cho báº¡n nháº­p mÃ£ káº¿t ná»‘i vÃ  xem tráº¡ng thÃ¡i.
              </p>
            </div>

            <div style={{ border: `1px solid ${pairingCode ? '#22c55e55' : theme.border}`, borderRadius: 8, padding: 11, background: theme.surface }}>
              <p style={{ color: '#bbf7d0', fontSize: 11, fontWeight: 800, marginBottom: 7 }}>2. GhÃ©p thiáº¿t bá»‹ vá»›i workspace</p>
              <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.45, minHeight: 50 }}>
                MÃ£ nÃ y chá»‰ dÃ nh cho thiáº¿t bá»‹ cá»§a báº¡n. Má»—i nhÃ¢n viÃªn Ä‘Äƒng nháº­p THG vÃ  tá»± táº¡o mÃ£ riÃªng; khÃ´ng dÃ¹ng chung mÃ£ trong workspace.
              </p>
              {dashboardServer && (
                <div style={{ display: 'flex', gap: 7, alignItems: 'center', marginBottom: 9 }}>
                  <code style={{ color: '#bae6fd', fontSize: 11, flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{dashboardServer}</code>
                  <button
                    type="button"
                    onClick={() => navigator.clipboard?.writeText(dashboardServer)}
                    style={{ display: 'inline-flex', alignItems: 'center', gap: 5, padding: '5px 8px', borderRadius: 7, border: '1px solid #38bdf866', background: '#07598533', color: '#e0f2fe', cursor: 'pointer', fontSize: 10, fontWeight: 700 }}
                    title="Copy THG server Ä‘á»ƒ dÃ¡n vÃ o Runtime"
                  >
                    <Copy size={11} /> Copy server
                  </button>
                </div>
              )}
              <p style={{ color: '#fef3c7', fontSize: 10, lineHeight: 1.45, marginBottom: 8 }}>
                THG server trong Runtime pháº£i trÃ¹ng domain dashboard Ä‘ang táº¡o mÃ£. MÃ£ chá»‰ dÃ¹ng má»™t láº§n, gáº¯n vá»›i user táº¡o mÃ£ vÃ  háº¿t háº¡n sau 10 phÃºt.
              </p>
              {pairingCode ? (
                <div style={{ display: 'flex', gap: 7, alignItems: 'center', flexWrap: 'wrap' }}>
                  <code style={{ color: '#dcfce7', fontSize: 18, fontWeight: 900, flex: '1 1 130px', letterSpacing: pairingCodeVisible ? 0 : 2 }}>
                    {pairingCodeVisible ? pairingCode : 'â€¢â€¢â€¢â€¢-â€¢â€¢â€¢â€¢'}
                  </code>
                  <button
                    type="button"
                    onClick={() => setPairingCodeVisible(v => !v)}
                    disabled={pairingExpired}
                    style={{ display: 'inline-flex', alignItems: 'center', gap: 5, padding: '6px 9px', borderRadius: 7, border: `1px solid ${pairingCodeVisible ? '#f59e0b66' : theme.border}`, background: pairingCodeVisible ? '#78350f33' : theme.surfaceAlt, color: pairingExpired ? theme.textFaint : (pairingCodeVisible ? '#fcd34d' : theme.textMuted), cursor: pairingExpired ? 'not-allowed' : 'pointer', opacity: pairingExpired ? 0.6 : 1, fontSize: 11, fontWeight: 700 }}
                  >
                    {pairingCodeVisible ? <EyeOff size={12} /> : <Eye size={12} />}
                    {pairingCodeVisible ? 'áº¨n mÃ£' : 'Hiá»‡n mÃ£'}
                  </button>
                  <button
                    type="button"
                    disabled={!pairingCodeVisible || pairingExpired}
                    onClick={() => {
                      if (pairingCodeVisible && !pairingExpired) navigator.clipboard?.writeText(pairingCode);
                    }}
                    style={{ display: 'inline-flex', alignItems: 'center', gap: 5, padding: '6px 9px', borderRadius: 7, border: `1px solid ${theme.border}`, background: theme.surfaceAlt, color: pairingCodeVisible && !pairingExpired ? theme.textMuted : theme.textFaint, cursor: pairingCodeVisible && !pairingExpired ? 'pointer' : 'not-allowed', opacity: pairingCodeVisible && !pairingExpired ? 1 : 0.55, fontSize: 11 }}
                    title={pairingExpired ? 'MÃ£ Ä‘Ã£ háº¿t háº¡n, hÃ£y táº¡o mÃ£ má»›i' : (pairingCodeVisible ? 'Copy mÃ£ káº¿t ná»‘i' : 'Hiá»‡n mÃ£ trÆ°á»›c khi copy')}
                  >
                    <Copy size={12} /> Copy
                  </button>
                  <button
                    type="button"
                    onClick={onCreate}
                    disabled={creating}
                    style={{ display: 'inline-flex', alignItems: 'center', gap: 5, padding: '6px 9px', borderRadius: 7, border: '1px solid #22c55e66', background: '#16653433', color: '#dcfce7', cursor: creating ? 'wait' : 'pointer', opacity: creating ? 0.65 : 1, fontSize: 11, fontWeight: 700 }}
                  >
                    {creating ? <RefreshCw size={12} className="spin" /> : <KeyRound size={12} />}
                    Táº¡o mÃ£ má»›i
                  </button>
                  <span style={{ display: 'inline-flex', alignItems: 'center', gap: 5, padding: '5px 8px', borderRadius: 999, border: `1px solid ${pairingExpired ? '#ef444466' : '#22c55e66'}`, background: pairingExpired ? '#7f1d1d33' : '#064e3b33', color: pairingExpired ? '#fecaca' : '#bbf7d0', fontSize: 11, fontWeight: 700 }}>
                    {pairingExpired ? 'ÄÃ£ háº¿t háº¡n' : `CÃ²n ${formatCountdown(pairingRemainingMs ?? 0)}`}
                  </span>
                </div>
              ) : (
                <button onClick={onCreate} disabled={creating} style={{ display: 'inline-flex', alignItems: 'center', gap: 6, padding: '7px 10px', borderRadius: 7, border: '1px solid #22c55e66', background: '#16653433', color: '#dcfce7', cursor: creating ? 'wait' : 'pointer', opacity: creating ? 0.65 : 1, fontSize: 12, fontWeight: 700 }}>
                  {creating ? <RefreshCw size={13} className="spin" /> : <KeyRound size={13} />}
                  Táº¡o mÃ£ káº¿t ná»‘i
                </button>
              )}
              {pairingExpiresAt && <p style={{ color: theme.textFaint, fontSize: 10, marginTop: 7 }}>Háº¿t háº¡n: {formatLastSeen(pairingExpiresAt)}</p>}
            </div>

            <div style={{ border: `1px solid ${theme.border}`, borderRadius: 8, padding: 11, background: theme.surface }}>
              <p style={{ color: '#fcd34d', fontSize: 11, fontWeight: 800, marginBottom: 7 }}>3. ÄÄƒng nháº­p trÃªn Chrome local</p>
              <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.45, minHeight: 50 }}>
                Báº¥m Má»Ÿ Chrome local trÃªn account. Runtime sáº½ má»Ÿ cá»­a sá»• Chrome trÃªn mÃ¡y nhÃ¢n viÃªn; hÃ£y Ä‘Äƒng nháº­p Facebook, 2FA hoáº·c checkpoint trá»±c tiáº¿p trong cá»­a sá»• Ä‘Ã³.
              </p>
              <span style={{ color: '#fef3c7', fontSize: 11, display: 'inline-flex', gap: 5, alignItems: 'center' }}><Shield size={12} /> KhÃ´ng nháº­p máº­t kháº©u Facebook vÃ o THG</span>
            </div>

            <div style={{ border: `1px solid ${runtimeOnline ? '#22c55e66' : theme.border}`, borderRadius: 8, padding: 11, background: theme.surface }}>
              <p style={{ color: runtimeOnline ? '#86efac' : theme.textMuted, fontSize: 11, fontWeight: 800, marginBottom: 7 }}>4. Dashboard tá»± nháº­n session</p>
              <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.45, minHeight: 50 }}>
                Khi Chrome Ä‘Ã£ vÃ o Facebook, Runtime tá»± Ä‘Æ°a cá»­a sá»• local ra ná»n, dashboard lÆ°u tráº¡ng thÃ¡i/Facebook ID vÃ  Browser tab trá»Ÿ thÃ nh nÆ¡i quan sÃ¡t automation táº­p trung.
              </p>
              <span style={{ color: runtimeOnline ? '#4ade80' : theme.textFaint, fontSize: 12, display: 'inline-flex', gap: 5, alignItems: 'center' }}>
                {runtimeOnline ? <CheckCircle size={13} /> : <Radio size={13} />} {runtimeOnline ? 'Runtime Ä‘Ã£ sáºµn sÃ ng Ä‘á»“ng bá»™' : online ? 'ÄÃ£ cÃ³ thiáº¿t bá»‹ online, Ä‘ang chá» Chrome local' : 'Äang chá» mÃ¡y káº¿t ná»‘i'}
              </span>
            </div>
          </div>
        </div>
      )}

      {connectors.length === 0 ? (
        <p style={{ color: theme.textMuted, fontSize: 12, padding: '12px 14px' }}>
          ChÆ°a cÃ³ thiáº¿t bá»‹ nÃ o káº¿t ná»‘i. CÃ i THG Local Runtime, táº¡o mÃ£ káº¿t ná»‘i, rá»“i nháº­p mÃ£ trong app Ä‘á»ƒ má»Ÿ Chrome local vÃ  Ä‘á»“ng bá»™ session Facebook.
        </p>
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(260px, 1fr))', gap: 10, padding: 12 }}>
          {connectors.map(c => {
            const identityLabel = facebookIdentityLabel({
              displayName: c.fbDisplayName,
              username: c.fbUsername,
              fbUserId: c.fbUserId,
            });
            return (
            <div key={c.id} style={{ border: `1px solid ${c.online ? '#22c55e55' : theme.border}`, borderRadius: 8, background: theme.surfaceAlt, padding: 11 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 7 }}>
                <span style={{ width: 8, height: 8, borderRadius: '50%', background: c.online ? '#4ade80' : theme.textFaint }} />
                <p style={{ color: theme.text, fontSize: 13, fontWeight: 700, flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{c.name}</p>
                <span style={{ color: c.online ? '#4ade80' : theme.textFaint, fontSize: 10 }}>{c.online ? 'online' : 'offline'}</span>
              </div>
              <p style={{ color: theme.textMuted, fontSize: 11 }}>{c.hostname || 'unknown host'} Â· {c.os || 'unknown os'} Â· {c.version || 'no version'}</p>
              <p style={{ color: theme.textFaint, fontSize: 11, marginTop: 5 }}>Láº§n cuá»‘i {formatLastSeen(c.lastSeen)} Â· {connectorStatusLabel(c.streamStatus)}</p>
              <p style={{ color: c.createdBy === currentUserId ? '#86efac' : theme.textFaint, fontSize: 11, marginTop: 5 }}>
                {c.createdBy === currentUserId ? 'Thiáº¿t bá»‹ cá»§a báº¡n' : `Thiáº¿t bá»‹ thÃ nh viÃªn #${c.createdBy}`}
                {c.assignedAccountId ? ` Â· gáº¯n account #${c.assignedAccountId}` : ''}
              </p>
              {c.currentUrl && <p style={{ color: '#93c5fd', fontSize: 11, marginTop: 5, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{c.currentUrl}</p>}
              {c.streamStatus === 'facebook_logged_in' && identityLabel && <p style={{ color: '#c4b5fd', fontSize: 11, marginTop: 5 }}>{identityLabel}</p>}
              {c.chromeError && <p style={{ color: '#fca5a5', fontSize: 11, marginTop: 5, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{c.chromeError}</p>}
              {(c.createdBy === currentUserId || currentUserRole === 'admin' || currentUserRole === 'founder' || currentUserRole === 'superadmin') && (
                <button
                  type="button"
                  onClick={() => onDisconnect(c)}
                  disabled={disconnectingId === c.id}
                  style={{ display: 'inline-flex', alignItems: 'center', gap: 6, marginTop: 10, padding: '6px 9px', borderRadius: 7, border: '1px solid #ef444455', background: '#7f1d1d33', color: '#fecaca', cursor: disconnectingId === c.id ? 'wait' : 'pointer', opacity: disconnectingId === c.id ? 0.65 : 1, fontSize: 11, fontWeight: 700 }}
                >
                  {disconnectingId === c.id ? <RefreshCw size={12} className="spin" /> : <Unplug size={12} />}
                  {disconnectingId === c.id ? 'Äang ngáº¯t' : 'Disconnect mÃ¡y nÃ y'}
                </button>
              )}
            </div>
          );})}
        </div>
      )}
    </div>
  );
}

