import { Brain, Save } from 'lucide-react';
import { Row } from '../ui';
import { cardStyle, primaryBtn, theme } from '../../constants/styles';

interface BusinessMemoryPanelProps {
  value: string;
  message: string;
  isSaving: boolean;
  onChange: (value: string) => void;
  onSave: () => void;
}

export default function BusinessMemoryPanel({ value, message, isSaving, onChange, onSave }: BusinessMemoryPanelProps) {
  return (
    <div style={cardStyle()}>
      <Row style={{ gap: 9, marginBottom: 12 }}>
        <Brain size={16} color={theme.primaryLight} />
        <p style={{ color: theme.text, fontWeight: 600, fontSize: 13 }}>Business memory</p>
      </Row>
      <textarea
        value={value}
        onChange={e => onChange(e.target.value)}
        placeholder="Mô tả brand, sản phẩm/dịch vụ, khách hàng mục tiêu, cách chốt sales, tone, chính sách từ chối, nguồn dữ liệu ưu tiên..."
        style={{
          width: '100%',
          minHeight: 120,
          resize: 'vertical',
          boxSizing: 'border-box',
          background: theme.border,
          border: '1px solid #374151',
          borderRadius: 10,
          padding: 12,
          color: '#fff',
          fontSize: 13,
          lineHeight: 1.55,
          outline: 'none',
        }}
      />
      <Row style={{ justifyContent: 'space-between', gap: 10, marginTop: 10 }}>
        <p style={{ color: theme.textFaint, fontSize: 12 }}>Context này được đưa vào AI prompt theo org hiện tại.</p>
        <Row style={{ gap: 10 }}>
          {message && <span style={{ color: message.startsWith('Đã') ? '#4ade80' : '#fca5a5', fontSize: 12 }}>{message}</span>}
          <button disabled={isSaving} onClick={onSave} style={primaryBtn({ padding: '8px 14px', fontSize: 12, display: 'flex', alignItems: 'center', gap: 6, opacity: isSaving ? 0.6 : 1 })}>
            <Save size={13} /> Lưu memory
          </button>
        </Row>
      </Row>
    </div>
  );
}
