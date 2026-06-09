// Pure validation/normalization for the Company Identity form. Self-contained (no
// imports) so it is unit-testable in isolation. Lenient by design — every field is
// optional; only a clearly-malformed website or email is rejected.

// normalizeWebsite trims and, when a bare domain is entered, prepends https://.
// Returns '' for empty input.
export function normalizeWebsite(raw: string): string {
  const s = (raw || '').trim();
  if (!s) return '';
  if (/^https?:\/\//i.test(s)) return s;
  return `https://${s}`;
}

// validateWebsite returns an error message, or null when valid (or empty).
export function validateWebsite(raw: string): string | null {
  const s = (raw || '').trim();
  if (!s) return null;
  try {
    const u = new URL(normalizeWebsite(s));
    if (!u.hostname.includes('.')) return 'Website không hợp lệ — nhập dạng company.vn hoặc https://company.vn';
    return null;
  } catch {
    return 'Website không hợp lệ — nhập dạng company.vn hoặc https://company.vn';
  }
}

// validateContact is lenient: Telegram/Zalo/email/phone/plain text all pass. Only a
// string that clearly tries to be an email but is malformed is rejected.
export function validateContact(raw: string): string | null {
  const s = (raw || '').trim();
  if (!s) return null;
  const looksLikeLoneEmail = s.includes('@') && !s.includes(' ');
  if (looksLikeLoneEmail && !/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(s)) {
    return 'Email không hợp lệ.';
  }
  return null;
}

// validateCta keeps the CTA short and link-light (it must not become a spam blob).
export function validateCta(raw: string): string | null {
  const s = (raw || '').trim();
  if (!s) return null;
  if (s.length > 200) return 'CTA quá dài — giữ ngắn gọn.';
  const urlCount = (s.match(/https?:\/\//gi) || []).length;
  if (urlCount > 1) return 'CTA chỉ nên có tối đa một liên kết.';
  return null;
}
