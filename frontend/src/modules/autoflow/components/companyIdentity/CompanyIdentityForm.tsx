'use client';

import { useEffect, useState } from 'react';
import { AlertTriangle, CheckCircle2, Sparkles } from 'lucide-react';
import { getCompanyIdentity, saveCompanyIdentity } from '../../services/companyIdentityService';
import { EMPTY_COMPANY_IDENTITY, type CompanyIdentity } from './types';
import { normalizeWebsite, validateContact, validateCta, validateWebsite } from './validation';

interface Props { isAdmin: boolean; }

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 5 }}>
      <label style={{ fontSize: 12.5, fontWeight: 600, color: 'var(--text)' }}>{label}</label>
      {children}
      {hint && <span style={{ fontSize: 11.5, color: 'var(--text-faint)' }}>{hint}</span>}
    </div>
  );
}

export function CompanyIdentityForm({ isAdmin }: Props) {
  const [form, setForm] = useState<CompanyIdentity>(EMPTY_COMPANY_IDENTITY);
  const [saving, setSaving] = useState(false);
  const [msg, setMsg] = useState<{ tone: 'ok' | 'hot'; text: string } | null>(null);

  useEffect(() => { getCompanyIdentity().then(setForm).catch(() => {}); }, []);
  const set = (k: keyof CompanyIdentity) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
    setForm(f => ({ ...f, [k]: e.target.value }));

  const save = async () => {
    const err = validateWebsite(form.website) || validateContact(form.official_contact) || validateCta(form.primary_cta);
    if (err) { setMsg({ tone: 'hot', text: err }); return; }
    setSaving(true);
    setMsg(null);
    try {
      const payload = { ...form, website: normalizeWebsite(form.website) };
      setForm(await saveCompanyIdentity(payload));
      setMsg({ tone: 'ok', text: 'Đã lưu danh tính công ty.' });
    } catch (e) {
      setMsg({ tone: 'hot', text: e instanceof Error ? e.message : 'Không lưu được — thử lại.' });
    } finally {
      setSaving(false);
    }
  };

  const preview = [form.company_name || '—', form.website, form.primary_cta].filter(Boolean).join(' · ');

  return (
    <div className="card" style={{ padding: 'var(--s-5)', display: 'flex', flexDirection: 'column', gap: 16 }}>
      <div>
        <h3 style={{ fontSize: 16, fontWeight: 700, margin: 0 }}>Danh tính công ty</h3>
        <p style={{ fontSize: 12.5, color: 'var(--text-mute)', marginTop: 4 }}>
          Thông tin này giúp agent giới thiệu đúng thương hiệu và không tự bịa website/contact. Để trống field nào thì agent sẽ không nêu field đó.
        </p>
      </div>

      <Field label="Tên công ty"><input className="input" disabled={!isAdmin} value={form.company_name} onChange={set('company_name')} placeholder="THG Fulfill" /></Field>
      <Field label="Website chính thức" hint="Để trống nếu chưa có — agent sẽ không nêu website."><input className="input" disabled={!isAdmin} value={form.website} onChange={set('website')} placeholder="thgfulfill.com" /></Field>
      <Field label="Liên hệ chính thức" hint="Telegram / Zalo / email / SĐT. Để trống thì agent không nêu liên hệ."><input className="input" disabled={!isAdmin} value={form.official_contact} onChange={set('official_contact')} placeholder="t.me/thgfulfill" /></Field>
      <Field label="CTA chính" hint="Lời mời hành động ngắn gọn."><input className="input" disabled={!isAdmin} value={form.primary_cta} onChange={set('primary_cta')} placeholder="Inbox mình để bên em khảo sát mẫu và gửi phương án phù hợp." /></Field>
      <Field label="Mô tả ngắn dịch vụ" hint="Giúp agent hiểu business — không phải để copy y nguyên vào comment.">
        <textarea className="input" disabled={!isAdmin} rows={3} style={{ resize: 'vertical' }} value={form.service_summary} onChange={set('service_summary')}
          placeholder="THG Fulfill hỗ trợ sourcing VN/CN, gom hàng, kho US và fulfillment cho seller TikTok Shop." />
      </Field>

      <div style={{ display: 'flex', alignItems: 'flex-start', gap: 8, background: 'var(--bg-elev-2)', borderRadius: 8, padding: '10px 12px' }}>
        <Sparkles size={14} color="var(--text-mute)" style={{ flexShrink: 0, marginTop: 2 }} />
        <span style={{ fontSize: 12, color: 'var(--text-mute)' }}>Agent có thể dùng: <strong style={{ color: 'var(--text)' }}>{preview}</strong></span>
      </div>

      {msg && (
        <div className={`banner banner-${msg.tone === 'ok' ? 'ok' : 'hot'}`} style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          {msg.tone === 'ok' ? <CheckCircle2 size={15} color="var(--ok)" /> : <AlertTriangle size={15} color="var(--hot)" />}
          <span style={{ fontSize: 12.5 }}>{msg.text}</span>
        </div>
      )}

      {isAdmin && (
        <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
          <button type="button" className="btn btn-primary btn-sm" onClick={() => void save()} disabled={saving}>
            {saving ? 'Đang lưu...' : 'Lưu danh tính công ty'}
          </button>
        </div>
      )}
    </div>
  );
}
