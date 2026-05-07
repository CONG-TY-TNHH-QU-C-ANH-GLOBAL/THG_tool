'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Bot, CheckCircle2, Clock, Cpu, RefreshCw, Send, Trash2, UserRound } from 'lucide-react';
import { useWorkspaces } from '../../hooks/useWorkspaces';
import {
  clearAgentHistory,
  deleteAgentHistoryItem,
  getAgentHistory,
  sendAgentPrompt,
  type AgentChatHistoryItem,
} from '../../services/agentChatService';
import { getCrawlIntents, type CrawlIntent } from '../../services/crawlIntentService';
import { useLang } from '../../i18n/useLang';
import type { DashboardStrings } from '../../i18n/strings';

interface WorkspaceChatViewProps {
  orgId: string;
}

type ChatRole = 'user' | 'assistant' | 'system';

interface ChatMessage {
  id: string;
  role: ChatRole;
  text: string;
  time: string;
  ok?: boolean;
  historyId?: number;
}

function nowLabel(locale: string) {
  return new Date().toLocaleTimeString(locale, { hour: '2-digit', minute: '2-digit' });
}

function historyTimeLabel(value: string, locale: string) {
  const ts = new Date(value);
  if (Number.isNaN(ts.getTime())) return nowLabel(locale);
  return ts.toLocaleTimeString(locale, { hour: '2-digit', minute: '2-digit' });
}

function scheduleLabel(value: string | undefined, tv: DashboardStrings['chatView'], locale: string) {
  if (!value) return '—';
  const timestamp = new Date(value).getTime();
  if (!Number.isFinite(timestamp)) return '—';
  const diff = timestamp - Date.now();
  if (diff <= 0) return tv.schedulePending;
  const minutes = Math.ceil(diff / 60000);
  if (minutes < 60) return tv.scheduleInMinutes(minutes);
  return new Date(value).toLocaleString(locale, {
    hour: '2-digit',
    minute: '2-digit',
    day: '2-digit',
    month: '2-digit',
  });
}

function flattenHistory(items: AgentChatHistoryItem[], locale: string): ChatMessage[] {
  const next: ChatMessage[] = [];
  for (const item of items) {
    const time = historyTimeLabel(item.createdAt, locale);
    if (item.source === 'system' || item.actionTaken.startsWith('system_')) {
      next.push({
        id: `${item.id}-s`,
        historyId: item.id,
        role: 'system',
        text: item.aiResponse || item.userPrompt,
        time,
        ok: item.success,
      });
      continue;
    }
    next.push({
      id: `${item.id}-u`,
      historyId: item.id,
      role: 'user',
      text: item.userPrompt,
      time,
      ok: item.success,
    });
    next.push({
      id: `${item.id}-a`,
      historyId: item.id,
      role: 'assistant',
      text: item.aiResponse,
      time,
      ok: item.success,
    });
  }
  return next;
}

function workspaceIdentityLabel(workspace: {
  fbDisplayName?: string;
  fbUsername?: string;
  email?: string;
  fbUserId?: string;
  accountName: string;
}) {
  return workspace.fbDisplayName
    || workspace.fbUsername
    || workspace.email
    || workspace.fbUserId
    || workspace.accountName;
}

export default function WorkspaceChatView({ orgId }: WorkspaceChatViewProps) {
  void orgId;
  const { lang, t } = useLang();
  const tv = t.chatView;
  const locale = lang === 'vi' ? 'vi-VN' : 'en-US';
  const { workspaces, refresh } = useWorkspaces();
  const [accountId, setAccountId] = useState<number | ''>('');
  const [draft, setDraft] = useState('');
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [crawlIntents, setCrawlIntents] = useState<CrawlIntent[]>([]);
  const [loadingHistory, setLoadingHistory] = useState(true);
  const [loadingIntents, setLoadingIntents] = useState(true);
  const [sending, setSending] = useState(false);
  const [deletingHistoryId, setDeletingHistoryId] = useState<number | null>(null);
  const [clearingHistory, setClearingHistory] = useState(false);
  const [compact, setCompact] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);

  const activeAccounts = useMemo(
    () => workspaces.filter((workspace) => workspace.running || workspace.loggedIn),
    [workspaces],
  );
  const selectedAccount = activeAccounts.find((workspace) => workspace.accountId === accountId);
  const enabledIntents = crawlIntents.filter((intent) => intent.enabled);

  const loadCrawlIntents = useCallback(async () => {
    setLoadingIntents(true);
    try {
      setCrawlIntents(await getCrawlIntents());
    } catch {
      setCrawlIntents([]);
    } finally {
      setLoadingIntents(false);
    }
  }, []);

  const loadHistory = useCallback(async () => {
    setLoadingHistory(true);
    try {
      setMessages(flattenHistory(await getAgentHistory(18), locale));
    } catch {
      setMessages([]);
    } finally {
      setLoadingHistory(false);
    }
  }, [locale]);

  useEffect(() => {
    if (accountId !== '' || activeAccounts.length === 0) return;
    const loggedIn = activeAccounts.find((workspace) => workspace.loggedIn);
    setAccountId((loggedIn ?? activeAccounts[0]).accountId);
  }, [accountId, activeAccounts]);

  useEffect(() => {
    void loadHistory();
  }, [loadHistory]);

  useEffect(() => {
    void loadCrawlIntents();
  }, [loadCrawlIntents]);

  useEffect(() => {
    const id = window.setInterval(() => {
      void loadHistory();
      void loadCrawlIntents();
    }, 10000);
    return () => window.clearInterval(id);
  }, [loadCrawlIntents, loadHistory]);

  useEffect(() => {
    const updateCompact = () => setCompact(window.innerWidth < 1180);
    updateCompact();
    window.addEventListener('resize', updateCompact);
    return () => window.removeEventListener('resize', updateCompact);
  }, []);

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: 'smooth' });
  }, [messages, sending]);

  const appendSystemMessage = (text: string) => {
    setMessages((prev) => [
      ...prev,
      { id: `system-${Date.now()}`, role: 'system', text, time: nowLabel(locale), ok: false },
    ]);
  };

  const handleSend = async () => {
    const text = draft.trim();
    if (!text || sending) return;
    setDraft('');
    setMessages((prev) => [
      ...prev,
      { id: `pending-user-${Date.now()}`, role: 'user', text, time: nowLabel(locale), ok: true },
    ]);
    setSending(true);
    try {
      await sendAgentPrompt(text, accountId === '' ? undefined : accountId);
      sessionStorage.setItem('autoflow:last_crawl_dispatch', String(Date.now()));
      await loadHistory();
      void refresh();
      void loadCrawlIntents();
    } catch (error) {
      appendSystemMessage(error instanceof Error ? error.message : tv.copilotErrorFallback);
    } finally {
      setSending(false);
    }
  };

  const handleDeleteHistoryItem = async (historyId: number) => {
    if (deletingHistoryId === historyId || sending) return;
    if (!window.confirm(tv.confirmDeleteTurn)) return;
    setDeletingHistoryId(historyId);
    try {
      await deleteAgentHistoryItem(historyId);
      setMessages((prev) => prev.filter((message) => message.historyId !== historyId));
    } catch (error) {
      appendSystemMessage(error instanceof Error ? error.message : tv.deleteError);
    } finally {
      setDeletingHistoryId(null);
    }
  };

  const handleClearHistory = async () => {
    if (clearingHistory || sending) return;
    if (!window.confirm(tv.confirmClearAll)) return;
    setClearingHistory(true);
    try {
      await clearAgentHistory();
      setMessages([]);
    } catch (error) {
      appendSystemMessage(error instanceof Error ? error.message : tv.clearError);
    } finally {
      setClearingHistory(false);
    }
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16, height: 'calc(100vh - 56px - 48px)' }}>
      <header>
        <div className="eyebrow"><span className="dot" />{tv.eyebrow}</div>
        <h2 style={{ fontSize: 28, marginTop: 8 }}>{t.views.chatTitle || 'Workspace chat'}</h2>
        <p style={{ color: 'var(--text-mute)', fontSize: 13.5, marginTop: 6 }}>{t.views.chatSub}</p>
      </header>

      <div style={{ display: 'grid', gridTemplateColumns: compact ? 'minmax(0, 1fr)' : 'minmax(0, 1fr) 300px', gap: 16, flex: 1, minHeight: 480 }}>
        <div className="card" style={{ padding: 0, display: 'flex', flexDirection: 'column' }}>
          <header style={{ display: 'flex', alignItems: 'center', gap: 12, padding: 16, borderBottom: '1px solid var(--line)' }}>
            <div style={{ flex: 1, minWidth: 0, display: 'flex', alignItems: 'center', gap: 10 }}>
              <span className="avatar" style={{ background: 'var(--accent)', color: 'var(--accent-ink)', borderColor: 'var(--accent)' }}>A</span>
              <div>
                <div style={{ fontWeight: 500, color: 'var(--text)' }}>{tv.copilotName}</div>
                <div className="mono" style={{ fontSize: 11, color: 'var(--text-faint)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {selectedAccount ? tv.copilotSubtitleWith(selectedAccount.accountName) : tv.copilotSubtitleNoAccount}
                </div>
              </div>
            </div>
            <button type="button" className="btn btn-ghost btn-sm" onClick={() => void loadHistory()}>
              <RefreshCw size={12} />
              {t.common.refresh}
            </button>
            <button
              type="button"
              className="btn btn-ghost btn-sm"
              onClick={() => void handleClearHistory()}
              disabled={clearingHistory || messages.length === 0}
              style={{ color: 'var(--hot)', opacity: clearingHistory || messages.length === 0 ? 0.5 : 1 }}
            >
              <Trash2 size={12} />
              {clearingHistory ? tv.clearingHistoryLabel : tv.clearHistoryLabel}
            </button>
          </header>

          <div ref={scrollRef} style={{ flex: 1, overflowY: 'auto', padding: 16, display: 'flex', flexDirection: 'column', gap: 12 }}>
            {loadingHistory && (
              <div style={{ color: 'var(--text-mute)', fontSize: 13, display: 'flex', alignItems: 'center', gap: 8 }}>
                <div className="skeleton" style={{ width: 220, height: 14 }} />
              </div>
            )}

            {!loadingHistory && messages.length === 0 && (
              <div className="empty" style={{ margin: 'auto' }}>
                <div className="eyebrow"><span className="dot" />{tv.emptyEyebrow}</div>
                <h3>{tv.emptyTitle}</h3>
                <p>{tv.emptyDesc}</p>
              </div>
            )}

            {messages.map((message) => {
              const isUser = message.role === 'user';
              const isSystem = message.role === 'system';
              const canDelete = message.role === 'assistant' && !!message.historyId;
              const deletingThis = canDelete && deletingHistoryId === message.historyId;
              const senderLabel = isUser ? tv.senderYou : isSystem ? tv.senderSystem : tv.senderCopilot;

              return (
                <div key={message.id} style={{ display: 'flex', gap: 12, alignItems: 'flex-start' }}>
                  <span className={`avatar ${isUser ? 'avatar-sm' : ''}`}
                    style={!isUser ? { background: isSystem ? 'var(--hot)' : 'var(--accent)', color: isSystem ? '#fff' : 'var(--accent-ink)', borderColor: isSystem ? 'var(--hot)' : 'var(--accent)' } : {}}>
                    {isUser ? 'U' : isSystem ? 'S' : 'A'}
                  </span>
                  <div style={{ flex: 1 }}>
                    <div className="mono" style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 10.5, color: 'var(--text-faint)', letterSpacing: '0.1em', marginBottom: 6 }}>
                      {senderLabel.toUpperCase()}
                      <span style={{ opacity: 0.5 }}>· {message.time}</span>
                      {canDelete && (
                        <button
                          type="button"
                          onClick={() => void handleDeleteHistoryItem(message.historyId!)}
                          disabled={!!deletingThis}
                          aria-label={tv.deleteAria}
                          style={{
                            background: 'transparent',
                            border: 0,
                            color: 'inherit',
                            cursor: deletingThis ? 'not-allowed' : 'pointer',
                            padding: 0,
                            display: 'grid',
                            placeItems: 'center',
                          }}
                        >
                          <Trash2 size={11} />
                        </button>
                      )}
                    </div>
                    <div style={{ fontSize: 14, lineHeight: 1.55, whiteSpace: 'pre-wrap', color: isSystem ? 'var(--hot)' : 'var(--text)' }}>
                      {message.text}
                    </div>
                  </div>
                </div>
              );
            })}

            {sending && (
              <div className="mono" style={{ color: 'var(--text-mute)', fontSize: 12, display: 'flex', alignItems: 'center', gap: 8 }}>
                <span className="pulse" />
                {tv.thinking}
              </div>
            )}
          </div>

          <div style={{ padding: 12, borderTop: '1px solid var(--line)', display: 'flex', gap: 10, alignItems: 'flex-end', flexWrap: compact ? 'wrap' : 'nowrap' }}>
            <textarea
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) {
                  event.preventDefault();
                  void handleSend();
                }
              }}
              placeholder={tv.placeholderInput}
              rows={3}
              className="input"
              style={{ flex: 1, minWidth: compact ? '100%' : 0, resize: 'none', lineHeight: 1.5 }}
            />
            <button
              type="button"
              className="btn btn-primary btn-icon"
              onClick={() => void handleSend()}
              disabled={sending || !draft.trim()}
              style={{ width: 42, height: 42, opacity: sending || !draft.trim() ? 0.55 : 1 }}
              aria-label={tv.sendAria}
            >
              <Send size={16} />
            </button>
          </div>
        </div>

        <aside style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
          <div className="card">
            <div className="eyebrow" style={{ marginBottom: 8 }}>{tv.accountLabel}</div>
            <select
              className="input"
              value={accountId}
              onChange={(event) => setAccountId(event.target.value ? Number(event.target.value) : '')}
            >
              <option value="">{tv.accountAuto}</option>
              {activeAccounts.map((workspace) => (
                <option key={workspace.accountId} value={workspace.accountId}>
                  {workspace.accountName} | {workspaceIdentityLabel(workspace)}
                </option>
              ))}
            </select>

            {activeAccounts.length === 0 && (
              <div className="banner" style={{ marginTop: 12, fontSize: 12 }}>
                <Cpu size={14} style={{ color: 'var(--info)', flexShrink: 0 }} />
                <div>
                  <div className="mono" style={{ fontSize: 10, letterSpacing: '0.1em', color: 'var(--text-faint)', marginBottom: 4 }}>
                    {tv.noteEyebrow}
                  </div>
                  {tv.noAccountWarning}
                </div>
              </div>
            )}

            {selectedAccount && (
              <dl style={{ marginTop: 12, display: 'flex', flexDirection: 'column', gap: 6, fontSize: 12 }}>
                {[
                  [tv.fieldRunning, selectedAccount.running ? tv.valYes : tv.valNo],
                  [tv.fieldSession, selectedAccount.loggedIn ? tv.valSaved : tv.valPending],
                  [tv.fieldIdentity, workspaceIdentityLabel(selectedAccount)],
                  [tv.fieldFbId, selectedAccount.fbUserId ?? '—'],
                ].map(([key, value]) => (
                  <div key={key} style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
                    <dt style={{ color: 'var(--text-faint)' }}>{key}</dt>
                    <dd className="mono" style={{ color: 'var(--text)', overflow: 'hidden', textOverflow: 'ellipsis', margin: 0 }}>
                      {value}
                    </dd>
                  </div>
                ))}
              </dl>
            )}
          </div>

          <div className="card">
            <div className="eyebrow" style={{ marginBottom: 8 }}>{tv.connectorLabel}</div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, color: 'var(--text-mute)', fontSize: 13 }}>
              <CheckCircle2 size={14} style={{ color: activeAccounts.length > 0 ? 'var(--ok)' : 'var(--text-faint)' }} />
              <span className="mono tabular">{activeAccounts.length}</span>
              <span>{tv.connectorSuffix}</span>
            </div>
          </div>

          <div className="card">
            <div className="eyebrow" style={{ marginBottom: 8 }}>{tv.automationLabel}</div>
            {loadingIntents && <div className="skeleton" style={{ height: 14, marginTop: 4 }} />}
            {!loadingIntents && enabledIntents.length === 0 && (
              <p style={{ fontSize: 12, lineHeight: 1.5 }}>{tv.automationEmpty}</p>
            )}

            {!loadingIntents && enabledIntents.slice(0, 4).map((intent) => (
              <div key={intent.id} style={{ borderTop: '1px solid var(--line)', paddingTop: 10, marginTop: 10 }}>
                <div style={{ fontSize: 12, fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {intent.name || intent.source_type}
                </div>
                <div className="mono" style={{ fontSize: 11, color: 'var(--text-faint)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', marginTop: 3 }}>
                  {intent.source_url}
                </div>
                <div style={{ marginTop: 6, display: 'flex', alignItems: 'center', justifyContent: 'space-between', fontSize: 11, gap: 8 }}>
                  <span style={{ display: 'flex', alignItems: 'center', gap: 5, color: 'var(--text-mute)' }}>
                    <Clock size={11} />
                    {tv.automationEvery(intent.interval_minutes)}
                  </span>
                  <span className="mono" style={{ color: intent.last_error ? 'var(--hot)' : 'var(--ok)' }}>
                    {intent.last_error ? tv.automationError : scheduleLabel(intent.next_run_at, tv, locale)}
                  </span>
                </div>
              </div>
            ))}
          </div>
        </aside>
      </div>
    </div>
  );
}
