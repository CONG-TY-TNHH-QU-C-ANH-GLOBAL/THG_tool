'use client';

import { useCallback, useEffect, useState } from 'react';
import { Plus, RefreshCw } from 'lucide-react';
import {
  archiveMission,
  deleteMission,
  getCrawlIntents,
  pauseMission,
  resumeMission,
  updateMissionInterval,
  type CrawlIntent,
} from '../../services/crawlIntentService';
import MissionCard from '../missions/MissionCard';
import CreateMissionForm from '../missions/CreateMissionForm';
import { useLang } from '../../i18n/useLang';

interface MissionsViewProps {
  orgId: string;
  isAdmin: boolean;
}

type Toast = { id: number; text: string; tone: 'ok' | 'warn' | 'error' };

export default function MissionsView({ orgId, isAdmin }: Readonly<MissionsViewProps>) {
  void orgId;
  const { t } = useLang();
  const tm = t.missionsView;

  const [intents, setIntents] = useState<CrawlIntent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [busyId, setBusyId] = useState<number | null>(null);
  const [toasts, setToasts] = useState<Toast[]>([]);

  const load = useCallback(async () => {
    try {
      const list = await getCrawlIntents();
      setIntents(list);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : tm.loadError);
    } finally {
      setLoading(false);
    }
  }, [tm.loadError]);

  useEffect(() => {
    void load();
    const id = window.setInterval(() => void load(), 10000);
    return () => window.clearInterval(id);
  }, [load]);

  const pushToast = (text: string, tone: Toast['tone'] = 'ok') => {
    const id = Date.now() + Math.random();
    setToasts((prev) => [...prev, { id, text, tone }]);
    window.setTimeout(() => setToasts((prev) => prev.filter((x) => x.id !== id)), 6000);
  };

  const handleCreated = (intent: CrawlIntent, created: boolean) => {
    setCreating(false);
    void load();
    if (created) {
      pushToast(tm.toastCreated, 'ok');
    } else {
      pushToast(tm.toastResurrected(intent.name || intent.source_label || intent.source_type), 'warn');
    }
  };

  const wrap = async (id: number, action: () => Promise<void>) => {
    setBusyId(id);
    try {
      await action();
      await load();
    } catch (err) {
      pushToast(err instanceof Error ? err.message : t.common.error, 'error');
    } finally {
      setBusyId(null);
    }
  };

  const handlePause = (intent: CrawlIntent) => void wrap(intent.id, () => pauseMission(intent.id));
  const handleResume = (intent: CrawlIntent) => void wrap(intent.id, () => resumeMission(intent.id));
  const handleArchive = (intent: CrawlIntent) => {
    if (!window.confirm(tm.confirmArchive(intent.name || intent.source_label || intent.source_type))) return;
    void wrap(intent.id, () => archiveMission(intent.id));
  };
  const handleEditInterval = (intent: CrawlIntent) => {
    const raw = window.prompt(tm.promptInterval(intent.interval_minutes), String(intent.interval_minutes));
    if (raw === null) return;
    const minutes = Number.parseInt(raw.trim(), 10);
    if (!Number.isFinite(minutes) || minutes <= 0) {
      pushToast(tm.invalidInterval, 'error');
      return;
    }
    void wrap(intent.id, async () => {
      await updateMissionInterval(intent.id, minutes);
      pushToast(tm.toastIntervalUpdated(minutes), 'ok');
    });
  };
  const handleDelete = (intent: CrawlIntent) => {
    const label = intent.name || intent.source_label || intent.source_type;
    if (!window.confirm(tm.confirmDelete(label))) return;
    void wrap(intent.id, async () => {
      await deleteMission(intent.id);
      pushToast(tm.toastDeleted(label), 'ok');
    });
  };

  const hasIntents = intents.length > 0;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16, position: 'relative' }}>
      <header style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
        <div>
          <div className="eyebrow"><span className="dot" />{tm.eyebrow}</div>
          <h2 style={{ fontSize: 28, marginTop: 8 }}>{tm.title}</h2>
          <p style={{ color: 'var(--text-mute)', fontSize: 13.5, marginTop: 6, maxWidth: 640 }}>{tm.subtitle}</p>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <button type="button" className="btn btn-ghost btn-sm" onClick={() => void load()}>
            <RefreshCw size={12} /> {t.common.refresh}
          </button>
          {isAdmin && hasIntents && !creating && (
            <button type="button" className="btn btn-primary btn-sm" onClick={() => setCreating(true)}>
              <Plus size={14} /> {tm.createCtaShort}
            </button>
          )}
        </div>
      </header>

      {creating && isAdmin && (
        <div className="card" style={{ padding: 18 }}>
          <div className="eyebrow" style={{ marginBottom: 12 }}><span className="dot" />{tm.createCta}</div>
          <CreateMissionForm onCreated={handleCreated} onCancel={() => setCreating(false)} />
        </div>
      )}

      {!hasIntents && !loading && isAdmin && !creating && (
        <div className="card" style={{ padding: 24 }}>
          <div className="empty" style={{ marginBottom: 16 }}>
            <div className="eyebrow"><span className="dot" />{tm.eyebrow}</div>
            <h3>{tm.emptyTitle}</h3>
            <p>{tm.emptyDesc}</p>
          </div>
          <CreateMissionForm onCreated={handleCreated} />
        </div>
      )}

      {!hasIntents && !loading && !isAdmin && (
        <div className="empty">
          <h3>{tm.emptyTitle}</h3>
          <p>{tm.emptyDesc}</p>
        </div>
      )}

      {loading && !hasIntents && (
        <div className="skeleton" style={{ height: 120, borderRadius: 12 }} />
      )}

      {error && (
        <div style={{ fontSize: 12, color: 'var(--hot)', background: 'rgba(220,40,40,0.08)', padding: '8px 12px', borderRadius: 8 }}>
          {error}
        </div>
      )}

      {hasIntents && (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(300px, 1fr))', gap: 12 }}>
          {intents.map((intent) => (
            <MissionCard
              key={intent.id}
              intent={intent}
              variant="full"
              busy={busyId === intent.id}
              onPause={isAdmin ? handlePause : undefined}
              onResume={isAdmin ? handleResume : undefined}
              onArchive={isAdmin ? handleArchive : undefined}
              onEditInterval={isAdmin ? handleEditInterval : undefined}
              onDelete={isAdmin ? handleDelete : undefined}
            />
          ))}
        </div>
      )}

      {toasts.length > 0 && (
        <div style={{ position: 'fixed', bottom: 16, right: 16, display: 'flex', flexDirection: 'column', gap: 8, zIndex: 1000 }}>
          {toasts.map((toast) => (
            <div
              key={toast.id}
              className="card"
              style={{
                padding: '10px 14px',
                maxWidth: 360,
                fontSize: 13,
                borderLeft: `3px solid ${toast.tone === 'error' ? 'var(--hot)' : toast.tone === 'warn' ? 'var(--info)' : 'var(--ok)'}`,
                background: 'var(--surface)',
              }}
            >
              {toast.text}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
