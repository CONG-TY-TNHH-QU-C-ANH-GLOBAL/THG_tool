import { Brain, Save } from 'lucide-react';
import { Row } from '../ui';
import { cardStyle, inputStyle, primaryBtn, theme } from '../../constants/styles';
import type { BusinessContext } from '../../services/settingsService';

interface BusinessMemoryPanelProps {
  context: BusinessContext;
  message: string;
  isSaving: boolean;
  onChange: (patch: Partial<BusinessContext>) => void;
  onSave: () => void;
}

const fieldStyle = { display: 'flex', flexDirection: 'column', gap: 6 } as const;
const labelStyle = { color: theme.textFaint, fontSize: 11, fontWeight: 700 } as const;

export default function BusinessMemoryPanel({ context, message, isSaving, onChange, onSave }: BusinessMemoryPanelProps) {
  return (
    <div style={cardStyle()}>
      <Row style={{ gap: 9, marginBottom: 12 }}>
        <Brain size={16} color={theme.primaryLight} />
        <p style={{ color: theme.text, fontWeight: 600, fontSize: 13 }}>Định vị doanh nghiệp</p>
      </Row>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', gap: 10, marginBottom: 10 }}>
        <label style={fieldStyle}>
          <span style={labelStyle}>Brand / tổ chức</span>
          <input value={context.business_name || ''} onChange={e => onChange({ business_name: e.target.value })} placeholder="VD: THG Fulfill" style={inputStyle} />
        </label>
        <label style={fieldStyle}>
          <span style={labelStyle}>Ngành / mô hình</span>
          <input value={context.business_industry || ''} onChange={e => onChange({ business_industry: e.target.value })} placeholder="VD: fulfillment, logistics ecommerce" style={inputStyle} />
        </label>
        <label style={fieldStyle}>
          <span style={labelStyle}>Sản phẩm / dịch vụ</span>
          <input value={context.services || ''} onChange={e => onChange({ services: e.target.value })} placeholder="Dịch vụ chính, offer, gói bán" style={inputStyle} />
        </label>
        <label style={fieldStyle}>
          <span style={labelStyle}>Vai trò tệp cần tìm</span>
          <select value={context.target_author_role || 'customers'} onChange={e => onChange({ target_author_role: e.target.value })} style={inputStyle}>
            <option value="customers">Khách đang có nhu cầu</option>
            <option value="suppliers">Nhà cung cấp / xưởng / vendor</option>
            <option value="partners">Đối tác / reseller</option>
            <option value="candidates">Ứng viên / nhân sự</option>
            <option value="providers">Đơn vị cung cấp dịch vụ</option>
          </select>
        </label>
      </div>

      <label style={{ ...fieldStyle, marginBottom: 10 }}>
        <span style={labelStyle}>Chân dung tệp mục tiêu</span>
        <textarea
          value={context.target_customers || ''}
          onChange={e => onChange({ target_customers: e.target.value })}
          placeholder="Ai là người cần tìm, pain point gì, quy mô nào, thị trường nào..."
          style={{ ...inputStyle, minHeight: 74, resize: 'vertical', lineHeight: 1.55 }}
        />
      </label>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', gap: 10, marginBottom: 10 }}>
        <label style={fieldStyle}>
          <span style={labelStyle}>Tín hiệu phải giữ lại</span>
          <textarea value={context.target_signals || ''} onChange={e => onChange({ target_signals: e.target.value })} placeholder="cần tìm, xin báo giá, looking for, cần supplier..." style={{ ...inputStyle, minHeight: 86, resize: 'vertical', lineHeight: 1.55 }} />
        </label>
        <label style={fieldStyle}>
          <span style={labelStyle}>Tín hiệu loại bỏ</span>
          <textarea value={context.negative_signals || ''} onChange={e => onChange({ negative_signals: e.target.value })} placeholder="bên em cung cấp, hotline, tuyển CTV, spam link..." style={{ ...inputStyle, minHeight: 86, resize: 'vertical', lineHeight: 1.55 }} />
        </label>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', gap: 10, marginBottom: 10 }}>
        <label style={fieldStyle}>
          <span style={labelStyle}>Thị trường ưu tiên</span>
          <input value={context.markets || ''} onChange={e => onChange({ markets: e.target.value })} placeholder="VN, US, EU, local city..." style={inputStyle} />
        </label>
        <label style={fieldStyle}>
          <span style={labelStyle}>Khu vực hoạt động</span>
          <input value={context.business_location || ''} onChange={e => onChange({ business_location: e.target.value })} placeholder="HCM, Hà Nội, toàn quốc, quốc tế..." style={inputStyle} />
        </label>
        <label style={fieldStyle}>
          <span style={labelStyle}>Giọng tư vấn</span>
          <input value={context.tone || ''} onChange={e => onChange({ tone: e.target.value })} placeholder="Tư vấn, ngắn gọn, chuyên gia, friendly..." style={inputStyle} />
        </label>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', gap: 10, marginBottom: 10 }}>
        <label style={fieldStyle}>
          <span style={labelStyle}>Điểm khác biệt</span>
          <textarea
            value={context.business_usp || ''}
            onChange={e => onChange({ business_usp: e.target.value })}
            placeholder="Lý do khách nên chọn bạn: tốc độ, giá, kinh nghiệm, network, bảo hành..."
            style={{ ...inputStyle, minHeight: 82, resize: 'vertical', lineHeight: 1.55 }}
          />
        </label>
        <label style={fieldStyle}>
          <span style={labelStyle}>Chính sách automation</span>
          <textarea
            value={context.approval_policy || ''}
            onChange={e => onChange({ approval_policy: e.target.value })}
            placeholder="VD: comment cần duyệt, inbox auto với hot lead, không inbox ngoài giờ..."
            style={{ ...inputStyle, minHeight: 82, resize: 'vertical', lineHeight: 1.55 }}
          />
        </label>
      </div>

      <label style={{ ...fieldStyle, marginBottom: 10 }}>
        <span style={labelStyle}>Luật loại bỏ nâng cao</span>
        <textarea
          value={context.reject_rules || ''}
          onChange={e => onChange({ reject_rules: e.target.value })}
          placeholder="Những loại post/lead không bao giờ đưa vào pipeline, kể cả có keyword đúng."
          style={{ ...inputStyle, minHeight: 74, resize: 'vertical', lineHeight: 1.55 }}
        />
      </label>

      <textarea
        value={context.business_profile || ''}
        onChange={e => onChange({ business_profile: e.target.value })}
        placeholder="Mô tả tự do: định vị thương hiệu, mô hình kinh doanh, chính sách, nguồn dữ liệu ưu tiên, điều cấm..."
        style={{
          ...inputStyle,
          minHeight: 120,
          resize: 'vertical',
          lineHeight: 1.55,
        }}
      />
      <Row style={{ justifyContent: 'space-between', gap: 10, marginTop: 10 }}>
        <p style={{ color: theme.textFaint, fontSize: 12 }}>Định vị này điều khiển crawler, classifier và outbound của workspace.</p>
        <Row style={{ gap: 10 }}>
          {message && <span style={{ color: message.startsWith('Đã') ? '#4ade80' : '#fca5a5', fontSize: 12 }}>{message}</span>}
          <button disabled={isSaving} onClick={onSave} style={primaryBtn({ padding: '8px 14px', fontSize: 12, display: 'flex', alignItems: 'center', gap: 6, opacity: isSaving ? 0.6 : 1 })}>
            <Save size={13} /> Lưu định vị
          </button>
        </Row>
      </Row>
    </div>
  );
}
