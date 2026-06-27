'use client';

import { useEffect, useState } from 'react';
import { RefreshCw, Plus } from 'lucide-react';
import { useLang } from '../../i18n/useLang';
import { REEL_STRINGS } from '../../i18n/reelStrings';
import { listReels, type Reel } from '../../services/reelsService';
import CreateReelForm from '../reels/CreateReelForm';
import ReelCard from '../reels/ReelCard';

interface ReelViewProps {
  orgId: string;
  isAdmin: boolean;
}

type ReelFilter = 'all' | 'draft' | 'rendering' | 'ready' | 'published' | 'failed';
const FILTER_MAP: Record<Exclude<ReelFilter, 'all'>, string[]> = {
  draft: ['draft', 'scripting', 'script_ready'],
  rendering: ['rendering', 'render_stuck'],
  ready: ['assembled'],
  published: ['posting', 'published'],
  failed: ['failed'],
};
const POLL_STATES = new Set(['scripting', 'rendering', 'posting']);

// ReelView is the top-level Reel tab. Header/filters/create-card mirror MissionsView +
// PostingView so the surface matches the rest of the dashboard. It polls (~2.5s) only
// while at least one reel is in an active state.
export default function ReelView({ orgId, isAdmin }: Readonly<ReelViewProps>) {
  const { lang } = useLang();
  const tr = REEL_STRINGS[lang];
  void orgId;

  const [reels, setReels] = useState<Reel[]>([]);
  const [filter, setFilter] = useState<ReelFilter>('all');
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [tick, setTick] = useState(0);
  const [msg, setMsg] = useState('');

  const load = async () => {
    setLoading(true);
    try {
      setReels(await listReels(100));
    } catch (err) {
      setMsg(err instanceof Error ? err.message : 'error');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void load(); }, []);

  // Poll while any reel is active; advance `tick` so each active ReelCard re-fetches.
  useEffect(() => {
    const active = reels.some((r) => POLL_STATES.has(r.status));
    if (!active) return;
    const id = window.setInterval(() => { setTick((t) => t + 1); void load(); }, 2500);
    return () => window.clearInterval(id);
  }, [reels]); // eslint-disable-line react-hooks/exhaustive-deps

  const FILTERS: Array<{ value: ReelFilter; label: string }> = [
    { value: 'all', label: tr.fAll }, { value: 'draft', label: tr.fDraft },
    { value: 'rendering', label: tr.fRendering }, { value: 'ready', label: tr.fReady },
    { value: 'published', label: tr.fPublished }, { value: 'failed', label: tr.fFailed },
  ];
  const filtered = filter === 'all' ? reels : reels.filter((r) => FILTER_MAP[filter].includes(r.status));

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <header style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
        <div>
          <div className="eyebrow"><span className="dot" />{tr.eyebrow}</div>
          <h2 style={{ fontSize: 28, marginTop: 8 }}>{tr.title}</h2>
          <p style={{ color: 'var(--text-mute)', fontSize: 13.5, marginTop: 6, maxWidth: 640 }}>{tr.subtitle}</p>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <button type="button" className="btn btn-ghost btn-sm" onClick={() => void load()}>
            <RefreshCw size={12} /> {tr.refresh}
          </button>
          {isAdmin && !creating && (
            <button type="button" className="btn btn-primary btn-sm" onClick={() => setCreating(true)}>
              <Plus size={14} /> {tr.createCta}
            </button>
          )}
        </div>
      </header>

      {creating && isAdmin && (
        <div className="card" style={{ padding: 18 }}>
          <div className="eyebrow" style={{ marginBottom: 12 }}><span className="dot" />{tr.createCardTitle}</div>
          <CreateReelForm onCreated={() => { setCreating(false); void load(); }} onCancel={() => setCreating(false)} />
        </div>
      )}

      {msg && <div className="banner banner-hot">{msg}</div>}

      <header style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
        {FILTERS.map((f) => (
          <button key={f.value} onClick={() => setFilter(f.value)} className={`filter-pill ${filter === f.value ? 'is-active' : ''}`}>
            {f.label}
          </button>
        ))}
      </header>

      {loading && reels.length === 0 && (
        <div style={{ display: 'flex', justifyContent: 'center', padding: 40 }}>
          <RefreshCw size={24} className="spin" style={{ color: 'var(--text-mute)' }} />
        </div>
      )}

      {!loading && filtered.length === 0 && !creating && (
        <div className="empty" style={{ margin: 40 }}>
          <div className="eyebrow"><span className="dot" />{tr.eyebrow}</div>
          <h3>{tr.emptyTitle}</h3>
          <p>{tr.emptyDesc}</p>
        </div>
      )}

      {filtered.length > 0 && (
        <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'repeat(auto-fill, minmax(340px, 1fr))' }}>
          {filtered.map((r) => (
            <ReelCard key={r.id} reel={r} tick={tick} isAdmin={isAdmin} tr={tr} onChanged={() => void load()} />
          ))}
        </div>
      )}
    </div>
  );
}
