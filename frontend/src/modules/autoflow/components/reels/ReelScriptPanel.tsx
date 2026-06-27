'use client';

import { useState } from 'react';
import { Save, AlertTriangle } from 'lucide-react';
import type { ReelStrings } from '../../i18n/reelStrings';
import { parseShots, parseFlags, updateScriptCaption, type ReelScript } from '../../services/reelsService';

interface ReelScriptPanelProps {
  reelId: number;
  script: ReelScript;
  editable: boolean;
  tr: ReelStrings;
  onSaved: () => void;
}

// ReelScriptPanel shows the AI script (dialogue + shot list, read-only) and lets the user
// edit just the caption → PATCH /reels/:id/script (creates version +1). Verify flags are
// surfaced as a warning chip so users don't post unverified claims (grounding rule).
export default function ReelScriptPanel({ reelId, script, editable, tr, onSaved }: Readonly<ReelScriptPanelProps>) {
  const [caption, setCaption] = useState(script.caption);
  const [saving, setSaving] = useState(false);
  const shots = parseShots(script.shot_list);
  const flags = parseFlags(script.verify_flags);

  const save = async () => {
    if (saving) return;
    setSaving(true);
    try {
      await updateScriptCaption(reelId, caption);
      onSaved();
    } finally {
      setSaving(false);
    }
  };

  return (
    <div style={{ background: 'var(--surface-alt)', borderRadius: 8, padding: 12, marginBottom: 4 }}>
      <p style={{ color: 'var(--text-mute)', fontSize: 12.5, lineHeight: 1.5, whiteSpace: 'pre-wrap', marginTop: 0 }}>
        {script.dialogue}
      </p>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 4, margin: '10px 0' }}>
        {shots.map((s) => (
          <div key={s.scene} className="mono" style={{ fontSize: 11, color: 'var(--text-faint)' }}>
            {s.scene}. <span style={{ color: 'var(--text-mute)' }}>{s.kind}</span> · {s.prompt} · {s.dur_sec}s
          </div>
        ))}
      </div>

      {flags.length > 0 && (
        <div style={{ display: 'flex', gap: 6, alignItems: 'flex-start', fontSize: 11, color: 'var(--hot)', marginBottom: 10 }}>
          <AlertTriangle size={13} style={{ flexShrink: 0, marginTop: 1 }} />
          <span>{tr.verifyHeading}: {flags.join(' · ')}</span>
        </div>
      )}

      <label style={{ display: 'block', fontSize: 11, color: 'var(--text-faint)', marginBottom: 6 }}>{tr.captionLabel}</label>
      <input
        className="input"
        value={caption}
        disabled={!editable}
        onChange={(e) => setCaption(e.target.value)}
        style={{ width: '100%' }}
      />
      {editable && (
        <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 10 }}>
          <button type="button" className="btn btn-ghost btn-sm" disabled={saving || caption === script.caption} onClick={() => void save()}>
            <Save size={12} /> {tr.saveScript}
          </button>
        </div>
      )}
    </div>
  );
}
