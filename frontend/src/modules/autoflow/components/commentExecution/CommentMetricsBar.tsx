'use client';

import { useEffect, useState } from 'react';
import { getCommentMetrics, type CommentMetricsResponse } from '../../services/outboxService';

// Comment outcome metrics bar (spec: specs/COMMENT_ASYNC_REVERIFY.md companion, Part C).
// Self-contained: fetches the 7-day summary and shows the submitted_unverified rate against
// the decision threshold (<10% = edge case + manual fallback; >10–15% sustained = reopen
// async reverify as a core bug). Best-effort — hidden if the call fails or there's no data.

export function CommentMetricsBar() {
  const [data, setData] = useState<CommentMetricsResponse | null>(null);

  useEffect(() => {
    let cancelled = false;
    getCommentMetrics(7)
      .then((r) => { if (!cancelled) setData(r); })
      .catch(() => { /* best-effort */ });
    return () => { cancelled = true; };
  }, []);

  if (!data || data.metrics.total <= 0) return null;
  const m = data.metrics;
  const ratePct = Math.round(data.submitted_unverified_rate * 1000) / 10;
  const hot = data.submitted_unverified_rate > 0.1;

  const item = (label: string, value: number | string, color?: string) => (
    <span style={{ display: 'inline-flex', gap: 4, alignItems: 'baseline' }}>
      <span style={{ color: 'var(--text-faint)', fontSize: 11 }}>{label}</span>
      <strong style={{ color: color ?? 'var(--text)' }}>{value}</strong>
    </span>
  );

  return (
    <div
      className="card"
      style={{ display: 'flex', flexWrap: 'wrap', gap: 16, padding: '10px 14px', marginBottom: 12, fontSize: 12 }}
      title="Tỉ lệ comment đã gửi nhưng chưa xác minh (7 ngày). >10% kéo dài → mở lại async reverify."
    >
      {item('Comment (7d)', m.total)}
      {item('Đã xác minh', data.effective_verified, 'var(--ok, #16a34a)')}
      {item('Chờ xác minh', data.submitted_unverified_open, hot ? 'var(--hot)' : undefined)}
      {item('Tỉ lệ chưa xác minh', `${ratePct}%`, hot ? 'var(--hot)' : 'var(--text-mute)')}
      {item('Xác nhận thủ công', m.human_verified)}
      {m.reverified > 0 && item('Reverify', m.reverified)}
      {m.comment_button_not_found > 0 && item('Thiếu nút bình luận', m.comment_button_not_found)}
      {hot && <span style={{ color: 'var(--hot)', fontSize: 11 }}>⚠ Tỉ lệ cao — cân nhắc mở lại async reverify</span>}
    </div>
  );
}
