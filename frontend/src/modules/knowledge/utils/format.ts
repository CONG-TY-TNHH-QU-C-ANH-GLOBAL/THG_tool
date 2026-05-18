/**
 * Tiny pure helpers used across Knowledge Hub panels.
 *
 * formatRelativeTime returns a human-friendly delta ("3 phút trước",
 * "2 hours ago") rather than a timestamp, so operators see freshness
 * at a glance. Locale follows useLang(), passed in explicitly so this
 * module stays SSR-safe and tree-shakeable.
 */

export function formatRelativeTime(iso: string | null, lang: 'vi' | 'en'): string {
  if (!iso) return lang === 'vi' ? 'Chưa từng' : 'Never';
  const then = new Date(iso).getTime();
  const now = Date.now();
  const diffSec = Math.max(0, Math.floor((now - then) / 1000));
  if (diffSec < 60) return lang === 'vi' ? 'Vừa xong' : 'Just now';
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) {
    return lang === 'vi' ? `${diffMin} phút trước` : `${diffMin} min ago`;
  }
  const diffHr = Math.floor(diffMin / 60);
  if (diffHr < 24) {
    return lang === 'vi' ? `${diffHr} giờ trước` : `${diffHr}h ago`;
  }
  const diffDay = Math.floor(diffHr / 24);
  if (diffDay < 14) {
    return lang === 'vi' ? `${diffDay} ngày trước` : `${diffDay}d ago`;
  }
  return new Date(iso).toLocaleDateString(lang === 'vi' ? 'vi-VN' : 'en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  });
}

export function formatNumber(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n);
}

export function formatPercent(numerator: number, denominator: number): string {
  if (denominator <= 0) return '—';
  return `${Math.round((numerator / denominator) * 100)}%`;
}
