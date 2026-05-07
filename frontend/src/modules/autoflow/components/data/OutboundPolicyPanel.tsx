import { ShieldCheck, Zap } from 'lucide-react';
import { Row } from '../ui';
import { cardStyle, theme } from '../../constants/styles';
import type { OutboundMode } from '../../services/settingsService';

interface OutboundPolicyPanelProps {
  mode: OutboundMode;
  message: string;
  isSaving: boolean;
  isAdmin: boolean;
  onChange: (next: OutboundMode) => void | Promise<void>;
}

export default function OutboundPolicyPanel({ mode, message, isSaving, isAdmin, onChange }: OutboundPolicyPanelProps) {
  const isAuto = mode === 'auto';
  const disabled = isSaving || !isAdmin;

  const optionStyle = (active: boolean): React.CSSProperties => ({
    flex: 1,
    padding: '12px 14px',
    borderRadius: 10,
    border: `1px solid ${active ? theme.primaryLight : theme.border}`,
    background: active ? 'rgba(96, 165, 250, 0.08)' : 'transparent',
    cursor: disabled ? 'not-allowed' : 'pointer',
    opacity: disabled && !active ? 0.6 : 1,
    display: 'flex',
    flexDirection: 'column',
    gap: 4,
    textAlign: 'left',
    color: theme.text,
  });

  return (
    <div style={cardStyle()}>
      <Row style={{ gap: 9, marginBottom: 10 }}>
        <ShieldCheck size={16} color={theme.primaryLight} />
        <p style={{ color: theme.text, fontWeight: 600, fontSize: 13 }}>Chính sách outbound</p>
      </Row>
      <p style={{ color: theme.textFaint, fontSize: 12, marginBottom: 12 }}>
        Quyết định comment / inbox / post AI sinh ra sẽ chạy ngay hay phải chờ admin duyệt trên Dashboard.
      </p>

      <div style={{ display: 'flex', gap: 10, marginBottom: 10 }}>
        <button
          type="button"
          disabled={disabled}
          onClick={() => { if (!isAuto) void onChange('draft'); }}
          style={optionStyle(!isAuto)}
        >
          <Row style={{ gap: 7, marginBottom: 2 }}>
            <ShieldCheck size={13} color={!isAuto ? theme.primaryLight : theme.textFaint} />
            <span style={{ fontSize: 13, fontWeight: 600 }}>Cần duyệt (draft)</span>
          </Row>
          <span style={{ fontSize: 11.5, color: theme.textFaint, lineHeight: 1.5 }}>
            Outbound vào hàng chờ. Admin xem trước, duyệt rồi extension mới gửi đi. An toàn, mặc định khi mới khởi tạo workspace.
          </span>
        </button>
        <button
          type="button"
          disabled={disabled}
          onClick={() => { if (isAuto) return; void onChange('auto'); }}
          style={optionStyle(isAuto)}
        >
          <Row style={{ gap: 7, marginBottom: 2 }}>
            <Zap size={13} color={isAuto ? theme.primaryLight : theme.textFaint} />
            <span style={{ fontSize: 13, fontWeight: 600 }}>Tự động chạy (auto)</span>
          </Row>
          <span style={{ fontSize: 11.5, color: theme.textFaint, lineHeight: 1.5 }}>
            Bỏ hàng chờ. Mỗi comment / inbox / post AI sinh ra được duyệt sẵn và đẩy thẳng xuống Chrome Extension. Chỉ bật khi đã tin định vị + classifier.
          </span>
        </button>
      </div>

      <Row style={{ justifyContent: 'space-between', gap: 10 }}>
        <p style={{ color: theme.textFaint, fontSize: 11.5 }}>
          {isAdmin ? 'Bạn là admin — có quyền đổi chế độ này.' : 'Chỉ admin của workspace mới đổi được chính sách này.'}
        </p>
        {message && (
          <span style={{ color: message.startsWith('Đã') ? '#4ade80' : '#fca5a5', fontSize: 12 }}>{message}</span>
        )}
      </Row>
    </div>
  );
}
