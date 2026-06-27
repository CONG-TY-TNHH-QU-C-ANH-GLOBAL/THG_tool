'use client';

import { useEffect, useState } from 'react';
import { FileText, Play, Download, Film } from 'lucide-react';
import type { ReelStrings } from '../../i18n/reelStrings';
import { approveReel, fetchReelVideo, getReel, parseShots, type Reel, type ReelDetail } from '../../services/reelsService';
import ReelProgress from './ReelProgress';
import ReelScriptPanel from './ReelScriptPanel';
import PublishReelForm from './PublishReelForm';

interface ReelCardProps {
  reel: Reel;
  tick: number;
  isAdmin: boolean;
  tr: ReelStrings;
  onChanged: () => void;
}

const ACTIVE = new Set(['scripting', 'rendering', 'posting']);
// Statuses where a final.mp4 exists and can be streamed (final_output_key is set).
const HAS_VIDEO = new Set(['assembled', 'posting', 'published']);

function statusMeta(status: string, tr: ReelStrings): { label: string; cls: string } {
  switch (status) {
    case 'script_ready': return { label: tr.sReady, cls: 'tag tag-cold' };
    case 'rendering': return { label: tr.sRendering, cls: 'tag tag-warm' };
    case 'render_stuck': return { label: tr.sStuck, cls: 'tag tag-hot' };
    case 'assembled': return { label: tr.sAssembled, cls: 'tag tag-warm' };
    case 'posting': return { label: tr.sPosting, cls: 'tag tag-warm' };
    case 'published': return { label: tr.sPublished, cls: 'tag tag-ok' };
    case 'failed': return { label: tr.sFailed, cls: 'tag tag-hot' };
    default: return { label: tr.sDraft, cls: 'tag tag-cold' };
  }
}

// ReelCard is the per-reel lifecycle surface: it self-loads detail (script + shots) and
// polls while active, then renders the right action for the current state. Reuses the
// PostingView card shell; the spend gate (approve) is window.confirm-guarded.
export default function ReelCard({ reel, tick, isAdmin, tr, onChanged }: Readonly<ReelCardProps>) {
  const [detail, setDetail] = useState<ReelDetail | null>(null);
  const [scriptOpen, setScriptOpen] = useState(false);
  const [publishOpen, setPublishOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ text: string; ok: boolean } | null>(null);
  const [videoUrl, setVideoUrl] = useState<string | null>(null);
  const [videoBusy, setVideoBusy] = useState(false);

  const status = detail?.status ?? reel.status;

  useEffect(() => {
    if (detail && !ACTIVE.has(status)) return; // idle + loaded → no re-fetch
    let alive = true;
    getReel(reel.id).then((d) => { if (alive) setDetail(d); }).catch(() => {});
    return () => { alive = false; };
  }, [reel.id, tick]); // eslint-disable-line react-hooks/exhaustive-deps

  const refresh = () => {
    getReel(reel.id).then(setDetail).catch(() => {});
    onChanged();
  };

  const approve = async () => {
    if (busy) return;
    if (typeof window !== 'undefined' && !window.confirm(tr.confirmApprove(reel.id))) return;
    setBusy(true);
    setMsg(null);
    try {
      await approveReel(reel.id);
      refresh();
    } catch (err) {
      setMsg({ text: err instanceof Error ? err.message : 'error', ok: false });
    } finally {
      setBusy(false);
    }
  };

  // Download the current reel result (reel + script + shots + keys + cost) as a JSON file
  // for offline review. In fake-render mode there is no real video file — this JSON is the
  // inspectable artifact; real video download arrives with the real-provider track.
  const downloadResult = () => {
    if (typeof window === 'undefined' || !detail) return;
    const blob = new Blob([JSON.stringify(detail, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `reel-${reel.id}-result.json`;
    a.click();
    URL.revokeObjectURL(url);
  };

  // Toggle the inline player: fetch the .mp4 as an authenticated blob (the endpoint needs the
  // JWT header so a plain <video src> would 401), turn it into an object URL, and play it.
  // Re-clicking hides the player and frees the blob URL.
  const toggleVideo = async () => {
    if (videoUrl) {
      URL.revokeObjectURL(videoUrl);
      setVideoUrl(null);
      return;
    }
    if (videoBusy) return;
    setVideoBusy(true);
    setMsg(null);
    try {
      const blob = await fetchReelVideo(reel.id);
      setVideoUrl(URL.createObjectURL(blob));
    } catch (err) {
      setMsg({ text: err instanceof Error ? err.message : 'error', ok: false });
    } finally {
      setVideoBusy(false);
    }
  };

  // Free the blob URL when the card unmounts so we don't leak object URLs.
  useEffect(() => () => { if (videoUrl) URL.revokeObjectURL(videoUrl); }, [videoUrl]);

  const script = detail?.script ?? null;
  const shotCount = script ? parseShots(script.shot_list).length : 0;
  const cost = (detail?.total_cost_usd ?? reel.total_cost_usd ?? 0).toFixed(2);
  const meta = statusMeta(status, tr);
  const caption = script?.caption || reel.brief_style;

  return (
    <div className="card" style={{ display: 'flex', flexDirection: 'column' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12, flexWrap: 'wrap' }}>
        <span className="mono" style={{ background: 'var(--bg-elev)', color: 'var(--text)', padding: '2px 6px', borderRadius: 4, fontSize: 11 }}>reel #{reel.id}</span>
        <div style={{ flex: 1 }} />
        <span className={meta.cls}>{meta.label}</span>
      </div>

      <p style={{ color: 'var(--text)', fontSize: 13.5, lineHeight: 1.6, whiteSpace: 'pre-wrap' }}>{caption || '(Trống)'}</p>

      {script && (
        <div className="mono" style={{ fontSize: 11, color: 'var(--text-faint)', margin: '10px 0' }}>
          {tr.scriptSummary(script.version, shotCount, reel.target_duration_sec, cost)}
        </div>
      )}

      {scriptOpen && script && (
        <ReelScriptPanel reelId={reel.id} script={script} editable={status === 'script_ready'} tr={tr} onSaved={() => { setScriptOpen(false); refresh(); }} />
      )}
      {status === 'rendering' && detail && <ReelProgress done={detail.shots_done} total={detail.shots_total} cost={detail.total_cost_usd} tr={tr} />}
      {status === 'render_stuck' && <div className="banner banner-hot" style={{ fontSize: 12 }}>{tr.stuckBanner}</div>}
      {status === 'failed' && <div className="banner banner-hot" style={{ fontSize: 12 }}>{tr.sFailed}</div>}
      {status === 'assembled' && !publishOpen && <div style={{ fontSize: 12, color: 'var(--text-mute)' }}>{tr.assembledLabel}</div>}
      {status === 'assembled' && publishOpen && (
        <PublishReelForm reelId={reel.id} tr={tr} onResult={(text, ok) => { setMsg({ text, ok }); if (ok) { setPublishOpen(false); refresh(); } }} />
      )}
      {msg && <div className={`banner ${msg.ok ? 'banner-ok' : 'banner-warm'}`} style={{ fontSize: 12, marginTop: 8 }}>{msg.text}</div>}

      {videoUrl && (
        <video
          src={videoUrl}
          controls
          autoPlay
          playsInline
          style={{ width: '100%', maxHeight: 480, marginTop: 12, borderRadius: 8, background: '#000' }}
        />
      )}

      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 16, paddingTop: 16, borderTop: '1px solid var(--line)' }}>
        {script && (
          <button type="button" className="btn btn-ghost btn-sm" onClick={() => setScriptOpen((v) => !v)} style={{ color: 'var(--text-mute)' }}>
            <FileText size={12} /> {tr.viewScript}
          </button>
        )}
        {detail && (
          <button type="button" className="btn btn-ghost btn-sm" onClick={downloadResult} style={{ color: 'var(--text-mute)' }} title={tr.download}>
            <Download size={12} />
          </button>
        )}
        {HAS_VIDEO.has(status) && (
          <button type="button" className="btn btn-ghost btn-sm" disabled={videoBusy} onClick={() => void toggleVideo()} style={{ color: 'var(--ok)' }}>
            <Film size={12} /> {videoBusy ? tr.videoLoading : videoUrl ? tr.hideVideo : tr.watchVideo}
          </button>
        )}
        <div style={{ flex: 1 }} />
        {isAdmin && status === 'script_ready' && (
          <button type="button" className="btn btn-primary btn-sm" disabled={busy} onClick={() => void approve()}>
            <Play size={12} /> {tr.approve}
          </button>
        )}
        {isAdmin && status === 'assembled' && (
          <button type="button" className="btn btn-primary btn-sm" onClick={() => setPublishOpen((v) => !v)}>{tr.publish}</button>
        )}
        {(status === 'posting' || status === 'published') && (
          <span className="tag tag-ok">{tr.published}</span>
        )}
      </div>
    </div>
  );
}
