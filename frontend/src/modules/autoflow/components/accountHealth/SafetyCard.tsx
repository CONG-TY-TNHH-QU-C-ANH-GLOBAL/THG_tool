'use client';

import { Lock, MapPin, ShieldCheck, UserCheck } from 'lucide-react';

const ITEMS: { Icon: typeof Lock; text: string }[] = [
  { Icon: Lock, text: 'THG không lưu mật khẩu Facebook.' },
  { Icon: MapPin, text: 'Chạy trên Chrome và IP của chính bạn.' },
  { Icon: UserCheck, text: 'Mỗi nhân viên tự quản lý tài khoản của mình.' },
  { Icon: ShieldCheck, text: 'Admin chỉ thấy trạng thái và kết quả hành động.' },
];

export function SafetyCard() {
  return (
    <div className="card" style={{ padding: 'var(--s-4)', display: 'flex', flexDirection: 'column', gap: 12 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <ShieldCheck size={16} color="var(--ok)" />
        <span style={{ fontSize: 14, fontWeight: 600 }}>An toàn &amp; riêng tư</span>
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
        {ITEMS.map(({ Icon, text }, i) => (
          <div key={i} style={{ display: 'flex', alignItems: 'flex-start', gap: 9, fontSize: 12.5, color: 'var(--text-mute)' }}>
            <Icon size={14} style={{ flexShrink: 0, marginTop: 2, color: 'var(--text-faint)' }} />
            <span>{text}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
