import { Bot, Building2, Check, Database, Globe2, ShieldCheck, Target, Trophy, Zap } from 'lucide-react';
import { theme } from '../constants/styles';

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
  { I: ShieldCheck, t: 'Chrome Extension', d: 'Facebook chạy trên Chrome thật của organization, browser automation vẫn quan sát được từ dashboard.' },
];

const PLANS = [
  { n: 'Starter', p: '990K', f: ['1 tổ chức', '3 AI Agents', '5 nhân viên', 'Email support'] },
  { n: 'Pro', p: '2.9M', f: ['3 tổ chức', '10 AI Agents', '20 nhân viên', 'Priority support', 'Custom branding'], hot: true },
  { n: 'Enterprise', p: 'Liên hệ', f: ['Unlimited org', 'Unlimited agents', 'SLA 99.9%', 'Dedicated support'] },
];

const statusTone = (status: string) => ({ Hot: theme.red, Warm: theme.yellow, Cold: theme.blue }[status] ?? theme.textFaint);

export default function Landing({ onLogin, onRegister, onAdmin }: LandingProps) {
  return (
    <div className="landing-shell">
      <nav className="landing-nav af-glass">
        <div className="landing-brand">
          <div className="landing-brand-mark">
            <Zap size={16} color="#fff" />
          </div>
          <span>AutoFlow</span>
        </div>

        <div className="landing-nav-links">
          {['Tính năng', 'Bảng giá', 'Bảo mật'].map(label => (
            <a key={label} href="#">{label}</a>
          ))}
        </div>

        <div className="landing-nav-actions">
          <button type="button" className="af-marketing-btn af-marketing-btn-ghost" onClick={onLogin}>Đăng nhập</button>
          <button type="button" className="af-marketing-btn af-marketing-btn-primary" onClick={onRegister}>Dùng thử miễn phí</button>
        </div>
      </nav>

      <main>
        <section className="landing-hero">
          <div className="landing-hero-copy">
            <div className="landing-badge">
              <Globe2 size={14} />
              Facebook Sales Intelligence Workspace
            </div>
            <h1>Automation Facebook phải bắt đầu từ hiểu doanh nghiệp.</h1>
            <p>
              THG AutoFlow kết hợp business calibration, browser session thật và agent workflow để tìm đúng tệp khách,
              lọc tín hiệu nhiễu và giúp sales team làm việc nhanh hơn.
            </p>
            <div className="landing-cta-row">
              <button type="button" className="af-marketing-btn af-marketing-btn-primary af-marketing-btn-lg" onClick={onRegister}>Tạo workspace</button>
              <button type="button" className="af-marketing-btn af-marketing-btn-ghost af-marketing-btn-lg" onClick={onLogin}>Vào dashboard</button>
            </div>
          </div>

          <div className="landing-product-preview" aria-hidden="true">
            <div className="landing-window-dots">
              {[theme.red, theme.yellow, theme.green].map(color => <span key={color} style={{ background: color }} />)}
            </div>
            <div className="landing-preview-grid">
              <div className="landing-preview-sidebar">
                {['Leads', 'Chat', 'Browser', 'Data Private', 'Settings'].map((label, index) => (
                  <div key={label} className={index === 0 ? 'is-active' : ''}>{label}</div>
                ))}
              </div>
              <div className="landing-preview-main">
                <div className="landing-preview-stats">
                  {[{ l: 'Leads', v: '1,284' }, { l: 'Hot', v: '342' }, { l: 'Reply', v: '89' }, { l: 'Revenue', v: '4.2B' }].map(stat => (
                    <div key={stat.l}>
                      <small>{stat.l}</small>
                      <strong>{stat.v}</strong>
                    </div>
                  ))}
                </div>
                <div className="landing-preview-table">
                  {PREVIEW_ROWS.map(row => (
                    <div key={row.name} className="landing-preview-row">
                      <span className="landing-preview-avatar">{row.name[0]}</span>
                      <p>{row.name}</p>
                      <small style={{ color: statusTone(row.status), borderColor: `${statusTone(row.status)}66`, background: `${statusTone(row.status)}1f` }}>{row.status}</small>
                      <strong>{row.score}</strong>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          </div>
        </section>

        <section className="landing-trust-strip">
          {[{ v: '500+', l: 'Tổ chức' }, { v: '50K+', l: 'Leads/tháng' }, { v: '98%', l: 'Uptime' }, { v: '3x', l: 'Năng suất sales' }].map(stat => (
            <div key={stat.l}>
              <strong>{stat.v}</strong>
              <span>{stat.l}</span>
            </div>
          ))}
        </section>

        <section className="landing-section">
          <h2>Một workspace, nhiều lớp intelligence</h2>
          <div className="landing-feature-grid">
            {FEATS.map(({ I, t, d }) => (
              <article key={t} className="landing-card">
                <div className="landing-card-icon">
                  <I size={17} color={theme.primaryLight} />
                </div>
                <h3>{t}</h3>
                <p>{d}</p>
              </article>
            ))}
          </div>
        </section>

        <section className="landing-section landing-pricing-section">
          <h2>Bảng giá minh bạch</h2>
          <div className="landing-pricing-grid">
            {PLANS.map(plan => (
              <article key={plan.n} className={`landing-card landing-plan ${plan.hot ? 'is-hot' : ''}`}>
                {plan.hot && <span className="landing-plan-badge">Phổ biến nhất</span>}
                <p>{plan.n}</p>
                <h3>
                  {plan.p}
                  {plan.p !== 'Liên hệ' && <small>/tháng</small>}
                </h3>
                <div className="landing-plan-list">
                  {plan.f.map(item => (
                    <div key={item}>
                      <Check size={12} color={theme.green} />
                      <span>{item}</span>
                    </div>
                  ))}
                </div>
                <button type="button" className={`af-marketing-btn ${plan.hot ? 'af-marketing-btn-primary' : 'af-marketing-btn-ghost'}`} onClick={onRegister}>
                  {plan.n === 'Enterprise' ? 'Liên hệ' : 'Bắt đầu ngay'}
                </button>
              </article>
            ))}
          </div>
        </section>
      </main>

      <footer className="landing-footer">
        <div>
          <Zap size={13} color={theme.primaryLight} />
          <span>AutoFlow © 2026. All rights reserved.</span>
        </div>
        <button type="button" onClick={onAdmin}>Admin Portal</button>
      </footer>
    </div>
  );
}
