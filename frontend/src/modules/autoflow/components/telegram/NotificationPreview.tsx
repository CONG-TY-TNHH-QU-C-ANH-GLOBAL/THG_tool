import React from 'react';
import { theme, cardStyle } from '../../constants/styles';
import { strings, type Lang } from './telegramCopy';

// Static preview of what THG sends into a channel, so admins understand the output. Render-only;
// vi-primary sample copy. No real data, no chat_id, no token.
const SAMPLES = [
  '📌 Lead mới từ Facebook\nWorkspace: THG Fulfill\nNguồn: Facebook group\n' +
    'Lead: Anonymous participant\nTóm tắt: đang tìm supplier cho mẫu đèn...\n' +
    'Trạng thái: sẵn sàng xử lý\nMở dashboard: <link>',
  '✅ Agent đã gửi comment\nChannel: Facebook\nAccount: David Anh\nLead: Anonymous participant\n' +
    'Trạng thái: đã gửi / chờ xác minh\nMở post: <link>\nMở dashboard: <link>',
  '⚠️ Cần kiểm tra\nFacebook David Anh gặp lỗi khi comment.\nLý do: submitted_unverified\nMở dashboard: <link>',
];

export const NotificationPreview = React.memo(({ lang }: { lang: Lang }) => {
  const { t } = strings(lang);
  return (
    <div style={cardStyle({ display: 'grid', gap: 10 })}>
      <p style={{ color: theme.text, fontSize: 14, fontWeight: 650, margin: 0 }}>{t('preview_title')}</p>
      <div style={{ display: 'grid', gap: 10 }}>
        {SAMPLES.map((s, i) => (
          <pre key={i} style={{
            margin: 0, whiteSpace: 'pre-wrap', fontFamily: 'var(--font-mono)', fontSize: 12,
            color: theme.textMuted, background: 'var(--bg-elev-2)', border: `1px solid ${theme.border}`,
            borderRadius: 'var(--radius-md)', padding: '10px 12px',
          }}>{s}</pre>
        ))}
      </div>
    </div>
  );
});
NotificationPreview.displayName = 'NotificationPreview';
