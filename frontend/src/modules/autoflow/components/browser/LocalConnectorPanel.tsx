import { useEffect, useState } from 'react';
import {
  AlertCircle,
  CheckCircle2,
  Copy,
  ExternalLink,
  Eye,
  EyeOff,
  KeyRound,
  Puzzle,
  Radio,
  RefreshCw,
  Shield,
  Unplug,
} from 'lucide-react';
import type { SystemInfo } from '../../services/systemService';
import type { LocalConnector } from '../../types';
import { useLang } from '../../i18n/useLang';
import { connectorStatusLabel, facebookIdentityLabel, formatCountdown, formatLastSeen, isDashboardStreamConnector } from './browserHelpers';

function resolveExtensionStoreUrl(systemInfo: SystemInfo | null): string {
  const directUrl = (systemInfo?.chrome_extension_store_url || '').trim();
  if (directUrl) return directUrl;
  const extensionId = (systemInfo?.chrome_extension_id || '').trim();
  if (!extensionId) return '';
  return `https://chromewebstore.google.com/detail/thg-chrome-extension/${extensionId}`;
}

function resolveExtensionBetaUrl(systemInfo: SystemInfo | null): string {
  const betaUrl = (systemInfo?.chrome_extension_beta_url || '').trim();
  if (betaUrl) return betaUrl;
  return (systemInfo?.chrome_extension_beta_package_url || '').trim();
}

interface LocalConnectorPanelProps {
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
}

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
}: LocalConnectorPanelProps) {
  const { t } = useLang();
  const tc = t.connector;
  const online = connectors.filter((c) => c.online).length;
  const extensionOnline = connectors.filter((c) => c.online && isDashboardStreamConnector(c)).length;
  const [setupOpen, setSetupOpen] = useState(connectors.length === 0);
  const [pairingCodeVisible, setPairingCodeVisible] = useState(false);
  const [pairingRemainingMs, setPairingRemainingMs] = useState<number | null>(null);
  const [dashboardServer, setDashboardServer] = useState('');
  const pairingExpired = pairingCode !== '' && pairingRemainingMs !== null && pairingRemainingMs <= 0;
  const extensionStoreUrl = resolveExtensionStoreUrl(systemInfo);
  const extensionBetaUrl = resolveExtensionBetaUrl(systemInfo);
  const extensionBetaReady = extensionBetaUrl.length > 0;
  const extensionInstallReady = extensionStoreUrl.length > 0;

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

  const streamStatusTag = extensionOnline > 0 ? 'tag-ok' : online > 0 ? 'tag-warm' : 'tag-mute';

  return (
    <div className="card" style={{ padding: 0, overflow: 'hidden' }}>
      <header
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 'var(--s-3)',
          padding: 'var(--s-4) var(--s-5)',
          borderBottom: '1px solid var(--line)',
          flexWrap: 'wrap',
        }}
      >
        <div
          style={{
            width: 36,
            height: 36,
            borderRadius: 'var(--radius-md)',
            background: 'var(--accent-soft)',
            display: 'grid',
            placeItems: 'center',
            color: 'var(--accent)',
            flexShrink: 0,
          }}
        >
          <Puzzle size={17} />
        </div>
        <div style={{ minWidth: 240, flex: '1 1 320px' }}>
          <h3 style={{ margin: 0, fontSize: 15, fontWeight: 600, color: 'var(--text)' }}>{tc.panelTitle}</h3>
          <p style={{ margin: '4px 0 0', fontSize: 12.5, color: 'var(--text-mute)', lineHeight: 1.5 }}>{tc.panelSub}</p>
        </div>
        <span className={`tag ${streamStatusTag}`} style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
          <Radio size={12} />
          {tc.statusReady(extensionOnline, connectors.length)}
        </span>
        <button type="button" className="btn btn-ghost btn-sm" onClick={() => setSetupOpen((v) => !v)}>
          <Puzzle size={13} />
          {tc.setupToggle}
        </button>
      </header>

      {setupOpen && (
        <div
          style={{
            padding: 'var(--s-5)',
            borderBottom: '1px solid var(--line)',
            background: 'var(--bg-elev)',
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))',
            gap: 'var(--s-4)',
          }}
        >
          <article className="card" style={{ padding: 'var(--s-5)', display: 'flex', flexDirection: 'column', gap: 'var(--s-3)' }}>
            <div className="eyebrow"><span className="dot" />01</div>
            <h4 style={{ margin: 0, fontSize: 14, fontWeight: 600, color: 'var(--text)' }}>{tc.stepInstallTitle}</h4>
            <p style={{ margin: 0, fontSize: 12.5, color: 'var(--text-mute)', lineHeight: 1.55 }}>{tc.stepInstallBody}</p>
            {extensionInstallReady ? (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--s-2)', alignItems: 'flex-start' }}>
                <a className="btn btn-primary btn-sm" href={extensionStoreUrl} target="_blank" rel="noreferrer">
                  <ExternalLink size={13} />
                  {tc.stepInstallStore}
                </a>
                {extensionBetaReady && (
                  <a className="btn btn-ghost btn-sm" href={extensionBetaUrl} target="_blank" rel="noreferrer">
                    <ExternalLink size={13} />
                    {tc.stepInstallBeta}
                  </a>
                )}
              </div>
            ) : (
              <button type="button" className="btn btn-ghost btn-sm" disabled style={{ alignSelf: 'flex-start' }}>
                <AlertCircle size={13} />
                {tc.stepInstallNoConfig}
              </button>
            )}
            <p style={{ margin: 0, fontSize: 11.5, color: 'var(--text-faint)', lineHeight: 1.5 }}>
              {extensionInstallReady ? tc.stepInstallStoreHint : tc.stepInstallNoConfigHint}
            </p>
            {extensionBetaReady && (
              <p style={{ margin: 0, fontSize: 11.5, color: 'var(--warn)', lineHeight: 1.5 }}>{tc.stepInstallBetaHint}</p>
            )}
          </article>

          <article
            className="card"
            style={{
              padding: 'var(--s-5)',
              display: 'flex',
              flexDirection: 'column',
              gap: 'var(--s-3)',
              borderColor: pairingCode && !pairingExpired ? 'var(--accent-glow)' : 'var(--line-strong)',
            }}
          >
            <div className="eyebrow"><span className="dot" />02</div>
            <h4 style={{ margin: 0, fontSize: 14, fontWeight: 600, color: 'var(--text)' }}>{tc.stepPairTitle}</h4>
            <p style={{ margin: 0, fontSize: 12.5, color: 'var(--text-mute)', lineHeight: 1.55 }}>{tc.stepPairBody}</p>

            {dashboardServer && (
              <div style={{ display: 'grid', gap: 4 }}>
                <span className="field-label">{tc.stepPairServerHint}</span>
                <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
                  <code
                    className="mono"
                    style={{
                      flex: 1,
                      fontSize: 12,
                      color: 'var(--text-mute)',
                      padding: '6px 10px',
                      background: 'var(--bg-elev-2)',
                      borderRadius: 'var(--radius-sm)',
                      border: '1px solid var(--line)',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                    }}
                  >
                    {dashboardServer}
                  </code>
                  <button
                    type="button"
                    className="btn btn-ghost btn-sm btn-icon"
                    onClick={() => navigator.clipboard?.writeText(dashboardServer)}
                    title={tc.stepPairCopyServer}
                  >
                    <Copy size={12} />
                  </button>
                </div>
              </div>
            )}

            {pairingCode ? (
              <>
                <div
                  className="mono tabular"
                  style={{
                    fontSize: 28,
                    fontWeight: 600,
                    letterSpacing: pairingCodeVisible ? '0.04em' : '0.4em',
                    color: pairingExpired ? 'var(--text-faint)' : 'var(--accent)',
                    background: 'var(--accent-soft)',
                    border: `1px solid ${pairingExpired ? 'var(--line-strong)' : 'var(--accent-glow)'}`,
                    borderRadius: 'var(--radius-md)',
                    padding: 'var(--s-4) var(--s-5)',
                    textAlign: 'center',
                    opacity: pairingExpired ? 0.55 : 1,
                  }}
                >
                  {pairingCodeVisible ? pairingCode : tc.stepPairHidden}
                </div>

                <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                  <button
                    type="button"
                    className="btn btn-ghost btn-sm"
                    onClick={() => setPairingCodeVisible((v) => !v)}
                    disabled={pairingExpired}
                  >
                    {pairingCodeVisible ? <EyeOff size={13} /> : <Eye size={13} />}
                    {pairingCodeVisible ? tc.stepPairHide : tc.stepPairShow}
                  </button>
                  <button
                    type="button"
                    className="btn btn-ghost btn-sm"
                    disabled={!pairingCodeVisible || pairingExpired}
                    onClick={() => {
                      if (pairingCodeVisible && !pairingExpired) navigator.clipboard?.writeText(pairingCode);
                    }}
                  >
                    <Copy size={13} />
                    {tc.stepPairCopy}
                  </button>
                  <button type="button" className="btn btn-primary btn-sm" onClick={onCreate} disabled={creating}>
                    {creating ? <RefreshCw size={13} className="spin" /> : <KeyRound size={13} />}
                    {tc.stepPairRefresh}
                  </button>
                </div>

                <span
                  className={`tag ${pairingExpired ? 'tag-hot' : 'tag-ok'}`}
                  style={{ alignSelf: 'flex-start' }}
                >
                  {pairingExpired ? tc.stepPairExpired : tc.stepPairRemaining(formatCountdown(pairingRemainingMs ?? 0))}
                </span>
                {pairingExpiresAt && (
                  <p style={{ margin: 0, fontSize: 11.5, color: 'var(--text-faint)' }}>
                    {tc.stepPairExpiresAt(formatLastSeen(pairingExpiresAt))}
                  </p>
                )}
              </>
            ) : (
              <button type="button" className="btn btn-primary btn-sm" onClick={onCreate} disabled={creating} style={{ alignSelf: 'flex-start' }}>
                {creating ? <RefreshCw size={13} className="spin" /> : <KeyRound size={13} />}
                {tc.stepPairCreate}
              </button>
            )}
          </article>

          <article className="card" style={{ padding: 'var(--s-5)', display: 'flex', flexDirection: 'column', gap: 'var(--s-3)' }}>
            <div className="eyebrow"><span className="dot" />03</div>
            <h4 style={{ margin: 0, fontSize: 14, fontWeight: 600, color: 'var(--text)' }}>{tc.stepFacebookTitle}</h4>
            <p style={{ margin: 0, fontSize: 12.5, color: 'var(--text-mute)', lineHeight: 1.55 }}>{tc.stepFacebookBody}</p>
            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontSize: 12, color: 'var(--text-mute)' }}>
              <Shield size={13} />
              {tc.stepFacebookSecurity}
            </span>
            <span
              className={`tag ${extensionOnline ? 'tag-ok' : online ? 'tag-warm' : 'tag-mute'}`}
              style={{ alignSelf: 'flex-start', marginTop: 'auto' }}
            >
              {extensionOnline ? <CheckCircle2 size={12} /> : <Radio size={12} />}
              {extensionOnline ? tc.statusStreamReady : online ? tc.statusWaitingFacebook : tc.statusWaitingChrome}
            </span>
          </article>
        </div>
      )}

      {connectors.length === 0 ? (
        <div className="empty" style={{ margin: 'var(--s-5)' }}>
          <p style={{ margin: 0 }}>{tc.statusEmpty}</p>
        </div>
      ) : (
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))',
            gap: 'var(--s-3)',
            padding: 'var(--s-5)',
          }}
        >
          {connectors.map((c) => {
            const identityLabel = facebookIdentityLabel({
              displayName: c.fbDisplayName,
              username: c.fbUsername,
              fbUserId: c.fbUserId,
            });
            const canDisconnect =
              c.createdBy === currentUserId ||
              currentUserRole === 'admin' ||
              currentUserRole === 'founder' ||
              currentUserRole === 'superadmin';
            const isMine = c.createdBy === currentUserId;
            return (
              <article
                key={c.id}
                className="card"
                style={{
                  padding: 'var(--s-4)',
                  display: 'flex',
                  flexDirection: 'column',
                  gap: 'var(--s-2)',
                  borderColor: c.online ? 'var(--accent-glow)' : 'var(--line)',
                }}
              >
                <header style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <span
                    style={{
                      width: 8,
                      height: 8,
                      borderRadius: '50%',
                      background: c.online ? 'var(--ok)' : 'var(--text-faint)',
                      flexShrink: 0,
                    }}
                  />
                  <strong
                    style={{
                      flex: 1,
                      fontSize: 13.5,
                      fontWeight: 600,
                      color: 'var(--text)',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                    }}
                  >
                    {c.name}
                  </strong>
                  <span className={`tag ${c.online ? 'tag-ok' : 'tag-mute'}`}>
                    {c.online ? tc.statusOnline : tc.statusOffline}
                  </span>
                </header>

                <p className="mono" style={{ margin: 0, fontSize: 11.5, color: 'var(--text-mute)' }}>
                  {[c.hostname || 'Chrome Extension', c.os || 'Chrome', c.version || '—'].join(' · ')}
                </p>
                <p style={{ margin: 0, fontSize: 11.5, color: 'var(--text-faint)' }}>
                  {tc.deviceLastSeen(formatLastSeen(c.lastSeen))} · {connectorStatusLabel(c.streamStatus)}
                </p>
                <p style={{ margin: 0, fontSize: 11.5, color: isMine ? 'var(--accent)' : 'var(--text-faint)' }}>
                  {isMine ? tc.deviceMine : tc.deviceOther(c.createdBy)}
                  {c.assignedAccountId ? ` · ${tc.deviceAccountBound(c.assignedAccountId)}` : ''}
                </p>
                {c.currentUrl && (
                  <p
                    className="mono"
                    style={{
                      margin: 0,
                      fontSize: 11.5,
                      color: 'var(--info)',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                    }}
                  >
                    {c.currentUrl}
                  </p>
                )}
                {c.streamStatus === 'facebook_logged_in' && identityLabel && (
                  <p style={{ margin: 0, fontSize: 11.5, color: 'var(--accent)' }}>{identityLabel}</p>
                )}
                {c.chromeError && (
                  <p
                    style={{
                      margin: 0,
                      fontSize: 11.5,
                      color: 'var(--hot)',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                    }}
                  >
                    {c.chromeError}
                  </p>
                )}
                {canDisconnect && (
                  <button
                    type="button"
                    className="btn btn-ghost btn-sm"
                    onClick={() => onDisconnect(c)}
                    disabled={disconnectingId === c.id}
                    style={{ alignSelf: 'flex-start', marginTop: 'var(--s-2)' }}
                  >
                    {disconnectingId === c.id ? <RefreshCw size={12} className="spin" /> : <Unplug size={12} />}
                    {disconnectingId === c.id ? tc.deviceDisconnecting : tc.deviceDisconnect}
                  </button>
                )}
              </article>
            );
          })}
        </div>
      )}
    </div>
  );
}
