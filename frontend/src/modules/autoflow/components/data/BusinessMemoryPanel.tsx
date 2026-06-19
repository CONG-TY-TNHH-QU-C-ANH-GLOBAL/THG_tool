import { Brain, Save } from 'lucide-react';
import { Row } from '../ui';
import { cardStyle, inputStyle, primaryBtn, theme } from '../../constants/styles';
import type { BusinessContext, InferredFieldKey } from '../../services/settingsService';
import ConfidenceTag from './ConfidenceTag';

export type BusinessConfidences = Partial<Record<InferredFieldKey, number>>;

interface BusinessMemoryPanelProps {
  context: BusinessContext;
  message: string;
  isSaving: boolean;
  confidences?: BusinessConfidences;
  onChange: (patch: Partial<BusinessContext>) => void;
  onSave: () => void;
}

const fieldStyle = { display: 'flex', flexDirection: 'column', gap: 6 } as const;
const labelStyle = { color: theme.textFaint, fontSize: 11, fontWeight: 700 } as const;

function Label({ text, confidence }: Readonly<{ text: string; confidence?: number }>) {
  return (
    <span style={{ ...labelStyle, display: 'inline-flex', alignItems: 'center', gap: 6 }}>
      {text}
      <ConfidenceTag confidence={confidence} />
    </span>
  );
}

export default function BusinessMemoryPanel({ context, message, isSaving, confidences, onChange, onSave }: Readonly<BusinessMemoryPanelProps>) {
  // When the user edits a field, we drop its confidence — the value is
  // now user-owned, not AI-suggested. This is a no-op when confidences
  // wasn't passed in.
  const cf = confidences ?? {};
  return (
    <div style={cardStyle()}>
      <Row style={{ gap: 9, marginBottom: 12 }}>
        <Brain size={16} color={theme.primaryLight} />
        <p style={{ color: theme.text, fontWeight: 600, fontSize: 13 }}>Định vị doanh nghiệp</p>
      </Row>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', gap: 10, marginBottom: 10 }}>
        <label style={fieldStyle}>
          <Label text="Brand / tổ chức" confidence={cf.business_name} />
          <input value={context.business_name || ''} onChange={e => onChange({ business_name: e.target.value })} placeholder="VD: THG Fulfill" style={inputStyle} />
        </label>
        <label style={fieldStyle}>
          <Label text="Ngành / mô hình" confidence={cf.business_industry} />
          <input value={context.business_industry || ''} onChange={e => onChange({ business_industry: e.target.value })} placeholder="VD: fulfillment, logistics ecommerce" style={inputStyle} />
        </label>
        <label style={fieldStyle}>
          <Label text="Sản phẩm / dịch vụ" confidence={cf.services} />
          <input value={context.services || ''} onChange={e => onChange({ services: e.target.value })} placeholder="Dịch vụ chính, offer, gói bán" style={inputStyle} />
        </label>
        <label style={fieldStyle}>
          <Label text="Vai trò tệp cần tìm" confidence={cf.target_author_role} />
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
        <Label text="Chân dung tệp mục tiêu" confidence={cf.target_customers} />
        <textarea
          value={context.target_customers || ''}
          onChange={e => onChange({ target_customers: e.target.value })}
          placeholder="Ai là người cần tìm, pain point gì, quy mô nào, thị trường nào..."
          style={{ ...inputStyle, minHeight: 74, resize: 'vertical', lineHeight: 1.55 }}
        />
      </label>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', gap: 10, marginBottom: 10 }}>
        <label style={fieldStyle}>
          <Label text="Tín hiệu phải giữ lại" confidence={cf.target_signals} />
          <textarea value={context.target_signals || ''} onChange={e => onChange({ target_signals: e.target.value })} placeholder="cần tìm, xin báo giá, looking for, cần supplier..." style={{ ...inputStyle, minHeight: 86, resize: 'vertical', lineHeight: 1.55 }} />
        </label>
        <label style={fieldStyle}>
          <Label text="Tín hiệu loại bỏ" confidence={cf.negative_signals} />
          <textarea value={context.negative_signals || ''} onChange={e => onChange({ negative_signals: e.target.value })} placeholder="bên em cung cấp, hotline, tuyển CTV, spam link..." style={{ ...inputStyle, minHeight: 86, resize: 'vertical', lineHeight: 1.55 }} />
        </label>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', gap: 10, marginBottom: 10 }}>
        <label style={fieldStyle}>
          <Label text="Thị trường ưu tiên" confidence={cf.markets} />
          <input value={context.markets || ''} onChange={e => onChange({ markets: e.target.value })} placeholder="VN, US, EU, local city..." style={inputStyle} />
        </label>
        <label style={fieldStyle}>
          <Label text="Khu vực hoạt động" confidence={cf.business_location} />
          <input value={context.business_location || ''} onChange={e => onChange({ business_location: e.target.value })} placeholder="HCM, Hà Nội, toàn quốc, quốc tế..." style={inputStyle} />
        </label>
        <label style={fieldStyle}>
          <Label text="Giọng tư vấn" confidence={cf.tone} />
          <input value={context.tone || ''} onChange={e => onChange({ tone: e.target.value })} placeholder="Tư vấn, ngắn gọn, chuyên gia, friendly..." style={inputStyle} />
        </label>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', gap: 10, marginBottom: 10 }}>
        <label style={fieldStyle}>
          <Label text="Điểm khác biệt" confidence={cf.business_usp} />
          <textarea
            value={context.business_usp || ''}
            onChange={e => onChange({ business_usp: e.target.value })}
            placeholder="Lý do khách nên chọn bạn: tốc độ, giá, kinh nghiệm, network, bảo hành..."
            style={{ ...inputStyle, minHeight: 82, resize: 'vertical', lineHeight: 1.55 }}
          />
        </label>
        <label style={fieldStyle}>
          <Label text="Chính sách automation" confidence={cf.approval_policy} />
          <textarea
            value={context.approval_policy || ''}
            onChange={e => onChange({ approval_policy: e.target.value })}
            placeholder="VD: comment cần duyệt, inbox auto với hot lead, không inbox ngoài giờ..."
            style={{ ...inputStyle, minHeight: 82, resize: 'vertical', lineHeight: 1.55 }}
          />
        </label>
      </div>

      <label style={{ ...fieldStyle, marginBottom: 10 }}>
        <Label text="Luật loại bỏ nâng cao" confidence={cf.reject_rules} />
        <textarea
          value={context.reject_rules || ''}
          onChange={e => onChange({ reject_rules: e.target.value })}
          placeholder="Những loại post/lead không bao giờ đưa vào pipeline, kể cả có keyword đúng."
          style={{ ...inputStyle, minHeight: 74, resize: 'vertical', lineHeight: 1.55 }}
        />
      </label>

      <Label text="Tóm tắt định vị (mô tả tự do)" confidence={cf.business_profile} />
      <textarea
        value={context.business_profile || ''}
        onChange={e => onChange({ business_profile: e.target.value })}
        placeholder="Mô tả tự do: định vị thương hiệu, mô hình kinh doanh, chính sách, nguồn dữ liệu ưu tiên, điều cấm..."
        style={{
          ...inputStyle,
          minHeight: 120,
          resize: 'vertical',
          lineHeight: 1.55,
          marginTop: 6,
        }}
      />
      <Row style={{ justifyContent: 'space-between', gap: 10, marginTop: 10 }}>
        <p style={{ color: theme.textFaint, fontSize: 12 }}>Định vị này điều khiển crawler, classifier và outbound của workspace.</p>
        <Row style={{ gap: 10 }}>
          {message && <span style={{ color: message.startsWith('Đã') ? theme.green : theme.red, fontSize: 12 }}>{message}</span>}
          <button disabled={isSaving} onClick={onSave} style={primaryBtn({ padding: '8px 14px', fontSize: 12, display: 'flex', alignItems: 'center', gap: 6, opacity: isSaving ? 0.6 : 1 })}>
            <Save size={13} /> Lưu định vị
          </button>
        </Row>
      </Row>
    </div>
  );
}
