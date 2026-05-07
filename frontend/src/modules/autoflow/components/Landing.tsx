'use client';

import {
  ArrowRight,
  Bot,
  Database,
  MessagesSquare,
  ShieldCheck,
  Target,
  Workflow,
  Zap,
} from 'lucide-react';

import { useLang } from '../i18n/useLang';
import { LangSwitch } from './ds/LangSwitch';
import styles from './Landing.module.css';

interface LandingProps {
  onLogin: () => void;
  onRegister: () => void;
  onAdmin: () => void;
}

const COPY = {
  vi: {
    navFeatures: 'Tính năng',
    navFlow: 'Quy trình',
    navSecurity: 'Bảo mật',
    login: 'Đăng nhập',
    register: 'Tạo workspace',
    heroKicker: 'Facebook Sales Intelligence Workspace',
    heroTitle: 'Facebook automation chỉ phát huy giá trị khi hệ thống thực sự hiểu doanh nghiệp.',
    heroBody:
      'THG AutoFlow đồng bộ trình duyệt Facebook thật, hồ sơ doanh nghiệp, dữ liệu nội bộ, tín hiệu thị trường và ngữ điệu sales vào một workspace duy nhất — để AI agent không crawl mù và không hành động vô nghĩa.',
    heroPrimary: 'Tạo workspace ngay',
    heroSecondary: 'Vào dashboard',
    heroNote:
      'Extension cài trên Chrome cá nhân của nhân viên. Dashboard nhận stream, tín hiệu lead, action log và kết quả automation theo thời gian thực.',
    heroStats: [
      { label: 'Browser stream', value: 'Live' },
      { label: 'Market gate', value: '24/7' },
      { label: 'Workspace log', value: 'Audit' },
      { label: 'Telegram sync', value: 'On' },
    ],
    trust: [
      { label: 'Signal loop', value: 'Lead thật → Gate → Voice → Action' },
      { label: 'Nguồn dữ liệu', value: 'Facebook + Data Private + Google Sheet' },
      { label: 'Phân quyền', value: 'Org-scoped · Role-based · Audit-ready' },
    ],
    workflowTitle: 'Từ prompt đến lead đã lọc và hành động đúng ngữ cảnh',
    workflowBody:
      'Workspace không quét tràn lan. Mỗi chiến dịch đi qua hồ sơ doanh nghiệp, Market Signal Gate, trạng thái hội thoại và browser log để sales team luôn nhìn rõ hệ thống đang làm gì.',
    workflowSteps: [
      {
        title: '01 · Định vị doanh nghiệp',
        body: 'Brand, offer, phân khúc khách hàng, quy tắc loại trừ và dữ liệu riêng được lưu theo từng organization.',
      },
      {
        title: '02 · Crawl đúng nguồn mục tiêu',
        body: 'Agent nhận prompt, mở đúng group/page/truy vấn và đọc bài post thật từ phiên Facebook đã đăng nhập.',
      },
      {
        title: '03 · Lọc tín hiệu',
        body: 'Market Signal Gate giữ lại bài có nhu cầu thật, loại bài quảng cáo, spam và provider không đúng tệp.',
      },
      {
        title: '04 · Đồng bộ dashboard và Telegram',
        body: 'Lead, action log, comment, inbox, posting và kết quả crawl được đồng bộ để toàn đội cùng quan sát.',
      },
    ],
    surfacesTitle: 'Mọi bề mặt vận hành nằm trong một workspace duy nhất',
    surfaces: [
      {
        icon: Target,
        title: 'Lead thật',
        body: 'Lead được chấm điểm, gắn trạng thái hot · warm · cold và luôn lưu nguồn bài post để sales kiểm tra lại tức thì.',
      },
      {
        icon: Workflow,
        title: 'Browser có thể quan sát',
        body: 'Thao tác diễn ra trên Chrome thật, workspace vẫn ghi nhận stream, trạng thái và action log liên tục.',
      },
      {
        icon: MessagesSquare,
        title: 'Sales Voice Memory',
        body: 'Hệ thống học ngữ điệu của doanh nghiệp để comment, inbox và posting nghe đúng giọng team — không như bot.',
      },
      {
        icon: Database,
        title: 'Data Private',
        body: 'File, bảng giá, script sales, ngữ cảnh ngành và connector chỉ đọc trở thành ký ức để agent truy xuất khi cần.',
      },
    ],
    securityTitle: 'Vận hành trên browser thật, vẫn giữ nguyên ranh giới enterprise',
    securityBody:
      'THG không nhận mật khẩu Facebook. Workspace chỉ lưu thông tin ghép nối, định danh tối thiểu, log hành động và phân quyền theo organization.',
    securityItems: [
      'Mọi bản ghi tenant-facing đều scoped theo organization.',
      'Chrome Extension chỉ kết nối tab Facebook đã được người dùng chấp thuận.',
      'Khi Facebook yêu cầu checkpoint hoặc xác minh, hệ thống dừng và chờ thao tác con người.',
      'Telegram và dashboard cùng nhận một bản tóm tắt hành động — toàn đội không mất ngữ cảnh.',
    ],
    finalTitle: 'Đây không phải scraper dùng một lần. Đây là workspace để sales vận hành Facebook liên tục.',
    finalBody:
      'Tạo workspace, kết nối browser Facebook thật, nạp Data Private và để agent làm việc trên đúng tệp khách hàng của doanh nghiệp bạn.',
    finalPrimary: 'Tạo workspace ngay',
    finalSecondary: 'Admin portal',
    footer: 'THG AutoFlow',
    footerCopy: 'Facebook sales intelligence workspace cho đội vận hành thật.',
  },
  en: {
    navFeatures: 'Features',
    navFlow: 'Workflow',
    navSecurity: 'Security',
    login: 'Sign in',
    register: 'Create workspace',
    heroKicker: 'Facebook Sales Intelligence Workspace',
    heroTitle: 'Facebook automation only works when the system understands the business first.',
    heroBody:
      'THG AutoFlow connects the real Facebook browser, business profile, private data, lead signals, and sales voice memory into one workspace so AI agents do not crawl blindly or act without context.',
    heroPrimary: 'Start with a workspace',
    heroSecondary: 'Open dashboard',
    heroNote:
      'The extension lives inside the employee Chrome profile. The dashboard receives stream frames, lead signals, action logs, and automation results in real time.',
    heroStats: [
      { label: 'Browser stream', value: 'Live' },
      { label: 'Market gate', value: '24/7' },
      { label: 'Workspace log', value: 'Audit' },
      { label: 'Telegram sync', value: 'On' },
    ],
    trust: [
      { label: 'Signal loop', value: 'Real leads -> Gate -> Voice -> Action' },
      { label: 'Data sources', value: 'Facebook + Data Private + Google Sheets' },
      { label: 'Security', value: 'Org-scoped, role-based, audit-ready' },
    ],
    workflowTitle: 'From prompt to filtered lead and context-aware action',
    workflowBody:
      'The workspace does not scan at random. Every campaign passes through business profile calibration, market signal gating, conversation state, and browser logging so the sales team can see exactly what the system is doing.',
    workflowSteps: [
      {
        title: '1. Business calibration',
        body: 'Brand, offer, target customer, reject rules, and private files are stored per organization.',
      },
      {
        title: '2. Targeted crawl',
        body: 'The agent receives a prompt, opens the right group, page, or query, and reads real posts from the logged-in Facebook session.',
      },
      {
        title: '3. Signal filtering',
        body: 'Market Signal Gate keeps true buyer intent and rejects service ads, spam, and provider noise outside the target segment.',
      },
      {
        title: '4. Dashboard and Telegram sync',
        body: 'Leads, action logs, comments, inbox actions, posting, and crawl results are synchronized for the team to review.',
      },
    ],
    surfacesTitle: 'The operating surfaces your team uses inside one workspace',
    surfaces: [
      {
        icon: Target,
        title: 'Real leads',
        body: 'Leads are scored, grouped by hot, warm, and cold, and always keep the original post source for review.',
      },
      {
        icon: Workflow,
        title: 'Observable browser',
        body: 'Actions still happen on the real Chrome profile, while the workspace keeps the browser stream, status, and action log visible.',
      },
      {
        icon: MessagesSquare,
        title: 'Sales Voice Memory',
        body: 'The system learns the team voice so comments, inbox replies, and posting sound like the business instead of a generic bot.',
      },
      {
        icon: Database,
        title: 'Data Private',
        body: 'Files, pricing tables, sales scripts, market context, and read-only connectors become retrievable memory for the agents.',
      },
    ],
    securityTitle: 'Real browser execution with enterprise boundaries',
    securityBody:
      'THG does not take Facebook passwords. The workspace stores only the pairing, the minimum identity state, action logs, and organization-scoped permissions.',
    securityItems: [
      'Every tenant-facing record is scoped to an organization.',
      'The Chrome Extension only connects user-approved Facebook tabs.',
      'If Facebook requires checkpoint or verification, the system stops for human action.',
      'Telegram and dashboard receive the same action summaries so the team never loses context.',
    ],
    finalTitle: 'This is not a one-off scraper. This is the workspace your sales team uses to run Facebook continuously.',
    finalBody:
      'Create the workspace, connect the real Facebook browser, load Data Private, and let the agents start working on the right customer segment for your business.',
    finalPrimary: 'Create workspace now',
    finalSecondary: 'Admin portal',
    footer: 'THG AutoFlow',
    footerCopy: 'Facebook sales intelligence workspace for real operating teams.',
  },
} as const;

const VALUE_PILLS = [
  'Market Signal Gate',
  'Sales Voice Memory',
  'Browser stream',
  'Telegram log',
];

export default function Landing({ onLogin, onRegister, onAdmin }: LandingProps) {
  const { lang } = useLang();
  const copy = COPY[lang];

  return (
    <main className={styles.page}>
      <div className={styles.backdrop} aria-hidden="true" />

      <header className={styles.navWrap}>
        <div className={styles.nav}>
          <div className={styles.brand}>
            <div className={styles.brandMark}>
              <img src="/assets/thg-pegasus.png" alt="THG" style={{ width: 28, height: 28, objectFit: 'contain' }} />
            </div>
            <div>
              <strong>THG AutoFlow</strong>
              <span>Workspace</span>
            </div>
          </div>

          <nav className={styles.navLinks} aria-label="Landing navigation">
            <a href="#features">{copy.navFeatures}</a>
            <a href="#workflow">{copy.navFlow}</a>
            <a href="#security">{copy.navSecurity}</a>
          </nav>

          <div className={styles.navActions}>
            <LangSwitch />
            <button type="button" className="btn btn-ghost" onClick={onLogin}>
              {copy.login}
            </button>
            <button type="button" className="btn btn-primary" onClick={onRegister}>
              {copy.register}
            </button>
          </div>
        </div>
      </header>

      <section className={styles.hero}>
        <div className={styles.heroCopy}>
          <p className="eyebrow">
            <span className="dot" />
            {copy.heroKicker}
          </p>
          <h1 className={styles.heroTitle}>{copy.heroTitle}</h1>
          <p className={styles.heroBody}>{copy.heroBody}</p>

          <div className={styles.heroActions}>
            <button type="button" className="btn btn-primary btn-lg" onClick={onRegister}>
              {copy.heroPrimary}
              <ArrowRight size={15} />
            </button>
            <button type="button" className="btn btn-ghost btn-lg" onClick={onLogin}>
              {copy.heroSecondary}
            </button>
          </div>

          <p className={styles.heroNote}>{copy.heroNote}</p>

          <div className={styles.valuePills}>
            {VALUE_PILLS.map((pill) => (
              <span key={pill}>{pill}</span>
            ))}
          </div>
        </div>

        <div className={styles.heroSurface} aria-hidden="true">
          <div className={styles.windowBar}>
            <div className={styles.windowDots}>
              <span />
              <span />
              <span />
            </div>
            <p>THG Browser Workspace</p>
          </div>

          <div className={styles.surfaceBody}>
            <aside className={styles.surfaceRail}>
              {['Leads', 'Chat', 'Browser', 'Inbox', 'Posting'].map((item, idx) => (
                <div key={item} className={idx === 2 ? styles.railActive : undefined}>
                  {item}
                </div>
              ))}
            </aside>

            <div className={styles.surfaceMain}>
              <div className={styles.surfaceMetrics}>
                {copy.heroStats.map((item) => (
                  <div key={item.label}>
                    <small>{item.label}</small>
                    <strong>{item.value}</strong>
                  </div>
                ))}
              </div>

              <div className={styles.signalBoard}>
                <div className={styles.signalHeader}>
                  <div>
                    <p className={styles.signalTitle}>Facebook Copilot</p>
                    <span>Workspace-synced execution log</span>
                  </div>
                  <span className={styles.signalBadge}>Extension online</span>
                </div>

                <div className={styles.signalStream}>
                  <article>
                    <small>Prompt</small>
                    <p>Find POD and dropship buyers looking for fulfillment from Vietnam or China.</p>
                  </article>
                  <article>
                    <small>Signal Gate</small>
                    <p>14 posts fetched, 5 buyer-intent leads kept, 9 provider or spam posts rejected.</p>
                  </article>
                  <article>
                    <small>Action log</small>
                    <p>Browser opened the target Facebook group, crawler stored leads, Telegram summary sent.</p>
                  </article>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section className={styles.trustBand}>
        {copy.trust.map((item) => (
          <div key={item.label}>
            <small>{item.label}</small>
            <strong>{item.value}</strong>
          </div>
        ))}
      </section>

      <section id="workflow" className={styles.section}>
        <div className={styles.sectionIntro}>
          <p className="eyebrow">
            <span className="dot" />
            Workflow
          </p>
          <h2>{copy.workflowTitle}</h2>
          <p>{copy.workflowBody}</p>
        </div>

        <div className={styles.stepGrid}>
          {copy.workflowSteps.map((step) => (
            <article key={step.title} className={styles.stepCard}>
              <h3>{step.title}</h3>
              <p>{step.body}</p>
            </article>
          ))}
        </div>
      </section>

      <section id="features" className={styles.section}>
        <div className={styles.sectionIntro}>
          <p className="eyebrow">
            <span className="dot" />
            Workspace Surfaces
          </p>
          <h2>{copy.surfacesTitle}</h2>
        </div>

        <div className={styles.featureGrid}>
          {copy.surfaces.map(({ icon: Icon, title, body }) => (
            <article key={title} className={styles.featureCard}>
              <div className={styles.featureIcon}>
                <Icon size={16} />
              </div>
              <h3>{title}</h3>
              <p>{body}</p>
            </article>
          ))}
        </div>
      </section>

      <section id="security" className={styles.section}>
        <div className={styles.securityBand}>
          <div className={styles.securityCopy}>
            <p className="eyebrow">
              <span className="dot" />
              Security
            </p>
            <h2>{copy.securityTitle}</h2>
            <p>{copy.securityBody}</p>
          </div>

          <div className={styles.securityList}>
            {copy.securityItems.map((item) => (
              <div key={item} className={styles.securityItem}>
                <ShieldCheck size={15} />
                <span>{item}</span>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className={styles.finalBand}>
        <div>
          <p className="eyebrow">
            <span className="dot" />
            Launch
          </p>
          <h2>{copy.finalTitle}</h2>
          <p>{copy.finalBody}</p>
        </div>

        <div className={styles.finalActions}>
          <button type="button" className="btn btn-primary btn-lg" onClick={onRegister}>
            {copy.finalPrimary}
          </button>
          <button type="button" className="btn btn-ghost btn-lg" onClick={onAdmin}>
            {copy.finalSecondary}
          </button>
        </div>
      </section>

      <footer className={styles.footer}>
        <div className={styles.footerBrand}>
          <div className={styles.brandMark} style={{ width: 32, height: 32, borderRadius: 10 }}>
            <img src="/assets/thg-pegasus.png" alt="THG" style={{ width: 20, height: 20, objectFit: 'contain' }} />
          </div>
          <div>
            <strong>{copy.footer}</strong>
            <span>{copy.footerCopy}</span>
          </div>
        </div>
        <button type="button" className={styles.footerLink} onClick={onLogin}>
          {copy.login}
        </button>
      </footer>
    </main>
  );
}
