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

function filterLabel(filter: LeadStatus | 'All', lang: 'vi' | 'en') {
  if (filter !== 'All') return filter;
  return lang === 'vi' ? 'Tất cả' : 'All';
}

function leadSearchValue(lead: Lead) {
  return [lead.name, lead.group, lead.agent, lead.phone, lead.facebookUrl ?? '']
    .join(' ')
    .toLowerCase();
}

export default function LeadsView({ orgId, isAdmin }: LeadsViewProps) {
  void isAdmin;
  const { lang, t } = useLang();
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
      <div style={{ display: 'flex', alignItems: 'flex-start', gap: 16, flexWrap: 'wrap' }}>
        <div>
          <div className="eyebrow">
            <span className="dot" />
            SALES
          </div>
          <h2 style={{ fontSize: 28, marginTop: 8 }}>{t.views.leadsTitle}</h2>
          <p style={{ color: 'var(--text-mute)', fontSize: 13.5, marginTop: 6 }}>{t.views.leadsSub}</p>
        </div>
        <div style={{ flex: 1 }} />
        <button className="btn btn-ghost btn-sm" type="button" onClick={() => void refetch()}>
          <RefreshCw size={13} />
          {t.common.refresh}
        </button>
      </div>

      <div className="stats-grid">
        <div className="stat">
          <div className="stat-label">{lang === 'vi' ? 'TỔNG' : 'TOTAL'}</div>
          <div className="stat-value tabular">{totals.all.toLocaleString(lang === 'vi' ? 'vi-VN' : 'en-US')}</div>
        </div>
        <div className="stat">
          <div className="stat-label">HOT</div>
          <div className="stat-value tabular" style={{ color: 'var(--hot)' }}>{totals.hot}</div>
        </div>
        <div className="stat">
          <div className="stat-label">WARM</div>
          <div className="stat-value tabular" style={{ color: 'var(--warn)' }}>{totals.warm}</div>
        </div>
        <div className="stat">
          <div className="stat-label">{lang === 'vi' ? 'ĐIỂM TB' : 'AVG SCORE'}</div>
          <div className="stat-value tabular">{totals.avgScore}</div>
        </div>
      </div>

      <div className="card" style={{ padding: 0, overflow: 'hidden', minHeight: 560 }}>
        <div className="three-pane" style={{ minHeight: 560 }}>
          <div style={{ padding: 16 }}>
            <div className="sidebar-section">{lang === 'vi' ? 'BỘ LỌC' : 'FILTERS'}</div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              {FILTERS.map((item) => {
                const count = item === 'All' ? totals.all : leads.filter((lead) => lead.status === item).length;
                return (
                  <button
                    key={item}
                    type="button"
                    className={`nav-item ${filter === item ? 'is-active' : ''}`}
                    style={{ width: '100%', background: 'transparent', border: 0, textAlign: 'left' }}
                    onClick={() => setFilter(item)}
                  >
                    <span>{filterLabel(item, lang)}</span>
                    <span className="badge-num badge">{count}</span>
                  </button>
                );
              })}
            </div>

            <div style={{ marginTop: 18 }}>
              <div className="sidebar-section" style={{ paddingLeft: 0 }}>
                {lang === 'vi' ? 'TÌM NHANH' : 'SEARCH'}
              </div>
              <div style={{ position: 'relative' }}>
                <Search size={13} style={{ position: 'absolute', left: 12, top: 11, color: 'var(--text-faint)' }} />
                <input
                  className="input"
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  placeholder={lang === 'vi' ? 'Tên, nhóm, role...' : 'Name, group, role...'}
                  style={{ paddingLeft: 34 }}
                />
              </div>
            </div>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column' }}>
            <div style={{ padding: 16, borderBottom: '1px solid var(--line)' }}>
              <div className="eyebrow">{lang === 'vi' ? 'DANH SÁCH LEAD' : 'LEAD LIST'}</div>
              <div style={{ marginTop: 6, fontSize: 13, color: 'var(--text-mute)' }}>
                {lang === 'vi'
                  ? `${filteredLeads.length} lead phù hợp với bộ lọc hiện tại`
                  : `${filteredLeads.length} leads match the current filter`}
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
                    ERROR
                  </div>
                  <h3>{lang === 'vi' ? 'Không tải được leads' : 'Could not load leads'}</h3>
                  <p>{error.message}</p>
                </div>
              ) : filteredLeads.length === 0 ? (
                <div className="empty" style={{ margin: 16 }}>
                  <div className="eyebrow">
                    <span className="dot" />
                    EMPTY
                  </div>
                  <h3>{lang === 'vi' ? 'Chưa có lead' : 'No leads yet'}</h3>
                  <p>
                    {lang === 'vi'
                      ? 'Crawler sẽ đổ lead vào đây ngay khi business profile được calibrate xong.'
                      : 'Crawler will populate this list once your business profile is calibrated.'}
                  </p>
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
                          {lead.group || (lang === 'vi' ? 'Không rõ nguồn' : 'Unknown source')}
                        </div>
                      </div>
                      <span className={statusTagClass(lead.status)}>{lead.status}</span>
                    </div>
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, paddingLeft: 32 }}>
                      <span className="mono" style={{ fontSize: 11, color: 'var(--text-mute)' }}>
                        {lead.agent || 'AI classifier'}
                      </span>
                      <span className="mono tabular" style={{ fontSize: 11, color: 'var(--text-faint)' }}>
                        {lang === 'vi' ? 'Điểm' : 'Score'} {lead.score}
                      </span>
                    </div>
                  </button>
                ))
              )}
            </div>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column' }}>
            {selectedLead ? (
              <>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: 16, borderBottom: '1px solid var(--line)' }}>
                  <span className="avatar avatar-lg">{(selectedLead.name.trim()[0] || 'L').toUpperCase()}</span>
                  <div style={{ minWidth: 0, flex: 1 }}>
                    <div style={{ fontSize: 16, color: 'var(--text)', fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {selectedLead.name}
                    </div>
                    <div className="mono" style={{ fontSize: 11, color: 'var(--text-faint)', marginTop: 4 }}>
                      {selectedLead.group || (lang === 'vi' ? 'Không rõ nhóm' : 'Unknown group')}
                    </div>
                  </div>
                  <span className={statusTagClass(selectedLead.status)}>{selectedLead.status}</span>
                </div>

                <div style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 16 }}>
                  <div className="stats-grid" style={{ gridTemplateColumns: 'repeat(2, 1fr)' }}>
                    <div className="stat">
                      <div className="stat-label">{lang === 'vi' ? 'ĐIỂM' : 'SCORE'}</div>
                      <div className="stat-value tabular" style={{ fontSize: 22 }}>{selectedLead.score}</div>
                    </div>
                    <div className="stat">
                      <div className="stat-label">{lang === 'vi' ? 'CẬP NHẬT' : 'LAST SEEN'}</div>
                      <div className="stat-value mono" style={{ fontSize: 16 }}>{selectedLead.last}</div>
                    </div>
                  </div>

                  <div className="card" style={{ padding: 16 }}>
                    <div className="eyebrow" style={{ marginBottom: 10 }}>{lang === 'vi' ? 'NGỮ CẢNH LEAD' : 'LEAD CONTEXT'}</div>
                    <dl style={{ display: 'grid', gap: 10 }}>
                      <div style={{ display: 'grid', gap: 4 }}>
                        <dt className="field-label">{lang === 'vi' ? 'Nguồn / nhóm' : 'Source / group'}</dt>
                        <dd style={{ margin: 0, color: 'var(--text)' }}>{selectedLead.group || '-'}</dd>
                      </div>
                      <div style={{ display: 'grid', gap: 4 }}>
                        <dt className="field-label">{lang === 'vi' ? 'Classifier' : 'Classifier'}</dt>
                        <dd style={{ margin: 0, color: 'var(--text)' }}>{selectedLead.agent || '-'}</dd>
                      </div>
                      <div style={{ display: 'grid', gap: 4 }}>
                        <dt className="field-label">{lang === 'vi' ? 'Tín hiệu / ghi chú' : 'Signal / note'}</dt>
                        <dd style={{ margin: 0, color: 'var(--text-mute)', lineHeight: 1.5 }}>
                          {selectedLead.phone || (lang === 'vi' ? 'Chưa có ghi chú bổ sung cho lead này.' : 'No extra note has been stored for this lead yet.')}
                        </dd>
                      </div>
                    </dl>
                  </div>

                  <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                    {selectedLead.facebookUrl && (
                      <a className="btn btn-primary btn-sm" href={selectedLead.facebookUrl} target="_blank" rel="noopener noreferrer">
                        <ExternalLink size={13} />
                        {lang === 'vi' ? 'Mở Facebook' : 'Open Facebook'}
                      </a>
                    )}
                    <button type="button" className="btn btn-ghost btn-sm" onClick={() => void refetch()}>
                      <RefreshCw size={13} />
                      {lang === 'vi' ? 'Đồng bộ lại' : 'Sync again'}
                    </button>
                  </div>
                </div>
              </>
            ) : (
              <div className="empty" style={{ margin: 16 }}>
                <div className="eyebrow">
                  <span className="dot" />
                  SELECT
                </div>
                <h3>{lang === 'vi' ? 'Chọn một lead' : 'Select a lead'}</h3>
                <p>
                  {lang === 'vi'
                    ? 'Chọn lead bên trái để xem chi tiết market signal và action tiếp theo.'
                    : 'Pick a lead from the list to inspect the market signal and next action.'}
                </p>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
