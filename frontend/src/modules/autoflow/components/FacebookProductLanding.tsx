'use client';

import {
  ArrowRight,
  Check,
  Database,
  MessagesSquare,
  ShieldCheck,
  Target,
  Workflow,
} from 'lucide-react';

import { useLang } from '../i18n/useLang';
import MarketingNav from '../../../marketing/MarketingNav';
import styles from './facebookProduct.module.css';

interface FacebookProductLandingProps {
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

    heroKicker: 'Workspace bán hàng thông minh trên Facebook',
    heroTitlePre: 'Tự động hoá Facebook chỉ hiệu quả khi hệ thống ',
    heroTitleGrad: 'thật sự hiểu doanh nghiệp',
    heroTitlePost: ' của bạn.',
    heroBody:
      'THG đồng bộ trình duyệt Facebook thật, hồ sơ doanh nghiệp, dữ liệu nội bộ, tín hiệu thị trường và giọng văn bán hàng vào một workspace duy nhất — để AI không quét mù và không hành động vô nghĩa.',
    heroPrimary: 'Tạo workspace ngay',
    heroSecondary: 'Vào bảng điều khiển',
    heroNote:
      'Tiện ích cài trên Chrome của từng nhân viên. Bảng điều khiển nhận luồng trình duyệt, tín hiệu khách hàng, nhật ký hành động và kết quả tự động hoá theo thời gian thực.',
    pills: ['Bộ lọc tín hiệu', 'Ghi nhớ giọng bán hàng', 'Luồng trình duyệt trực tiếp', 'Tóm tắt qua Telegram'],

    termTitle: 'THG · Trình duyệt workspace',
    termLive: 'Trực tiếp',
    metrics: [
      { label: 'Luồng trình duyệt', value: 'Trực tiếp', tone: 'cy' },
      { label: 'Bộ lọc thị trường', value: '24/7', tone: 'li' },
      { label: 'Nhật ký workspace', value: 'Kiểm toán', tone: '' },
      { label: 'Đồng bộ Telegram', value: 'Bật', tone: 'cy' },
    ],
    copilotTitle: 'Trợ lý Facebook',
    copilotSub: 'Nhật ký vận hành đồng bộ workspace',
    online: 'Tiện ích đang kết nối',
    logs: [
      { tag: 'Yêu cầu', cls: 't-prompt', text: 'Tìm khách cần mua hộ / fulfillment hàng từ Việt Nam và Trung Quốc.' },
      { tag: 'Bộ lọc', cls: 't-gate', text: 'Đã quét 14 bài, giữ 5 khách có nhu cầu thật, loại 9 bài quảng cáo và spam.' },
      { tag: 'Hành động', cls: 't-action', text: 'Trình duyệt mở đúng nhóm Facebook, lưu khách tiềm năng, gửi tóm tắt qua Telegram.' },
    ],

    trust: [
      { label: 'Vòng tín hiệu', value: 'Khách thật → Lọc → Giọng bán hàng → Hành động' },
      { label: 'Nguồn dữ liệu', value: 'Facebook + Dữ liệu riêng + Google Sheet' },
      { label: 'Phân quyền', value: 'Theo tổ chức · Theo vai trò · Sẵn sàng kiểm toán' },
    ],

    flowEyebrow: 'Quy trình',
    workflowTitle: 'Từ yêu cầu đến khách đã lọc và hành động đúng ngữ cảnh',
    workflowBody:
      'Workspace không quét tràn lan. Mỗi chiến dịch đi qua hồ sơ doanh nghiệp, bộ lọc tín hiệu thị trường, trạng thái hội thoại và nhật ký trình duyệt — để đội bán hàng luôn thấy rõ hệ thống đang làm gì.',
    workflowSteps: [
      { n: '01', title: 'Định vị doanh nghiệp', body: 'Thương hiệu, sản phẩm, phân khúc khách, quy tắc loại trừ và dữ liệu riêng được lưu theo từng tổ chức.' },
      { n: '02', title: 'Quét đúng nguồn mục tiêu', body: 'AI nhận yêu cầu, mở đúng nhóm / trang / truy vấn và đọc bài đăng thật từ phiên Facebook đã đăng nhập.' },
      { n: '03', title: 'Lọc tín hiệu mua hàng', body: 'Bộ lọc giữ lại bài có nhu cầu thật, loại bài quảng cáo, spam và nhà cung cấp không đúng tệp.' },
      { n: '04', title: 'Đồng bộ bảng điều khiển & Telegram', body: 'Khách hàng, nhật ký hành động, bình luận, tin nhắn và kết quả quét được đồng bộ để cả đội cùng theo dõi.' },
    ],

    featEyebrow: 'Bề mặt vận hành',
    surfacesTitle: 'Mọi công cụ vận hành nằm trong một workspace duy nhất',
    surfaces: [
      { icon: Target, title: 'Khách hàng thật', body: 'Khách được chấm điểm, gắn trạng thái nóng · ấm · lạnh và luôn lưu nguồn bài đăng để kiểm tra lại tức thì.' },
      { icon: Workflow, title: 'Trình duyệt minh bạch', body: 'Thao tác diễn ra trên Chrome thật, workspace vẫn ghi nhận luồng, trạng thái và nhật ký hành động liên tục.' },
      { icon: MessagesSquare, title: 'Ghi nhớ giọng bán hàng', body: 'Hệ thống học giọng văn của doanh nghiệp để bình luận, nhắn tin và đăng bài nghe đúng chất đội bạn — không như bot.' },
      { icon: Database, title: 'Dữ liệu riêng', body: 'Tệp, bảng giá, kịch bản bán hàng, ngữ cảnh ngành và kết nối chỉ đọc trở thành ký ức để AI truy xuất khi cần.' },
    ],

    secEyebrow: 'Bảo mật',
    securityTitle: 'Chạy trên trình duyệt thật, vẫn giữ nguyên ranh giới doanh nghiệp',
    securityBody:
      'THG không nhận mật khẩu Facebook. Workspace chỉ lưu thông tin ghép nối, định danh tối thiểu, nhật ký hành động và phân quyền theo tổ chức.',
    securityItems: [
      'Mọi dữ liệu của khách thuê đều giới hạn theo từng tổ chức.',
      'Tiện ích Chrome chỉ kết nối tab Facebook mà người dùng đã chấp thuận.',
      'Khi Facebook yêu cầu xác minh, hệ thống dừng lại và chờ thao tác của con người.',
      'Telegram và bảng điều khiển cùng nhận một bản tóm tắt — cả đội không mất ngữ cảnh.',
    ],

    finalEyebrow: 'Khởi chạy',
    finalTitle: 'Đây không phải công cụ quét dùng một lần. Đây là workspace để đội bạn vận hành Facebook liên tục.',
    finalBody:
      'Tạo workspace, kết nối trình duyệt Facebook thật, nạp dữ liệu riêng và để AI làm việc trên đúng tệp khách hàng của doanh nghiệp bạn.',
    finalPrimary: 'Tạo workspace ngay',
    finalSecondary: 'Cổng quản trị',
    footer: 'THG · Facebook Automation',
    footerCopy: 'Workspace bán hàng thông minh trên Facebook cho đội vận hành thật.',
  },

  en: {
    navFeatures: 'Features',
    navFlow: 'Workflow',
    navSecurity: 'Security',
    login: 'Sign in',

    heroKicker: 'Facebook sales intelligence workspace',
    heroTitlePre: 'Facebook automation only works when the system ',
    heroTitleGrad: 'truly understands your business',
    heroTitlePost: '.',
    heroBody:
      'THG fuses the real Facebook browser, business profile, private data, market signals, and sales voice into one workspace — so AI never crawls blindly or acts without context.',
    heroPrimary: 'Create a workspace',
    heroSecondary: 'Open dashboard',
    heroNote:
      'The extension lives in each employee’s Chrome. The dashboard receives the browser stream, lead signals, action logs, and automation results in real time.',
    pills: ['Signal gate', 'Sales voice memory', 'Live browser stream', 'Telegram summary'],

    termTitle: 'THG · Browser workspace',
    termLive: 'Live',
    metrics: [
      { label: 'Browser stream', value: 'Live', tone: 'cy' },
      { label: 'Market gate', value: '24/7', tone: 'li' },
      { label: 'Workspace log', value: 'Audit', tone: '' },
      { label: 'Telegram sync', value: 'On', tone: 'cy' },
    ],
    copilotTitle: 'Facebook copilot',
    copilotSub: 'Workspace-synced execution log',
    online: 'Extension online',
    logs: [
      { tag: 'Prompt', cls: 't-prompt', text: 'Find buyers needing fulfillment / buy-for service from Vietnam or China.' },
      { tag: 'Gate', cls: 't-gate', text: '14 posts fetched, 5 buyer-intent leads kept, 9 ad/spam posts rejected.' },
      { tag: 'Action', cls: 't-action', text: 'Browser opened the target Facebook group, stored leads, sent a Telegram summary.' },
    ],

    trust: [
      { label: 'Signal loop', value: 'Real leads → Gate → Voice → Action' },
      { label: 'Data sources', value: 'Facebook + Private data + Google Sheets' },
      { label: 'Permissions', value: 'Org-scoped · Role-based · Audit-ready' },
    ],

    flowEyebrow: 'Workflow',
    workflowTitle: 'From prompt to filtered lead and context-aware action',
    workflowBody:
      'The workspace does not scan at random. Every campaign passes through the business profile, market signal gate, conversation state, and browser log so the sales team always sees what the system is doing.',
    workflowSteps: [
      { n: '01', title: 'Business calibration', body: 'Brand, offer, target segment, reject rules, and private files are stored per organization.' },
      { n: '02', title: 'Targeted crawl', body: 'The agent takes a prompt, opens the right group / page / query, and reads real posts from the logged-in Facebook session.' },
      { n: '03', title: 'Signal filtering', body: 'The gate keeps true buyer intent and rejects ads, spam, and providers outside the segment.' },
      { n: '04', title: 'Dashboard & Telegram sync', body: 'Leads, action logs, comments, inbox, and crawl results sync so the whole team can follow along.' },
    ],

    featEyebrow: 'Operating surfaces',
    surfacesTitle: 'Every operating surface inside one workspace',
    surfaces: [
      { icon: Target, title: 'Real leads', body: 'Leads are scored, tagged hot · warm · cold, and always keep the source post for instant review.' },
      { icon: Workflow, title: 'Observable browser', body: 'Actions run on real Chrome while the workspace keeps the stream, status, and action log visible.' },
      { icon: MessagesSquare, title: 'Sales voice memory', body: 'The system learns your business voice so comments, inbox, and posts sound like your team — not a bot.' },
      { icon: Database, title: 'Private data', body: 'Files, price lists, sales scripts, industry context, and read-only connectors become memory the AI can retrieve.' },
    ],

    secEyebrow: 'Security',
    securityTitle: 'Real browser execution, enterprise boundaries intact',
    securityBody:
      'THG never takes your Facebook password. The workspace stores only the pairing, minimal identity, action logs, and organization-scoped permissions.',
    securityItems: [
      'Every tenant-facing record is scoped to an organization.',
      'The Chrome extension only connects Facebook tabs the user has approved.',
      'When Facebook asks for verification, the system stops and waits for a human.',
      'Telegram and the dashboard receive the same summary — the team never loses context.',
    ],

    finalEyebrow: 'Launch',
    finalTitle: 'This is not a one-off scraper. It is the workspace your team uses to run Facebook continuously.',
    finalBody:
      'Create the workspace, connect the real Facebook browser, load your private data, and let the AI work on exactly your customer segment.',
    finalPrimary: 'Create a workspace',
    finalSecondary: 'Admin portal',
    footer: 'THG · Facebook Automation',
    footerCopy: 'Facebook sales intelligence workspace for real operating teams.',
  },
} as const;

export default function FacebookProductLanding({ onLogin, onRegister, onAdmin }: Readonly<FacebookProductLandingProps>) {
  const { lang } = useLang();
  const copy = COPY[lang];

  return (
    <main className={styles.page}>
      <div className={styles.backdrop} aria-hidden="true" />

      <MarketingNav
        onLogin={onLogin}
        onRegister={onRegister}
        currentServiceSlug="facebook-automation"
        sectionLinks={[
          { href: '#features', label: copy.navFeatures },
          { href: '#workflow', label: copy.navFlow },
          { href: '#security', label: copy.navSecurity },
        ]}
      />

      {/* ===================== HERO ===================== */}
      <section className={styles.hero}>
        <div className={styles.heroCopy}>
          <span className={`${styles.kicker} ${styles.reveal} ${styles.d1}`}>
            <span className={styles.live} />
            {copy.heroKicker}
          </span>
          <h1 className={`${styles.title} ${styles.reveal} ${styles.d2}`}>
            {copy.heroTitlePre}
            <span className={styles.grad}>{copy.heroTitleGrad}</span>
            {copy.heroTitlePost}
          </h1>
          <p className={`${styles.body} ${styles.reveal} ${styles.d3}`}>{copy.heroBody}</p>

          <div className={`${styles.actions} ${styles.reveal} ${styles.d4}`}>
            <button type="button" className={styles.btnPrimary} onClick={onRegister}>
              {copy.heroPrimary}
              <ArrowRight size={16} />
            </button>
            <button type="button" className={styles.btnGhost} onClick={onLogin}>
              {copy.heroSecondary}
            </button>
          </div>

          <p className={`${styles.note} ${styles.reveal} ${styles.d5}`}>{copy.heroNote}</p>

          <div className={`${styles.pills} ${styles.reveal} ${styles.d5}`}>
            {copy.pills.map((pill) => (
              <span key={pill}>{pill}</span>
            ))}
          </div>
        </div>

        {/* live operations terminal (hand-built, no AI imagery) */}
        <div className={styles.terminal} aria-hidden="true">
          <div className={styles.termBar}>
            <span className={styles.termDots}><i /><i /><i /></span>
            <span className={styles.termTitle}>{copy.termTitle}</span>
            <span className={styles.termLive}><i />{copy.termLive}</span>
          </div>

          <div className={styles.termBody}>
            <div className={styles.metrics}>
              {copy.metrics.map((m) => (
                <div key={m.label} className={styles.metric}>
                  <small>{m.label}</small>
                  <b className={m.tone ? styles[m.tone as 'cy' | 'li'] : ''}>{m.value}</b>
                </div>
              ))}
            </div>

            <div className={styles.console}>
              <div className={styles.consoleHead}>
                <div>
                  <b>{copy.copilotTitle}</b>
                  <small>{copy.copilotSub}</small>
                </div>
                <span className={styles.tagOnline}><i />{copy.online}</span>
              </div>
              <div className={styles.stream}>
                {copy.logs.map((log, i) => (
                  <div key={log.tag} className={styles.logLine}>
                    <span className={`${styles.logTag} ${styles[log.cls as 't-prompt']}`}>{log.tag}</span>
                    <p className={styles.logText}>
                      {log.text}
                      {i === copy.logs.length - 1 && <span className={styles.caret} />}
                    </p>
                  </div>
                ))}
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* ===================== TRUST ===================== */}
      <section className={styles.trust}>
        {copy.trust.map((item) => (
          <div key={item.label}>
            <small>{item.label}</small>
            <strong>{item.value}</strong>
          </div>
        ))}
      </section>

      {/* ===================== WORKFLOW ===================== */}
      <section id="workflow" className={styles.section}>
        <div className={styles.sectionHead}>
          <p className={styles.eyebrow}>{copy.flowEyebrow}</p>
          <h2 className={styles.h2}>{copy.workflowTitle}</h2>
          <p className={styles.lead}>{copy.workflowBody}</p>
        </div>

        <div className={styles.pipeline}>
          {copy.workflowSteps.map((step) => (
            <div key={step.n} className={styles.step}>
              <span className={styles.stepNode}>{step.n}</span>
              <div className={styles.stepCard}>
                <h3>{step.title}</h3>
                <p>{step.body}</p>
              </div>
            </div>
          ))}
        </div>
      </section>

      {/* ===================== SURFACES ===================== */}
      <section id="features" className={styles.section}>
        <div className={styles.sectionHead}>
          <p className={styles.eyebrow}>{copy.featEyebrow}</p>
          <h2 className={styles.h2}>{copy.surfacesTitle}</h2>
        </div>

        <div className={styles.featGrid}>
          {copy.surfaces.map(({ icon: Icon, title, body }) => (
            <article key={title} className={styles.featCard}>
              <div className={styles.featIcon}>
                <Icon size={18} />
              </div>
              <h3>{title}</h3>
              <p>{body}</p>
            </article>
          ))}
        </div>
      </section>

      {/* ===================== SECURITY ===================== */}
      <section id="security" className={styles.section}>
        <div className={styles.secBand}>
          <div className={styles.secCopy}>
            <p className={styles.eyebrow}>{copy.secEyebrow}</p>
            <h2 className={styles.h2}>{copy.securityTitle}</h2>
            <p>{copy.securityBody}</p>
          </div>

          <div className={styles.secList}>
            {copy.securityItems.map((item) => (
              <div key={item} className={styles.secItem}>
                <span className={styles.ico}><Check size={14} /></span>
                <span>{item}</span>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* ===================== FINAL CTA ===================== */}
      <section>
        <div className={styles.final}>
          <div>
            <p className={styles.eyebrow}>{copy.finalEyebrow}</p>
            <h2>{copy.finalTitle}</h2>
            <p>{copy.finalBody}</p>
          </div>
          <div className={styles.finalActions}>
            <button type="button" className={styles.btnPrimary} onClick={onRegister}>
              {copy.finalPrimary}
              <ArrowRight size={16} />
            </button>
            <button type="button" className={styles.btnGhost} onClick={onAdmin}>
              {copy.finalSecondary}
            </button>
          </div>
        </div>
      </section>

      {/* ===================== FOOTER ===================== */}
      <footer className={styles.footer}>
        <div className={styles.footerBrand}>
          <span className={styles.footMark}>
            <img src="/assets/thg-pegasus.png" alt="THG" />
          </span>
          <div>
            <strong>{copy.footer}</strong>
            <span>{copy.footerCopy}</span>
          </div>
        </div>
        <button type="button" className={styles.footLink} onClick={onLogin}>
          {copy.login}
        </button>
      </footer>
    </main>
  );
}
