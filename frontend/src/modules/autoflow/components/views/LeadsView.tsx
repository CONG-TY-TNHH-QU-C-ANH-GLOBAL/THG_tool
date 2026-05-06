'use client';

import { useEffect, useMemo, useState } from 'react';
import { ExternalLink, RefreshCw, Search } from 'lucide-react';
import type { Lead, LeadStatus } from '../../types';
import { useLeads } from '../../hooks/useLeads';
import { useLang } from '../../i18n/useLang';

interface LeadsViewProps {
  orgId: string;
  isAdmin: boolean;
}

const FILTERS: Array<LeadStatus | 'All'> = ['All', 'Hot', 'Warm', 'Cold'];

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
  return [lead.name, lead.group, lead.agent, lead.phone, lead.facebookUrl ?? '']
    .join(' ')
    .toLowerCase();
}

export default function LeadsView({ orgId, isAdmin }: LeadsViewProps) {
  void isAdmin;
  const { lang, t } = useLang();
  const tv = t.leadsView;
  const locale = lang === 'vi' ? 'vi-VN' : 'en-US';
  const [filter, setFilter] = useState<LeadStatus | 'All'>('All');
  const [query, setQuery] = useState('');
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const { leads, isLoading, error, refetch } = useLeads(orgId, filter);

  const filteredLeads = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    if (!normalized) return leads;
    return leads.filter((lead) => leadSearchValue(lead).includes(normalized));
  }, [leads, query]);

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
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              {FILTERS.map((item) => {
                const count = item === 'All' ? totals.all : leads.filter((lead) => lead.status === item).length;
                const label = item === 'All' ? tv.filterAll : item;
                return (
                  <button
                    key={item}
                    type="button"
                    className={`nav-item ${filter === item ? 'is-active' : ''}`}
                    style={{ width: '100%', background: 'transparent', border: 0, textAlign: 'left' }}
                    onClick={() => setFilter(item)}
                  >
                    <span>{label}</span>
                    <span className="badge-num badge">{count}</span>
                  </button>
                );
              })}
            </div>

            <div style={{ marginTop: 18 }}>
              <div className="sidebar-section" style={{ paddingLeft: 0 }}>{tv.searchLabel}</div>
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
            </div>
          </aside>

          <section style={{ display: 'flex', flexDirection: 'column' }}>
            <div style={{ padding: 16, borderBottom: '1px solid var(--line)' }}>
              <div className="eyebrow">{tv.listTitle}</div>
              <div style={{ marginTop: 6, fontSize: 13, color: 'var(--text-mute)' }}>
                {tv.listCount(filteredLeads.length)}
              </div>
            </div>

            <div style={{ flex: 1, overflowY: 'auto' }}>
              {isLoading ? (
                <div style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 10 }}>
                  {[0, 1, 2, 3, 4].map((item) => (
                    <div key={item} className="skeleton" style={{ height: 56 }} />
                  ))}
                </div>
              ) : error ? (
                <div className="empty" style={{ margin: 16 }}>
                  <div className="eyebrow">
                    <span className="dot" />
                    {t.common.error}
                  </div>
                  <h3>{tv.errorTitle}</h3>
                  <p>{error.message}</p>
                </div>
              ) : filteredLeads.length === 0 ? (
                <div className="empty" style={{ margin: 16 }}>
                  <div className="eyebrow">
                    <span className="dot" />
                    {t.common.empty}
                  </div>
                  <h3>{tv.emptyTitle}</h3>
                  <p>{tv.emptyDesc}</p>
                </div>
              ) : (
                filteredLeads.map((lead) => (
                  <button
                    key={lead.id}
                    type="button"
                    onClick={() => setSelectedId(lead.id)}
                    className={`nav-item ${selectedId === lead.id ? 'is-active' : ''}`}
                    style={{
                      width: '100%',
                      background: 'transparent',
                      border: 0,
                      borderBottom: '1px solid var(--line)',
                      borderRadius: 0,
                      padding: 14,
                      alignItems: 'stretch',
                      flexDirection: 'column',
                      gap: 8,
                      textAlign: 'left',
                    }}
                  >
                    <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                      <span className="avatar avatar-sm">{(lead.name.trim()[0] || 'L').toUpperCase()}</span>
                      <div style={{ minWidth: 0, flex: 1 }}>
                        <div style={{ fontSize: 13.5, color: 'var(--text)', fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                          {lead.name}
                        </div>
                        <div className="mono" style={{ fontSize: 11, color: 'var(--text-faint)', marginTop: 2, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                          {lead.group || tv.unknownSource}
                        </div>
                      </div>
                      <span className={statusTagClass(lead.status)}>{lead.status}</span>
                    </div>
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, paddingLeft: 32 }}>
                      <span className="mono" style={{ fontSize: 11, color: 'var(--text-mute)' }}>
                        {lead.agent || tv.defaultClassifier}
                      </span>
                      <span className="mono tabular" style={{ fontSize: 11, color: 'var(--text-faint)' }}>
                        {tv.statScore} {lead.score}
                      </span>
                    </div>
                  </button>
                ))
              )}
            </div>
          </section>

          <section style={{ display: 'flex', flexDirection: 'column' }}>
            {selectedLead ? (
              <>
                <header style={{ display: 'flex', alignItems: 'center', gap: 12, padding: 16, borderBottom: '1px solid var(--line)' }}>
                  <span className="avatar avatar-lg">{(selectedLead.name.trim()[0] || 'L').toUpperCase()}</span>
                  <div style={{ minWidth: 0, flex: 1 }}>
                    <div style={{ fontSize: 16, color: 'var(--text)', fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {selectedLead.name}
                    </div>
                    <div className="mono" style={{ fontSize: 11, color: 'var(--text-faint)', marginTop: 4 }}>
                      {selectedLead.group || tv.unknownGroup}
                    </div>
                  </div>
                  <span className={statusTagClass(selectedLead.status)}>{selectedLead.status}</span>
                </header>

                <div style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 16 }}>
                  <div className="stats-grid" style={{ gridTemplateColumns: 'repeat(2, 1fr)' }}>
                    <div className="stat">
                      <div className="stat-label">{tv.statScore}</div>
                      <div className="stat-value tabular" style={{ fontSize: 22 }}>{selectedLead.score}</div>
                    </div>
                    <div className="stat">
                      <div className="stat-label">{tv.statLastSeen}</div>
                      <div className="stat-value mono" style={{ fontSize: 16 }}>{selectedLead.last}</div>
                    </div>
                  </div>

                  <div className="card" style={{ padding: 16 }}>
                    <div className="eyebrow" style={{ marginBottom: 10 }}>{tv.contextTitle}</div>
                    <dl style={{ display: 'grid', gap: 10 }}>
                      <div style={{ display: 'grid', gap: 4 }}>
                        <dt className="field-label">{tv.fieldSource}</dt>
                        <dd style={{ margin: 0, color: 'var(--text)' }}>{selectedLead.group || '—'}</dd>
                      </div>
                      <div style={{ display: 'grid', gap: 4 }}>
                        <dt className="field-label">{tv.fieldClassifier}</dt>
                        <dd style={{ margin: 0, color: 'var(--text)' }}>{selectedLead.agent || tv.defaultClassifier}</dd>
                      </div>
                      <div style={{ display: 'grid', gap: 4 }}>
                        <dt className="field-label">{tv.fieldNote}</dt>
                        <dd style={{ margin: 0, color: 'var(--text-mute)', lineHeight: 1.5 }}>
                          {selectedLead.phone || tv.noteEmpty}
                        </dd>
                      </div>
                    </dl>
                  </div>

                  <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                    {selectedLead.facebookUrl && (
                      <a className="btn btn-primary btn-sm" href={selectedLead.facebookUrl} target="_blank" rel="noopener noreferrer">
                        <ExternalLink size={13} />
                        {tv.openFacebook}
                      </a>
                    )}
                    <button type="button" className="btn btn-ghost btn-sm" onClick={() => void refetch()}>
                      <RefreshCw size={13} />
                      {tv.syncAgain}
                    </button>
                  </div>
                </div>
              </>
            ) : (
              <div className="empty" style={{ margin: 16 }}>
                <h3>{tv.selectTitle}</h3>
                <p>{tv.selectDesc}</p>
              </div>
            )}
          </section>
        </div>
      </div>
    </div>
  );
}
