'use client';
import { useEffect, useState } from 'react';
import { Save } from 'lucide-react';
import {
  emptyContactProfile,
  getMyContactProfile,
  saveMyContactProfile,
  type StaffContactProfile,
} from '../../services/contactProfileService';

const field: React.CSSProperties = { display: 'flex', flexDirection: 'column', gap: 4 };
const labelStyle: React.CSSProperties = { fontSize: 11, color: 'var(--text-faint)' };

/**
 * Self-service contact profile (PR-5): the contact line AI comments cite
 * when a lead is handled by THIS salesperson. Empty fields are omitted —
 * never invented.
 */
export default function StaffContactProfileForm() {
  const [profile, setProfile] = useState<StaffContactProfile>(emptyContactProfile());
  const [contactLine, setContactLine] = useState('');
  const [msg, setMsg] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    getMyContactProfile()
      .then(r => { setProfile(r.profile); setContactLine(r.contact_line); })
      .catch(() => {});
  }, []);

  const set = (k: keyof StaffContactProfile) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
    setProfile(p => ({ ...p, [k]: e.target.value }));

  async function save() {
    setSaving(true);
    setMsg('');
    try {
      const r = await saveMyContactProfile(profile);
      setContactLine(r.contact_line);
      setMsg('Đã lưu. Comment AI do bạn phụ trách sẽ dùng liên hệ này.');
    } catch (e) {
      setMsg(e instanceof Error ? e.message : 'Lưu thất bại.');
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="card" style={{ padding: 14, display: 'flex', flexDirection: 'column', gap: 10 }}>
      <strong style={{ fontSize: 13 }}>Liên hệ của tôi trong comment AI</strong>
      <p style={{ fontSize: 12, color: 'var(--text-mute)', margin: 0 }}>
        Khi lead do bạn phụ trách, AI dùng liên hệ này thay cho liên hệ chung của công ty. Để trống = dùng
        liên hệ công ty (nếu workspace cho phép) hoặc bỏ qua dòng liên hệ.
      </p>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: 10 }}>
        <label style={field}><span style={labelStyle}>TÊN HIỂN THỊ</span>
          <input className="input" value={profile.display_name} onChange={set('display_name')} placeholder="Nguyễn A" /></label>
        <label style={field}><span style={labelStyle}>CHỨC DANH</span>
          <input className="input" value={profile.role_title} onChange={set('role_title')} placeholder="Sales / Fulfillment" /></label>
        <label style={field}><span style={labelStyle}>TELEGRAM</span>
          <input className="input" value={profile.telegram} onChange={set('telegram')} placeholder="@saleA" /></label>
        <label style={field}><span style={labelStyle}>ZALO</span>
          <input className="input" value={profile.zalo} onChange={set('zalo')} placeholder="09xx xxx xxx" /></label>
        <label style={field}><span style={labelStyle}>SĐT</span>
          <input className="input" value={profile.phone} onChange={set('phone')} /></label>
        <label style={field}><span style={labelStyle}>EMAIL</span>
          <input className="input" value={profile.email} onChange={set('email')} /></label>
      </div>
      <label style={field}><span style={labelStyle}>CTA ƯA THÍCH (tuỳ chọn)</span>
        <input className="input" value={profile.preferred_cta} onChange={set('preferred_cta')}
          placeholder="Inbox mình để mình khảo sát mẫu và gửi phương án phù hợp." /></label>
      {contactLine && (
        <p style={{ fontSize: 12, margin: 0 }}>
          <span style={{ color: 'var(--text-faint)' }}>Dòng liên hệ sẽ xuất hiện: </span>
          <strong>{contactLine}</strong>
        </p>
      )}
      {msg && <p style={{ fontSize: 12, color: 'var(--text-mute)', margin: 0 }}>{msg}</p>}
      <div>
        <button type="button" className="btn btn-primary btn-sm" disabled={saving} onClick={() => void save()}>
          <Save size={12} /> {saving ? 'Đang lưu…' : 'Lưu liên hệ'}
        </button>
      </div>
    </div>
  );
}
