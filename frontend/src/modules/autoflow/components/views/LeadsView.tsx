'use client';

import { useEffect, useMemo, useState } from 'react';
import { ExternalLink, RefreshCw, Search, Trash2 } from 'lucide-react';
import type { Lead, LeadStatus } from '../../types';
import { useLeads } from '../../hooks/useLeads';
import { useLang } from '../../i18n/useLang';

interface LeadsViewProps {
  orgId: string;
  isAdmin: boolean;
}

const FILTERS: Array<LeadStatus | 'All'> = ['All', 'Hot', 'Warm', 'Cold'];

type IntentKey = 'all' | 'candidate' | 'potential_customer' | 'partner' | 'provider_ad' | 'spam' | 'unknown';
const INTENT_FILTERS: Array<{ key: IntentKey; label: string }> = [
  { key: 'all', label: 'Tất cả tệp' },
  { key: 'potential_customer', label: 'Khách quan tâm' },
  { key: 'candidate', label: 'Ứng viên / Tuyển dụng' },
  { key: 'partner', label: 'Đối tác / Reseller' },
  { key: 'provider_ad', label: 'Provider quảng cáo' },
  { key: 'spam', label: 'Spam' },
  { key: 'unknown', label: 'Chưa phân loại' },
];

function intentDisplay(raw?: string): { label: string; className: string } {
  const v = (raw ?? '').toLowerCase().trim();
  switch (v) {
    case 'candidate':
      return { label: 'Ứng viên', className: 'tag tag-info' };
    case 'potential_customer':
      return { label: 'Khách quan tâm', className: 'tag tag-ok' };
    case 'partner':
      return { label: 'Đối tác', className: 'tag tag-warm' };
    case 'provider_ad':
      return { label: 'Provider', className: 'tag tag-mute' };
    case 'not_relevant':
      return { label: 'Không liên quan', className: 'tag tag-mute' };
    case 'spam':
      return { label: 'Spam', className: 'tag tag-hot' };
    default:
      return { label: 'Chưa rõ', className: 'tag tag-mute' };
  }
}

function leadIntentKey(lead: Lead): IntentKey {
  const v = (lead.agent ?? '').toLowerCase().trim();
  if (v === 'candidate' || v === 'potential_customer' || v === 'partner' || v === 'provider_ad' || v === 'spam') {
    return v;
  }
  return 'unknown';
}

function statusTagClass(status: string): string {
  switch (status) {
    case 'Hot':
      return 'tag tag-hot';
    case 'Warm':
      return 'tag tag-warm';
    case 'Cold':
      return 'tag tag-cold';
    case 'Active':
    case 'Converted':
      return 'tag tag-ok';
    default:
      return 'tag tag-mute';
  }
}

function leadSearchValue(lead: Lead) {
  return [lead.name, lead.group, lead.agent, lead.phone, lead.facebookUrl ?? '', lead.postUrl ?? '']
    .join(' ')
    .toLowerCase();
}

export default function LeadsView({ orgId, isAdmin }: LeadsViewProps) {
  void isAdmin;
  const { lang, t } = useLang();
  const tv = t.leadsView;
  const locale = lang === 'vi' ? 'vi-VN' : 'en-US';
  const [filter, setFilter] = useState<LeadStatus | 'All'>('All');
  const [intentFilter, setIntentFilter] = useState<IntentKey>('all');
  const [query, setQuery] = useState('');
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const { leads, isLoading, error, refetch, remove } = useLeads(orgId, filter);
  const [deletingId, setDeletingId] = useState<number | null>(null);

  const handleDelete = async (lead: Lead) => {
    if (deletingId !== null) return;
    if (typeof window !== 'undefined') {
      const ok = window.confirm(`Xoá lead này?\n\n${lead.name}`);
      if (!ok) return;
    }
    setDeletingId(lead.id);
    try {
      await remove(lead.id, lead.sourceType);
    } catch (err) {
      if (typeof window !== 'undefined') {
        window.alert(err instanceof Error ? err.message : String(err));
      }
    } finally {
      setDeletingId(null);
    }
  };

  const filteredLeads = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    return leads.filter((lead) => {
      if (intentFilter !== 'all' && leadIntentKey(lead) !== intentFilter) return false;
      if (normalized && !leadSearchValue(lead).includes(normalized)) return false;
      return true;
    });
  }, [leads, query, intentFilter]);

  const intentCounts = useMemo(() => {
    const counts: Record<IntentKey, number> = {
      all: leads.length,
      potential_customer: 0,
      candidate: 0,
      partner: 0,
      provider_ad: 0,
      spam: 0,
      unknown: 0,
    };
    for (const lead of leads) {
      const key = leadIntentKey(lead);
      counts[key] = (counts[key] ?? 0) + 1;
    }
    return counts;
  }, [leads]);

  useEffect(() => {
    if (filteredLeads.length === 0) {
      setSelectedId(null);
      return;
    }
    if (!filteredLeads.some((lead) => lead.id === selectedId)) {
      setSelectedId(filteredLeads[0].id);
    }
  }, [filteredLeads, selectedId]);

  const selectedLead = filteredLeads.find((lead) => lead.id === selectedId) ?? null;
  const totals = {
    all: leads.length,
    hot: leads.filter((lead) => lead.status === 'Hot').length,
    warm: leads.filter((lead) => lead.status === 'Warm').length,
    avgScore: leads.length ? Math.round(leads.reduce((sum, lead) => sum + lead.score, 0) / leads.length) : 0,
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
      <header style={{ display: 'flex', alignItems: 'flex-start', gap: 16, flexWrap: 'wrap' }}>
        <div>
          <div className="eyebrow">
            <span className="dot" />
            {tv.eyebrowSales}
          </div>
          <h2 style={{ fontSize: 28, marginTop: 8 }}>{t.views.leadsTitle}</h2>
          <p style={{ color: 'var(--text-mute)', fontSize: 13.5, marginTop: 6 }}>{t.views.leadsSub}</p>
        </div>
        <div style={{ flex: 1 }} />
        <button className="btn btn-ghost btn-sm" type="button" onClick={() => void refetch()}>
          <RefreshCw size={13} />
          {t.common.refresh}
        </button>
      </header>

      <div className="stats-grid">
        <div className="stat">
          <div className="stat-label">{tv.statTotal}</div>
          <div className="stat-value tabular">{totals.all.toLocaleString(locale)}</div>
        </div>
        <div className="stat">
          <div className="stat-label">{tv.statHot}</div>
          <div className="stat-value tabular" style={{ color: 'var(--hot)' }}>{totals.hot}</div>
        </div>
        <div className="stat">
          <div className="stat-label">{tv.statWarm}</div>
          <div className="stat-value tabular" style={{ color: 'var(--warn)' }}>{totals.warm}</div>
        </div>
        <div className="stat">
          <div className="stat-label">{tv.statAvgScore}</div>
          <div className="stat-value tabular">{totals.avgScore}</div>
        </div>
      </div>

      <div className="card" style={{ padding: 0, overflow: 'hidden', minHeight: 560 }}>
        <div className="three-pane" style={{ minHeight: 560 }}>
          <aside style={{ padding: 16 }}>
            <div className="sidebar-section">{tv.filtersLabel}</div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
              {FILTERS.map((item) => {
                const count = item === 'All' ? totals.all : leads.filter((lead) => lead.status === item).length;
                const label = item === 'All' ? tv.filterAll : item;
                return (
                  <button
                    key={item}
                    type="button"
                    className={`filter-pill ${filter === item ? 'is-active' : ''}`}
                    style={{ justifyContent: 'space-between', display: 'flex', textAlign: 'left' }}
                    onClick={() => setFilter(item)}
                  >
                    <span>{label}</span>
                    <span style={{ opacity: 0.7 }}>{count}</span>
                  </button>
                );
              })}
            </div>

            <div className="sidebar-section" style={{ marginTop: 16 }}>TỆP / INTENT</div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
              {INTENT_FILTERS.map((opt) => (
                <button
                  key={opt.key}
                  type="button"
                  className={`filter-pill ${intentFilter === opt.key ? 'is-active' : ''}`}
                  style={{ justifyContent: 'space-between', display: 'flex', textAlign: 'left' }}
                  onClick={() => setIntentFilter(opt.key)}
                >
                  <span>{opt.label}</span>
                  <span style={{ opacity: 0.7 }}>{intentCounts[opt.key] ?? 0}</span>
                </button>
              ))}
            </div>

            <div className="sidebar-section" style={{ marginTop: 16 }}>{tv.searchLabel}</div>
            <div style={{ position: 'relative' }}>
              <Search size={13} style={{ position: 'absolute', left: 12, top: 11, color: 'var(--text-faint)' }} />
              <input
                className="input"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder={tv.searchPlaceholder}
                style={{ paddingLeft: 34 }}
              />
            </div>
          </aside>

          <div style={{ overflowY: 'auto' }}>
            <div className="table-row table-head" style={{ gridTemplateColumns: '1fr 64px 70px' }}>
              <div>Lead</div><div>SCORE</div><div></div>
            </div>
            {isLoading ? (
              <div style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 10 }}>
                {[0, 1, 2, 3, 4].map((item) => (
                  <div key={item} className="skeleton" style={{ height: 56 }} />
                ))}
              </div>
            ) : error ? (
              <div className="empty" style={{ margin: 16 }}>
                <div className="eyebrow"><span className="dot" />{t.common.error}</div>
                <h3>{tv.errorTitle}</h3>
                <p>{error.message}</p>
              </div>
            ) : filteredLeads.length === 0 ? (
              <div className="empty" style={{ marginTop: 40 }}>
                <div className="eyebrow"><span className="dot" />KHÔNG CÓ LEAD</div>
                <h3>Không có gì khớp bộ lọc</h3>
                <p>Điều chỉnh bộ lọc hoặc chạy đợt crawl tiếp theo.</p>
              </div>
            ) : (
              filteredLeads.map((lead) => {
                const intent = intentDisplay(lead.agent);
                return (
                  <div
                    key={lead.id}
                    className={`table-row ${selectedId === lead.id ? 'is-active' : ''}`}
                    style={{ gridTemplateColumns: '1fr 64px 70px', cursor: 'pointer' }}
                    onClick={() => setSelectedId(lead.id)}
                  >
                    <div style={{ minWidth: 0 }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                        <span style={{ fontSize: 13.5, color: 'var(--text)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                          {lead.name}
                        </span>
                        <span className={intent.className} style={{ fontSize: 10, padding: '1px 6px' }}>{intent.label}</span>
                      </div>
                      <div className="mono" style={{ fontSize: 11, color: 'var(--text-faint)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', marginTop: 2 }}>
                        {lead.group || tv.unknownSource}
                      </div>
                    </div>
                    <div className="tabular mono" style={{ fontSize: 13.5 }}>{lead.score}</div>
                    <div><span className={statusTagClass(lead.status)}>{lead.status.toUpperCase()}</span></div>
                  </div>
                );
              })
            )}
          </div>

          <div style={{ padding: 24, overflowY: 'auto' }}>
            {selectedLead ? (
              <div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                  <span className="avatar avatar-lg">{(selectedLead.name.trim()[0] || 'L').toUpperCase()}</span>
                  <div>
                    <h3 style={{ fontSize: 18 }}>{selectedLead.name}</h3>
                    <div className="mono" style={{ fontSize: 11, color: 'var(--text-faint)' }}>{selectedLead.group || tv.unknownGroup}</div>
                  </div>
                  <div style={{ flex: 1 }} />
                  <span className={intentDisplay(selectedLead.agent).className}>{intentDisplay(selectedLead.agent).label}</span>
                  <span className={statusTagClass(selectedLead.status)}>{selectedLead.status.toUpperCase()}</span>
                </div>

                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 1, marginTop: 24, background: 'var(--line)', border: '1px solid var(--line)', borderRadius: 8, overflow: 'hidden' }}>
                  <div className="stat" style={{ background: 'var(--bg-elev)' }}>
                    <div className="stat-label">SCORE</div>
                    <div className="stat-value tabular">{selectedLead.score}</div>
                  </div>
                  <div className="stat" style={{ background: 'var(--bg-elev)' }}>
                    <div className="stat-label">LAST SEEN</div>
                    <div className="mono" style={{ fontSize: 14, color: 'var(--text)', marginTop: 6 }}>{selectedLead.last}</div>
                  </div>
                </div>

                <div className="sidebar-section" style={{ marginTop: 20, paddingLeft: 0 }}>AI PHÂN TÍCH</div>
                <p style={{ fontSize: 13.5, color: 'var(--text)', lineHeight: 1.55 }}>
                  {selectedLead.phone || tv.noteEmpty}
                </p>

                <div className="sidebar-section" style={{ marginTop: 20, paddingLeft: 0 }}>CHI TIẾT</div>
                <dl style={{ display: 'grid', gridTemplateColumns: 'auto 1fr', gap: '8px 16px', fontSize: 13, margin: 0 }}>
                  <dt style={{ color: 'var(--text-faint)' }}>Tệp</dt>
                  <dd className="mono" style={{ color: 'var(--text)', margin: 0 }}>{intentDisplay(selectedLead.agent).label}</dd>
                  <dt style={{ color: 'var(--text-faint)' }}>Ngành / nguồn</dt>
                  <dd className="mono" style={{ color: 'var(--text)', margin: 0 }}>{selectedLead.group || '—'}</dd>
                  <dt style={{ color: 'var(--text-faint)' }}>Lần cuối</dt>
                  <dd style={{ color: 'var(--text-mute)', margin: 0 }}>{selectedLead.last}</dd>
                </dl>

                <div style={{ display: 'flex', gap: 8, marginTop: 24, flexWrap: 'wrap' }}>
                  {selectedLead.postUrl && (
                    <a className="btn btn-primary btn-sm" href={selectedLead.postUrl} target="_blank" rel="noopener noreferrer">
                      <ExternalLink size={13} style={{ marginRight: 6 }} />
                      Mở bài viết
                    </a>
                  )}
                  {selectedLead.facebookUrl && (
                    <a className="btn btn-ghost btn-sm" href={selectedLead.facebookUrl} target="_blank" rel="noopener noreferrer">
                      <ExternalLink size={13} style={{ marginRight: 6 }} />
                      Mở profile
                    </a>
                  )}
                  <button type="button" className="btn btn-ghost btn-sm" onClick={() => void refetch()}>
                    <RefreshCw size={13} style={{ marginRight: 6 }} />
                    Đồng bộ
                  </button>
                  <button
                    type="button"
                    className="btn btn-ghost btn-sm"
                    style={{ marginLeft: 'auto', color: 'var(--danger, #c0392b)' }}
                    disabled={deletingId === selectedLead.id}
                    onClick={() => void handleDelete(selectedLead)}
                  >
                    <Trash2 size={13} style={{ marginRight: 6 }} />
                    {deletingId === selectedLead.id ? 'Đang xoá…' : 'Xoá lead'}
                  </button>
                </div>
              </div>
            ) : (
              <div className="empty" style={{ marginTop: 40 }}>
                <div className="eyebrow"><span className="dot" />KHÔNG CÓ LEAD</div>
                <h3>Không có gì khớp bộ lọc</h3>
                <p>Điều chỉnh bộ lọc hoặc chạy đợt crawl tiếp theo.</p>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
