import { theme } from '../constants/styles';
import { MOCK_LEADS } from '../services/mockData';
import { Zap, Check } from 'lucide-react';

interface LandingProps {
  onLogin: () => void;
  onRegister: () => void;
  onAdmin: () => void;
}

const D = { background: theme.bg, color: theme.text, fontFamily: 'system-ui, sans-serif' };
const PB = (p: Record<string, unknown> = {}) => ({ padding: '10px 20px', borderRadius: 9, border: 'none', cursor: 'pointer', fontSize: 14, fontWeight: 500, background: theme.primary, color: '#fff', ...p });
const SB = (p: Record<string, unknown> = {}) => ({ padding: '10px 20px', borderRadius: 9, border: '1px solid #374151', cursor: 'pointer', fontSize: 13, background: 'transparent', color: '#d1d5db', ...p });
const sc = (s: string) => ({ Hot: '#ef4444', Warm: '#f59e0b', Cold: '#3b82f6' }[s] ?? '#6b7280');

const FEATS = [
  { e: '⚡', t: 'AI Agents 24/7', d: 'Tự động tư vấn, chốt lead liên tục không cần nhân viên trực.' },
  { e: '🏢', t: 'Multi-Organization', d: 'Một platform, nhiều tổ chức, dữ liệu hoàn toàn độc lập.' },
  { e: '🎯', t: 'Lead Scoring AI', d: 'Chấm điểm và phân loại lead theo tỷ lệ chốt thực tế.' },
  { e: '🏆', t: 'KPI Leaderboard', d: 'Admin tự cấu hình thưởng phạt, không cần coder.' },
  { e: '🔒', t: 'Private Data', d: 'Upload tệp kinh doanh riêng, AI học theo sản phẩm của bạn.' },
  { e: '📱', t: 'Facebook Native', d: 'Tích hợp trực tiếp Facebook, session bền vững.' },
];

const PLANS = [
  { n: 'Starter', p: '990K', f: ['1 tổ chức', '3 AI Agents', '5 nhân viên', 'Email support'] },
  { n: 'Pro', p: '2.9M', f: ['3 tổ chức', '10 AI Agents', '20 nhân viên', 'Priority support', 'Custom branding'], hot: true },
  { n: 'Enterprise', p: 'Liên hệ', f: ['Unlimited org', 'Unlimited agents', 'SLA 99.9%', 'Dedicated support'] },
];

export default function Landing({ onLogin, onRegister, onAdmin }: LandingProps) {
  return (
    <div style={{ ...D, minHeight: '100vh', overflowY: 'auto' }}>
      {/* NAV */}
      <nav style={{ display: 'flex', alignItems: 'center', padding: '15px 48px', borderBottom: '1px solid #1e2130', position: 'sticky', top: 0, background: '#0d101aee', zIndex: 20 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <div style={{ width: 32, height: 32, background: theme.primary, borderRadius: 8, display: 'flex', alignItems: 'center', justifyContent: 'center' }}><Zap size={16} color="#fff" /></div>
          <span style={{ fontWeight: 800, fontSize: 17, color: '#fff' }}>AutoFlow</span>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 28, marginLeft: 44 }}>
          {['Tính năng', 'Bảng giá', 'Về chúng tôi'].map(l => <a key={l} href="#" style={{ color: '#9ca3af', fontSize: 13, textDecoration: 'none' }}>{l}</a>)}
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginLeft: 'auto' }}>
          <button onClick={onLogin} style={SB({ padding: '7px 18px' }) as React.CSSProperties}>Đăng nhập</button>
          <button onClick={onRegister} style={PB({ padding: '7px 18px', fontWeight: 700 }) as React.CSSProperties}>Dùng thử miễn phí</button>
        </div>
      </nav>

      {/* HERO */}
      <section style={{ textAlign: 'center', padding: '72px 24px 52px' }}>
        <div style={{ display: 'inline-block', background: '#312e8122', border: '1px solid #4f46e544', color: '#a5b4fc', fontSize: 12, padding: '5px 14px', borderRadius: 99, marginBottom: 18 }}>
          🚀 Facebook Automation Platform #1 Việt Nam
        </div>
        <h1 style={{ fontSize: 48, fontWeight: 900, color: '#fff', lineHeight: 1.15, margin: '0 auto 16px', maxWidth: 680 }}>
          Tự động hóa sales Facebook<br /><span style={{ color: '#818cf8' }}>tăng doanh thu 3×</span>
        </h1>
        <p style={{ color: '#9ca3af', fontSize: 16, maxWidth: 500, margin: '0 auto 32px', lineHeight: 1.8 }}>
          AI agents làm việc 24/7, tự động tư vấn và chốt leads từ hàng nghìn nhóm Facebook.
        </p>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, justifyContent: 'center' }}>
          <button onClick={onRegister} style={PB({ padding: '13px 30px', fontSize: 15, fontWeight: 800 }) as React.CSSProperties}>Tạo tổ chức miễn phí →</button>
          <button style={SB({ padding: '13px 30px', fontSize: 15 }) as React.CSSProperties}>Xem demo</button>
        </div>
        {/* Mini preview */}
        <div style={{ maxWidth: 740, margin: '48px auto 0', background: '#111520', border: '1px solid #1e2130', borderRadius: 16, padding: 14, textAlign: 'left' }}>
          <div style={{ display: 'flex', gap: 5, marginBottom: 10 }}>
            {['#ef4444', '#f59e0b', '#22c55e'].map(c => <div key={c} style={{ width: 9, height: 9, borderRadius: '50%', background: c }} />)}
          </div>
          <div style={{ display: 'flex', gap: 10, height: 150 }}>
            <div style={{ width: 105, background: '#0d101a', borderRadius: 8, padding: 8 }}>
              {['Leads', 'Inbox', 'Posting', 'Leaderboard', 'Settings'].map((t, i) => (
                <div key={t} style={{ padding: '5px 8px', borderRadius: 5, background: i === 0 ? theme.primary : 'transparent', color: i === 0 ? '#fff' : '#6b7280', fontSize: 10, marginBottom: 3 }}>{t}</div>
              ))}
            </div>
            <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 7 }}>
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4,1fr)', gap: 6 }}>
                {[{ l: 'Total', v: '1,284' }, { l: 'Hot', v: '342' }, { l: 'Conv', v: '89' }, { l: 'Revenue', v: '₫4.2B' }].map(s => (
                  <div key={s.l} style={{ background: '#1e2130', borderRadius: 7, padding: '8px 10px' }}>
                    <p style={{ color: '#6b7280', fontSize: 8 }}>{s.l}</p>
                    <p style={{ color: '#fff', fontWeight: 700, fontSize: 13 }}>{s.v}</p>
                  </div>
                ))}
              </div>
              <div style={{ flex: 1, background: '#1e2130', borderRadius: 7, padding: 10 }}>
                {MOCK_LEADS.slice(0, 3).map(l => (
                  <div key={l.id} style={{ display: 'flex', alignItems: 'center', gap: 7, marginBottom: 6 }}>
                    <div style={{ width: 18, height: 18, background: theme.primary, borderRadius: '50%', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 8, color: '#fff' }}>{l.name[0]}</div>
                    <span style={{ color: '#d1d5db', fontSize: 10, flex: 1 }}>{l.name}</span>
                    <span style={{ background: sc(l.status) + '22', color: sc(l.status), fontSize: 9, padding: '1px 6px', borderRadius: 99 }}>{l.status}</span>
                    <span style={{ color: '#6b7280', fontSize: 9 }}>{l.score}</span>
                  </div>
                ))}
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* STATS */}
      <div style={{ padding: '40px 48px', background: '#111520', borderTop: '1px solid #1e2130', borderBottom: '1px solid #1e2130' }}>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4,1fr)', maxWidth: 760, margin: '0 auto', textAlign: 'center', gap: 16 }}>
          {[{ v: '500+', l: 'Tổ chức tin dùng' }, { v: '50K+', l: 'Leads/tháng' }, { v: '98%', l: 'Uptime' }, { v: '3×', l: 'Tăng doanh thu TB' }].map(s => (
            <div key={s.l}>
              <p style={{ fontSize: 30, fontWeight: 900, color: '#818cf8' }}>{s.v}</p>
              <p style={{ color: '#9ca3af', fontSize: 13, marginTop: 4 }}>{s.l}</p>
            </div>
          ))}
        </div>
      </div>

      {/* FEATURES */}
      <section style={{ padding: '56px 48px', maxWidth: 1040, margin: '0 auto' }}>
        <h2 style={{ textAlign: 'center', fontSize: 32, fontWeight: 800, color: '#fff', marginBottom: 36 }}>Tất cả trong một nền tảng</h2>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3,1fr)', gap: 14 }}>
          {FEATS.map(f => (
            <div key={f.t} style={{ background: '#111520', border: '1px solid #1e2130', borderRadius: 14, padding: 22 }}>
              <span style={{ fontSize: 26 }}>{f.e}</span>
              <h3 style={{ color: '#e5e7eb', fontSize: 14, fontWeight: 600, margin: '10px 0 6px' }}>{f.t}</h3>
              <p style={{ color: '#6b7280', fontSize: 12, lineHeight: 1.7 }}>{f.d}</p>
            </div>
          ))}
        </div>
      </section>

      {/* PRICING */}
      <section style={{ padding: '56px 48px', maxWidth: 880, margin: '0 auto' }}>
        <h2 style={{ textAlign: 'center', fontSize: 32, fontWeight: 800, color: '#fff', marginBottom: 36 }}>Bảng giá minh bạch</h2>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3,1fr)', gap: 14 }}>
          {PLANS.map(p => (
            <div key={p.n} style={{ background: '#111520', border: `1px solid ${p.hot ? theme.primary : '#1e2130'}`, borderRadius: 16, padding: 24, position: 'relative' }}>
              {p.hot && (
                <span style={{ position: 'absolute', top: -11, left: '50%', transform: 'translateX(-50%)', background: theme.primary, color: '#fff', fontSize: 10, fontWeight: 700, padding: '3px 12px', borderRadius: 99, whiteSpace: 'nowrap' }}>Phổ biến nhất</span>
              )}
              <p style={{ color: p.hot ? '#a5b4fc' : '#9ca3af', fontWeight: 700, fontSize: 13 }}>{p.n}</p>
              <p style={{ color: '#fff', fontSize: 28, fontWeight: 900, margin: '8px 0' }}>
                {p.p}<span style={{ fontSize: 12, color: '#6b7280', fontWeight: 400 }}>{p.p !== 'Liên hệ' ? '/tháng' : ''}</span>
              </p>
              <div style={{ borderTop: '1px solid #1e2130', paddingTop: 12, marginBottom: 14 }}>
                {p.f.map(f => (
                  <div key={f} style={{ display: 'flex', gap: 7, alignItems: 'center', marginBottom: 8 }}>
                    <Check size={12} color="#22c55e" />
                    <span style={{ color: '#d1d5db', fontSize: 12 }}>{f}</span>
                  </div>
                ))}
              </div>
              <button onClick={onRegister} style={{ width: '100%', padding: '10px', background: p.hot ? theme.primary : '#1e2130', border: `1px solid ${p.hot ? theme.primary : '#374151'}`, borderRadius: 9, color: '#fff', fontSize: 13, cursor: 'pointer', fontWeight: p.hot ? 700 : 400 }}>
                {p.n === 'Enterprise' ? 'Liên hệ' : 'Bắt đầu ngay'}
              </button>
            </div>
          ))}
        </div>
      </section>

      {/* FOOTER */}
      <footer style={{ padding: '22px 48px', borderTop: '1px solid #1e2130', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <Zap size={13} color={theme.primary} />
          <span style={{ color: '#6b7280', fontSize: 12 }}>AutoFlow © 2025. All rights reserved.</span>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 20 }}>
          {['Điều khoản', 'Bảo mật', 'Liên hệ'].map(l => (
            <a key={l} href="#" style={{ color: '#6b7280', fontSize: 12, textDecoration: 'none' }}>{l}</a>
          ))}
          <button onClick={onAdmin} style={{ background: 'none', border: 'none', color: '#374151', fontSize: 11, cursor: 'pointer' }}>· Admin Portal</button>
        </div>
      </footer>
    </div>
  );
}
