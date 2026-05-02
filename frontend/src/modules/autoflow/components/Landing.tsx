import { theme, primaryBtn, secondaryBtn, cardStyle, rootStyle } from '../constants/styles';
import { Bot, Building2, Check, Database, Globe2, ShieldCheck, Target, Trophy, Zap } from 'lucide-react';

interface LandingProps {
  onLogin: () => void;
  onRegister: () => void;
  onAdmin: () => void;
}

const PREVIEW_ROWS = [
  { name: 'Lead thật từ crawler', status: 'Hot', score: 92 },
  { name: 'Bài viết cần tư vấn', status: 'Warm', score: 74 },
  { name: 'Tín hiệu yếu', status: 'Cold', score: 41 },
];

const FEATS = [
  { I: Bot, t: 'AI Agents 24/7', d: 'Nhận prompt từ dashboard hoặc Telegram, chạy crawler, lọc lead và tạo hành động theo workflow.' },
  { I: Building2, t: 'Multi-Organization', d: 'Mỗi workspace có dữ liệu, nhân viên, Facebook session và calibration riêng.' },
  { I: Target, t: 'Market Signal Gate', d: 'Ưu tiên lead có nhu cầu thật, loại quảng cáo dịch vụ và tín hiệu nhiễu trước khi đổ về dashboard.' },
  { I: Trophy, t: 'KPI Leaderboard', d: 'Bảng hiệu suất sales lấy từ dữ liệu thật của workspace, không dùng mock UI.' },
  { I: Database, t: 'Private Data', d: 'Kết nối file, Google Sheet và memory doanh nghiệp để agent hiểu sản phẩm, giá và tone.' },
  { I: ShieldCheck, t: 'Local Runtime', d: 'Facebook chạy trên session thật của organization, browser automation vẫn quan sát được từ dashboard.' },
];

const PLANS = [
  { n: 'Starter', p: '990K', f: ['1 tổ chức', '3 AI Agents', '5 nhân viên', 'Email support'] },
  { n: 'Pro', p: '2.9M', f: ['3 tổ chức', '10 AI Agents', '20 nhân viên', 'Priority support', 'Custom branding'], hot: true },
  { n: 'Enterprise', p: 'Liên hệ', f: ['Unlimited org', 'Unlimited agents', 'SLA 99.9%', 'Dedicated support'] },
];

const statusTone = (status: string) => ({ Hot: theme.red, Warm: theme.yellow, Cold: theme.blue }[status] ?? theme.textFaint);

export default function Landing({ onLogin, onRegister, onAdmin }: LandingProps) {
  return (
    <div style={{ ...rootStyle, minHeight: '100vh', overflowY: 'auto' }}>
      <nav className="af-glass" style={{ display: 'flex', alignItems: 'center', padding: '15px 48px', borderTop: 0, borderLeft: 0, borderRight: 0, borderRadius: 0, position: 'sticky', top: 0, zIndex: 20 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <div style={{ width: 34, height: 34, background: `linear-gradient(135deg, ${theme.primary}, ${theme.primaryLight})`, borderRadius: 8, display: 'grid', placeItems: 'center', boxShadow: '0 16px 34px rgba(24,86,255,0.28)' }}>
            <Zap size={16} color="#fff" />
          </div>
          <span style={{ fontWeight: 850, fontSize: 17, color: theme.textWhite }}>AutoFlow</span>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 24, marginLeft: 44 }}>
          {['Tính năng', 'Bảng giá', 'Bảo mật'].map(label => (
            <a key={label} href="#" style={{ color: theme.textMuted, fontSize: 13, textDecoration: 'none', fontWeight: 650 }}>{label}</a>
          ))}
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginLeft: 'auto' }}>
          <button onClick={onLogin} style={secondaryBtn({ padding: '7px 18px' })}>Đăng nhập</button>
          <button onClick={onRegister} style={primaryBtn({ padding: '7px 18px' })}>Dùng thử miễn phí</button>
        </div>
      </nav>

      <section style={{ display: 'grid', gridTemplateColumns: 'minmax(320px, 0.9fr) minmax(420px, 1.1fr)', gap: 34, alignItems: 'center', padding: '68px 48px 52px', maxWidth: 1180, margin: '0 auto' }}>
        <div>
          <div style={{ display: 'inline-flex', alignItems: 'center', gap: 8, background: 'rgba(24,86,255,0.12)', border: `1px solid ${theme.primaryLight}55`, color: theme.primaryPale, fontSize: 12, fontWeight: 800, padding: '6px 12px', borderRadius: 99, marginBottom: 18 }}>
            <Globe2 size={14} /> Facebook Sales Intelligence Workspace
          </div>
          <h1 style={{ fontSize: 48, fontWeight: 900, color: theme.textWhite, lineHeight: 1.12, margin: '0 0 16px', maxWidth: 720 }}>
            Automation Facebook phải bắt đầu từ hiểu doanh nghiệp.
          </h1>
          <p style={{ color: theme.textMuted, fontSize: 16, maxWidth: 560, margin: '0 0 30px', lineHeight: 1.8 }}>
            THG AutoFlow kết hợp business calibration, browser session thật và agent workflow để tìm đúng tệp khách, lọc tín hiệu nhiễu và hỗ trợ sales team làm việc nhanh hơn.
          </p>
          <div style={{ display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap' }}>
            <button onClick={onRegister} style={primaryBtn({ padding: '13px 24px', fontSize: 15 })}>Tạo workspace</button>
            <button onClick={onLogin} style={secondaryBtn({ padding: '13px 24px', fontSize: 15 })}>Vào dashboard</button>
          </div>
        </div>

        <div style={cardStyle({ padding: 14 })}>
          <div style={{ display: 'flex', gap: 6, marginBottom: 12 }}>
            {[theme.red, theme.yellow, theme.green].map(color => <div key={color} style={{ width: 10, height: 10, borderRadius: '50%', background: color }} />)}
          </div>
          <div style={{ display: 'grid', gridTemplateColumns: '128px 1fr', gap: 12, minHeight: 260 }}>
            <div style={{ background: theme.surfaceAlt, border: `1px solid ${theme.borderAlt}`, borderRadius: 8, padding: 10 }}>
              {['Leads', 'Chat', 'Browser', 'Data Private', 'Settings'].map((label, index) => (
                <div key={label} style={{ padding: '8px 9px', borderRadius: 8, background: index === 0 ? `linear-gradient(135deg, ${theme.primary}, ${theme.primaryDark})` : 'transparent', color: index === 0 ? '#fff' : theme.textMuted, fontSize: 11, fontWeight: 750, marginBottom: 4 }}>{label}</div>
              ))}
            </div>
            <div style={{ display: 'grid', gap: 10 }}>
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4,1fr)', gap: 8 }}>
                {[{ l: 'Leads', v: '1,284' }, { l: 'Hot', v: '342' }, { l: 'Reply', v: '89' }, { l: 'Revenue', v: '4.2B' }].map(stat => (
                  <div key={stat.l} style={{ background: theme.surfaceAlt, border: `1px solid ${theme.borderAlt}`, borderRadius: 8, padding: '10px 11px' }}>
                    <p style={{ color: theme.textFaint, fontSize: 10, margin: 0 }}>{stat.l}</p>
                    <p style={{ color: theme.textWhite, fontWeight: 850, fontSize: 15, margin: '3px 0 0' }}>{stat.v}</p>
                  </div>
                ))}
              </div>
              <div style={{ background: theme.surfaceAlt, border: `1px solid ${theme.borderAlt}`, borderRadius: 8, padding: 12 }}>
                {PREVIEW_ROWS.map(row => (
                  <div key={row.name} style={{ display: 'flex', alignItems: 'center', gap: 9, padding: '8px 0', borderBottom: `1px solid ${theme.borderAlt}` }}>
                    <div style={{ width: 22, height: 22, borderRadius: 8, background: `linear-gradient(135deg, ${theme.primary}, ${theme.primaryLight})`, display: 'grid', placeItems: 'center', fontSize: 10, fontWeight: 800, color: '#fff' }}>{row.name[0]}</div>
                    <span style={{ color: theme.text, fontSize: 12, flex: 1 }}>{row.name}</span>
                    <span style={{ background: statusTone(row.status) + '22', color: statusTone(row.status), border: `1px solid ${statusTone(row.status)}55`, fontSize: 10, padding: '2px 7px', borderRadius: 99 }}>{row.status}</span>
                    <span style={{ color: theme.textFaint, fontSize: 11 }}>{row.score}</span>
                  </div>
                ))}
              </div>
            </div>
          </div>
        </div>
      </section>

      <section style={{ padding: '28px 48px' }}>
        <div style={{ ...cardStyle({ maxWidth: 880, margin: '0 auto', padding: 24 }), display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', textAlign: 'center', gap: 16 }}>
          {[{ v: '500+', l: 'Tổ chức' }, { v: '50K+', l: 'Leads/tháng' }, { v: '98%', l: 'Uptime' }, { v: '3x', l: 'Năng suất sales' }].map(stat => (
            <div key={stat.l}>
              <p style={{ fontSize: 30, fontWeight: 900, color: theme.primaryLight, margin: 0 }}>{stat.v}</p>
              <p style={{ color: theme.textMuted, fontSize: 13, margin: '4px 0 0' }}>{stat.l}</p>
            </div>
          ))}
        </div>
      </section>

      <section style={{ padding: '56px 48px', maxWidth: 1080, margin: '0 auto' }}>
        <h2 style={{ textAlign: 'center', fontSize: 32, fontWeight: 850, color: theme.textWhite, marginBottom: 32 }}>Một workspace, nhiều lớp intelligence</h2>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, minmax(0, 1fr))', gap: 14 }}>
          {FEATS.map(({ I, t, d }) => (
            <div key={t} style={cardStyle({ padding: 22 })}>
              <div style={{ width: 34, height: 34, borderRadius: 8, background: 'rgba(24,86,255,0.15)', border: `1px solid ${theme.primaryLight}44`, display: 'grid', placeItems: 'center', marginBottom: 12 }}>
                <I size={17} color={theme.primaryLight} />
              </div>
              <h3 style={{ color: theme.textWhite, fontSize: 14, fontWeight: 800, margin: '0 0 7px' }}>{t}</h3>
              <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.7, margin: 0 }}>{d}</p>
            </div>
          ))}
        </div>
      </section>

      <section style={{ padding: '46px 48px 60px', maxWidth: 920, margin: '0 auto' }}>
        <h2 style={{ textAlign: 'center', fontSize: 32, fontWeight: 850, color: theme.textWhite, marginBottom: 32 }}>Bảng giá minh bạch</h2>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, minmax(0, 1fr))', gap: 14 }}>
          {PLANS.map(plan => (
            <div key={plan.n} style={cardStyle({ padding: 24, position: 'relative', border: `1px solid ${plan.hot ? theme.primaryLight : theme.border}` })}>
              {plan.hot && <span style={{ position: 'absolute', top: -11, left: '50%', transform: 'translateX(-50%)', background: theme.primary, color: '#fff', fontSize: 10, fontWeight: 800, padding: '3px 12px', borderRadius: 99, whiteSpace: 'nowrap' }}>Phổ biến nhất</span>}
              <p style={{ color: plan.hot ? theme.primaryPale : theme.textMuted, fontWeight: 800, fontSize: 13, margin: 0 }}>{plan.n}</p>
              <p style={{ color: theme.textWhite, fontSize: 28, fontWeight: 900, margin: '8px 0' }}>
                {plan.p}<span style={{ fontSize: 12, color: theme.textFaint, fontWeight: 500 }}>{plan.p !== 'Liên hệ' ? '/tháng' : ''}</span>
              </p>
              <div style={{ borderTop: `1px solid ${theme.borderAlt}`, paddingTop: 12, marginBottom: 14 }}>
                {plan.f.map(item => (
                  <div key={item} style={{ display: 'flex', gap: 7, alignItems: 'center', marginBottom: 8 }}>
                    <Check size={12} color={theme.green} />
                    <span style={{ color: theme.textMuted, fontSize: 12 }}>{item}</span>
                  </div>
                ))}
              </div>
              <button onClick={onRegister} style={{ width: '100%', ...primaryBtn({ background: plan.hot ? undefined : theme.surfaceAlt, boxShadow: plan.hot ? undefined : 'none' }) }}>
                {plan.n === 'Enterprise' ? 'Liên hệ' : 'Bắt đầu ngay'}
              </button>
            </div>
          ))}
        </div>
      </section>

      <footer style={{ padding: '22px 48px', borderTop: `1px solid ${theme.borderAlt}`, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <Zap size={13} color={theme.primaryLight} />
          <span style={{ color: theme.textFaint, fontSize: 12 }}>AutoFlow © 2026. All rights reserved.</span>
        </div>
        <button onClick={onAdmin} style={{ background: 'none', border: 'none', color: theme.textFaint, fontSize: 11, cursor: 'pointer' }}>Admin Portal</button>
      </footer>
    </div>
  );
}
