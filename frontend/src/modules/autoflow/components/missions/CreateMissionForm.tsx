'use client';

import { useEffect, useMemo, useState } from 'react';
import { Send, ChevronDown, ChevronRight, AlertCircle } from 'lucide-react';
import { createMission, type CreateMissionInput, type CrawlIntent } from '../../services/crawlIntentService';
import { useWorkspaces } from '../../hooks/useWorkspaces';
import { useLang } from '../../i18n/useLang';

interface CreateMissionFormProps {
  onCreated?: (intent: CrawlIntent, created: boolean) => void;
  onCancel?: () => void;
  compact?: boolean;
}

const FACEBOOK_URL_RE = /^https?:\/\/(www\.|m\.)?facebook\.com\//i;
const MIN_PROMPT = 20;
const DEFAULT_INTERVAL = 60;
const DEFAULT_MAX_ITEMS = 50;

export default function CreateMissionForm({ onCreated, onCancel, compact }: CreateMissionFormProps) {
  const { t } = useLang();
  const tm = t.missionsView;
  const { workspaces } = useWorkspaces();

  const activeAccounts = useMemo(
    () => workspaces.filter((w) => w.running || w.loggedIn),
    [workspaces],
  );

  const [prompt, setPrompt] = useState('');
  const [sourceUrl, setSourceUrl] = useState('');
  const [intervalMinutes, setIntervalMinutes] = useState<number>(DEFAULT_INTERVAL);
  const [maxItems, setMaxItems] = useState<number>(DEFAULT_MAX_ITEMS);
  const [accountId, setAccountId] = useState<number | ''>('');
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // PR-A Mission Preflight: the account is REQUIRED — the system must never
  // auto-pick or silently fall back. Preselect only when exactly one account is
  // ready, purely as a UX convenience; the user still confirms by submitting.
  useEffect(() => {
    if (accountId === '' && activeAccounts.length === 1) {
      setAccountId(activeAccounts[0].accountId);
    }
  }, [activeAccounts, accountId]);

  const promptTrim = prompt.trim();
  const urlTrim = sourceUrl.trim();
  const promptValid = promptTrim.length >= MIN_PROMPT;
  const urlValid = FACEBOOK_URL_RE.test(urlTrim);
  const canSubmit = promptValid && urlValid && accountId !== '' && !submitting;

  const submit = async () => {
    if (!canSubmit) return;
    setSubmitting(true);
    setError(null);
    try {
      const payload: CreateMissionInput = {
        prompt: promptTrim,
        source_url: urlTrim,
        interval_minutes: intervalMinutes,
        max_items: maxItems,
        // Required (PR-A): canSubmit guarantees a chosen account; never omit it.
        account_id: accountId as number,
      };
      const result = await createMission(payload);
      setPrompt('');
      setSourceUrl('');
      setIntervalMinutes(DEFAULT_INTERVAL);
      setMaxItems(DEFAULT_MAX_ITEMS);
      setAccountId('');
      setAdvancedOpen(false);
      onCreated?.(result.intent, result.created);
    } catch (err) {
      setError(err instanceof Error ? err.message : t.common.error);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form
      onSubmit={(e) => { e.preventDefault(); void submit(); }}
      style={{ display: 'flex', flexDirection: 'column', gap: 14 }}
    >
      <div>
        <label style={{ display: 'block', fontSize: 12, color: 'var(--text-faint)', marginBottom: 6 }}>
          {tm.formPromptLabel}
        </label>
        <textarea
          className="input"
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          placeholder={tm.formPromptPlaceholder}
          rows={compact ? 2 : 3}
          style={{ width: '100%', resize: 'vertical', lineHeight: 1.5 }}
        />
        <p style={{ fontSize: 11, color: promptValid ? 'var(--text-faint)' : 'var(--text-mute)', marginTop: 4, margin: '4px 0 0' }}>
          {promptValid ? tm.formPromptHelp : tm.formPromptShort(MIN_PROMPT)}
        </p>
      </div>

      <div>
        <label style={{ display: 'block', fontSize: 12, color: 'var(--text-faint)', marginBottom: 6 }}>
          {tm.formUrlLabel}
        </label>
        <input
          type="url"
          className="input"
          value={sourceUrl}
          onChange={(e) => setSourceUrl(e.target.value)}
          placeholder={tm.formUrlPlaceholder}
          style={{ width: '100%' }}
        />
        <p style={{ fontSize: 11, marginTop: 4, margin: '4px 0 0', color: urlTrim === '' || urlValid ? 'var(--text-faint)' : 'var(--hot)' }}>
          {urlTrim === '' || urlValid ? tm.formUrlHelp : tm.formUrlInvalid}
        </p>
      </div>

      <div>
        <label style={{ display: 'block', fontSize: 12, color: 'var(--text-faint)', marginBottom: 6 }}>
          {tm.formAccountLabel} <span style={{ color: 'var(--hot)' }}>*</span>
        </label>
        {activeAccounts.length === 0 ? (
          <div style={{ fontSize: 12, color: 'var(--hot)', background: 'rgba(220,40,40,0.08)', padding: '8px 10px', borderRadius: 8 }}>
            Chưa có account nào sẵn sàng. Mở Browser, pair Chrome Extension và đăng nhập Facebook cho account muốn chạy, rồi quay lại.
          </div>
        ) : (
          <select
            className="input"
            value={accountId}
            onChange={(e) => setAccountId(e.target.value ? Number(e.target.value) : '')}
            required
            style={{ width: '100%' }}
          >
            <option value="" disabled>— Chọn account sẽ chạy nhiệm vụ —</option>
            {activeAccounts.map((w) => (
              <option key={w.accountId} value={w.accountId}>
                {w.accountName}{w.fbUsername ? ` · ${w.fbUsername}` : ''} {w.loggedIn ? '· đã đăng nhập FB' : '· đang chạy'}
              </option>
            ))}
          </select>
        )}
        {accountId === '' && activeAccounts.length > 0 && (
          <p style={{ fontSize: 11, color: 'var(--text-mute)', margin: '4px 0 0' }}>
            Bắt buộc chọn account — hệ thống không tự chọn account thay bạn.
          </p>
        )}
      </div>

      <div>
        <button
          type="button"
          onClick={() => setAdvancedOpen((v) => !v)}
          style={{
            background: 'transparent',
            border: 0,
            padding: 0,
            cursor: 'pointer',
            display: 'flex',
            alignItems: 'center',
            gap: 4,
            color: 'var(--text-mute)',
            fontSize: 12,
          }}
        >
          {advancedOpen ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
          {tm.formAdvancedLabel}
        </button>
        {advancedOpen && (
          <div style={{ display: 'grid', gridTemplateColumns: compact ? '1fr' : '1fr 1fr', gap: 12, marginTop: 10, padding: 12, background: 'var(--surface-alt)', borderRadius: 8 }}>
            <div>
              <label style={{ display: 'block', fontSize: 11, color: 'var(--text-faint)', marginBottom: 6 }}>
                {tm.formIntervalLabel}
              </label>
              <input
                type="range"
                min={30}
                max={1440}
                step={30}
                value={intervalMinutes}
                onChange={(e) => setIntervalMinutes(Number(e.target.value))}
                style={{ width: '100%' }}
              />
              <div className="mono" style={{ fontSize: 11, color: 'var(--text-mute)', marginTop: 3 }}>
                {tm.formIntervalHint(intervalMinutes)}
              </div>
            </div>
            <div>
              <label style={{ display: 'block', fontSize: 11, color: 'var(--text-faint)', marginBottom: 6 }}>
                {tm.formMaxItemsLabel}
              </label>
              <input
                type="number"
                className="input"
                min={10}
                max={250}
                value={maxItems}
                onChange={(e) => setMaxItems(Math.max(10, Math.min(250, Number(e.target.value) || DEFAULT_MAX_ITEMS)))}
                style={{ width: '100%' }}
              />
            </div>
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
          <button type="button" className="btn btn-ghost" onClick={onCancel} disabled={submitting}>
            {tm.cancelCta}
          </button>
        )}
        <button type="submit" className="btn btn-primary" disabled={!canSubmit}>
          <Send size={14} />
          {submitting ? tm.formSubmittingCta : tm.formSubmitCta}
        </button>
      </div>
    </form>
  );
}
