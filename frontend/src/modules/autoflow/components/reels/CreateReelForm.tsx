'use client';

import { useState } from 'react';
import { Send, ChevronDown, ChevronRight, AlertCircle } from 'lucide-react';
import { createReel, type ReelResult } from '../../services/reelsService';
import { useLang } from '../../i18n/useLang';
import { REEL_STRINGS } from '../../i18n/reelStrings';

interface CreateReelFormProps {
  onCreated?: (result: ReelResult) => void;
  onCancel?: () => void;
}

const MIN_BRIEF = 20;
const DEFAULT_DURATION = 25;

// CreateReelForm mirrors CreateMissionForm: required brief (≥20 chars) + optional keywords,
// duration behind an Advanced toggle. It collects ONLY what POST /reels accepts; account +
// target are gathered later at the publish step.
export default function CreateReelForm({ onCreated, onCancel }: Readonly<CreateReelFormProps>) {
  const { lang } = useLang();
  const tr = REEL_STRINGS[lang];

  const [brief, setBrief] = useState('');
  const [keywords, setKeywords] = useState('');
  const [duration, setDuration] = useState<number>(DEFAULT_DURATION);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const briefTrim = brief.trim();
  const canSubmit = briefTrim.length >= MIN_BRIEF && !submitting;

  const submit = async () => {
    if (!canSubmit) return;
    setSubmitting(true);
    setError(null);
    try {
      const kws = keywords.split(',').map((s) => s.trim()).filter(Boolean);
      const result = await createReel({ brief_style: briefTrim, keywords: kws, target_duration_sec: duration });
      setBrief('');
      setKeywords('');
      setDuration(DEFAULT_DURATION);
      setAdvancedOpen(false);
      onCreated?.(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'error');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={(e) => { e.preventDefault(); void submit(); }} style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
      <div>
        <label style={{ display: 'block', fontSize: 12, color: 'var(--text-faint)', marginBottom: 6 }}>
          {tr.briefLabel} <span style={{ color: 'var(--hot)' }}>*</span>
        </label>
        <textarea
          className="input"
          value={brief}
          onChange={(e) => setBrief(e.target.value)}
          placeholder={tr.briefPlaceholder}
          rows={3}
          style={{ width: '100%', resize: 'vertical' }}
        />
        {briefTrim !== '' && briefTrim.length < MIN_BRIEF && (
          <p style={{ fontSize: 11, margin: '4px 0 0', color: 'var(--hot)' }}>{tr.briefTooShort}</p>
        )}
      </div>

      <div>
        <label style={{ display: 'block', fontSize: 12, color: 'var(--text-faint)', marginBottom: 6 }}>{tr.kwLabel}</label>
        <input className="input" value={keywords} onChange={(e) => setKeywords(e.target.value)} placeholder={tr.kwPlaceholder} style={{ width: '100%' }} />
      </div>

      <div>
        <button
          type="button"
          onClick={() => setAdvancedOpen((v) => !v)}
          style={{ background: 'transparent', border: 0, padding: 0, cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 4, color: 'var(--text-mute)', fontSize: 12 }}
        >
          {advancedOpen ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
          {tr.advanced}
        </button>
        {advancedOpen && (
          <div style={{ marginTop: 10, padding: 12, background: 'var(--surface-alt)', borderRadius: 8 }}>
            <label style={{ display: 'block', fontSize: 11, color: 'var(--text-faint)', marginBottom: 6 }}>{tr.durationLabel}</label>
            <input type="range" min={15} max={60} step={5} value={duration} onChange={(e) => setDuration(Number(e.target.value))} style={{ width: '100%' }} />
            <div className="mono" style={{ fontSize: 11, color: 'var(--text-mute)', marginTop: 3 }}>{tr.durationHint(duration)}</div>
          </div>
        )}
      </div>

      {error && (
        <div style={{ display: 'flex', gap: 8, alignItems: 'flex-start', fontSize: 12, color: 'var(--hot)', background: 'rgba(220,40,40,0.08)', padding: '8px 10px', borderRadius: 8 }}>
          <AlertCircle size={14} style={{ flexShrink: 0, marginTop: 1 }} />
          <span>{error}</span>
        </div>
      )}

      <div style={{ display: 'flex', gap: 10, justifyContent: 'flex-end' }}>
        {onCancel && (
          <button type="button" className="btn btn-ghost" onClick={onCancel} disabled={submitting}>{tr.cancel}</button>
        )}
        <button type="submit" className="btn btn-primary" disabled={!canSubmit}>
          <Send size={14} /> {submitting ? tr.submitting : tr.submit}
        </button>
      </div>
    </form>
  );
}
