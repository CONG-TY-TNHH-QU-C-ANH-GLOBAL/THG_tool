'use client';

import { useEffect, useRef, useState } from 'react';
import Link from 'next/link';
import {
  ArrowRight,
  Building2,
  Check,
  ChevronDown,
  Eye,
  Globe,
  Search,
  Send,
  ShieldCheck,
  Sparkles,
} from 'lucide-react';
import { useLang } from '../modules/autoflow/i18n/useLang';
import { LangSwitch } from '../modules/autoflow/components/ds/LangSwitch';
import { MARKETING_SERVICES } from './MarketingNav';
import styles from './platformLanding.module.css';

// Public home page for THG — a premium B2B SaaS marketing surface aimed at
// Vietnamese sales-business owners. Self-contained light theme (its own CSS
// module) so it does not disturb the dark /products + coming-soon pages that
// share landing.module.css. Per CLAUDE.md: no AI imagery — every "screenshot"
// is a hand-built HTML/CSS/SVG mock.

interface PlatformLandingProps {
  onLogin: () => void;
  onRegister: () => void;
}

const SERVICE_ICON: Record<string, typeof Send> = {
  'facebook-automation': Send,
  'taobao-sourcing': Search,
  'alibaba-1688': Globe,
};

const COPY = {
  vi: {
    navServices: 'Giải pháp',
    navWhy: 'Vì sao chọn THG',
    navLaunch: 'Bắt đầu',
    navLogin: 'Đăng nhập',
    navCta: 'Khởi tạo Workspace',

    heroEyebrow: 'Đang vận hành · Facebook Automation',
    heroTitle: ['THG · Hệ điều hành ', 'bán hàng tự động', ' đa kênh cho doanh nghiệp'],
    heroSub:
      'Quản lý và vận hành toàn bộ đội ngũ sales trên Facebook, Taobao, 1688 tập trung tại một workspace duy nhất. Tự động hóa thông minh bằng trình duyệt thật — an toàn tuyệt đối, không dùng bot ảo.',
    heroPrimary: 'Khởi tạo Workspace miễn phí',
    heroSecondary: 'Liên hệ tư vấn giải pháp',
    fine: [
      'Cài đặt trong 2 phút',
      'Không tốn tài nguyên máy',
      'Phân quyền theo từng nhân viên',
    ],

    proof: [
      { v: '1', l: 'Workspace cho cả đội — dùng chung một nguồn khách hàng' },
      { v: '3', l: 'Kênh thương mại: Facebook · Taobao · 1688' },
      { v: '100%', l: 'Trình duyệt thật, hành vi như người dùng thật' },
      { v: '24/7', l: 'Giám sát đội sales theo thời gian thực' },
    ],

    dash: {
      title: 'THG Workspace · Phòng điều hành sales',
      live: 'Trực tuyến',
      kpis: [
        { l: 'Khách tiềm năng hôm nay', v: '128', d: '+19%' },
        { l: 'Đang được chăm sóc', v: '34', d: 'live' },
        { l: 'Phản hồi TB', v: '2.4’', d: '−38%' },
      ],
      teamTitle: 'Đội ngũ đang hoạt động',
      teamMeta: '5 nhân viên',
      team: [
        { n: 'Lan', who: 'Lan · đang nhắn tin với khách', g: 'linear-gradient(140deg,#6366f1,#4338ca)', tag: 'Đang chat', cls: 'tagLive' },
        { n: 'Minh', who: 'Minh · vừa trả lời bình luận', g: 'linear-gradient(140deg,#0ea5e9,#0369a1)', tag: '4’ trước', cls: 'tagWait' },
        { n: 'Hà', who: 'Hà · chốt đơn khách nóng', g: 'linear-gradient(140deg,#f43f5e,#be123c)', tag: 'Khách nóng', cls: 'tagHot' },
      ],
      speedLabel: 'Tốc độ phản hồi · 7 ngày',
      capLabel: 'Hạn mức an toàn hôm nay',
      capValue: '72',
      capUnit: '/ 120 thao tác',
      capNote: 'Tự điều phối để bảo vệ tài khoản',
      chip: { b: 'Tài khoản an toàn', s: 'Hành vi mô phỏng người thật' },
    },

    solEyebrow: 'Ba mảng giải pháp',
    solTitle: 'Tự động hóa từng kênh — không phải công cụ scrape rời rạc.',
    solLead:
      'Một workspace bật được nhiều giải pháp. Dữ liệu khách hàng và quy trình dùng chung; mỗi kênh có cách vận hành riêng. Bạn cần kênh nào, kích hoạt kênh đó.',
    solCta: 'Xem chi tiết',
    solSoonCta: 'Đăng ký nhận thông báo',
    statusLive: 'Đang hoạt động',
    statusSoon: 'Sắp ra mắt',
    solutions: {
      'facebook-automation': {
        name: 'Facebook Automation',
        tag: 'Tự động tìm & chăm khách trên Facebook',
        body: 'Tự động quét nhóm tìm khách hàng tiềm năng, lọc đúng tín hiệu mua hàng.',
        points: [
          'Quét nhóm, lọc tín hiệu mua hàng bằng AI',
          'Trả lời bình luận & nhắn tin theo văn phong doanh nghiệp',
          'Đăng bài, tương tác đúng nhịp người thật',
        ],
      },
      'taobao-sourcing': {
        name: 'Taobao Sourcing',
        tag: 'Theo dõi nguồn hàng & đối thủ trên Taobao',
        body: 'Tự động theo dõi đối thủ, giám sát nhà cung cấp và nối thẳng quy trình mua hộ.',
        points: [
          'Theo dõi seller & đối thủ theo danh mục',
          'Giám sát biến động giá và sản phẩm mới',
          'Kết nối thẳng quy trình mua hộ / dropship',
        ],
      },
      'alibaba-1688': {
        name: '1688 Sourcing',
        tag: 'Phát hiện nguồn hàng giá tốt trên 1688',
        body: 'Tự động phát hiện nguồn hàng giá tốt, kiểm tra số lượng mua tối thiểu (MOQ).',
        points: [
          'Phát hiện nhà cung cấp & mức giá tốt nhất',
          'Đối chiếu số lượng mua tối thiểu (MOQ)',
          'Đồng bộ dữ liệu cho cả đội mua hàng và sales',
        ],
      },
    } as Record<string, { name: string; tag: string; body: string; points: string[] }>,

    whyEyebrow: 'Vì sao chọn THG',
    whyTitle: 'Được xây cho chủ doanh nghiệp muốn đội sales vận hành trơn tru.',
    why: [
      {
        icon: Building2,
        title: 'Quản lý tập trung, tiết kiệm tài nguyên',
        body: 'Cả đội dùng chung một nguồn dữ liệu khách hàng. Không cần cài đặt phức tạp, không tốn tài nguyên máy tính của từng nhân viên.',
        viz: 'hub',
      },
      {
        icon: ShieldCheck,
        title: 'An toàn & chống khóa tài khoản',
        body: 'Hệ thống tự điều phối hành vi như người thật, có hạn mức thao tác mỗi ngày để bảo vệ tối đa các tài khoản bán hàng của doanh nghiệp.',
        viz: 'shield',
      },
      {
        icon: Eye,
        title: 'Giám sát thời gian thực',
        body: 'Quản lý nhìn rõ ngay nhân viên nào đang chăm khách nào, tốc độ phản hồi ra sao — không cần báo cáo thủ công phức tạp.',
        viz: 'pulse',
      },
    ],

    ctaEyebrow: 'Bắt đầu',
    ctaTitle: 'Sẵn sàng số hóa quy trình bán hàng của bạn sau 2 phút.',
    ctaSub:
      'Tạo workspace, kích hoạt giải pháp Facebook và kết nối trình duyệt thật bằng một tiện ích cài một lần. Đơn giản, an toàn, sẵn sàng cho cả đội.',
    ctaPrimary: 'Khởi tạo Workspace miễn phí',
    ctaSecondary: 'Tôi đã có workspace',
    ctaContact: 'Cần tư vấn cho đội lớn? ',
    ctaContactLink: 'Liên hệ đội ngũ THG',
    steps: [
      { b: 'Tạo Workspace', s: 'Đăng ký tài khoản và lập không gian làm việc cho cả đội trong vài giây.' },
      { b: 'Kích hoạt giải pháp Facebook', s: 'Bật Facebook Automation và thiết lập văn phong, mục tiêu khách hàng.' },
      { b: 'Cài tiện ích mở rộng', s: 'Cài Extension một lần để kết nối trình duyệt thật và bắt đầu vận hành.' },
    ],

    footTagline: 'Hệ điều hành bán hàng tự động đa kênh cho doanh nghiệp Việt.',
    footProduct: 'Sản phẩm',
    footCompany: 'Công ty',
    footResources: 'Tài nguyên',
    footRights: 'THG Automation Platform. Bảo lưu mọi quyền.',
    footPrivacy: 'Quyền riêng tư',
    footExtension: 'Tiện ích mở rộng',
    footContact: 'Liên hệ',
    footAbout: 'Về chúng tôi',
    footStatus: 'Hệ thống ổn định',
    footLogin: 'Đăng nhập',
  },

  en: {
    navServices: 'Solutions',
    navWhy: 'Why THG',
    navLaunch: 'Get started',
    navLogin: 'Sign in',
    navCta: 'Create workspace',

    heroEyebrow: 'Live · Facebook Automation',
    heroTitle: ['THG · The multi-channel ', 'sales automation', ' OS for businesses'],
    heroSub:
      'Run your entire sales team across Facebook, Taobao, and 1688 from a single workspace. Smart automation driven by real browsers — fully safe, never fake bots.',
    heroPrimary: 'Create a free workspace',
    heroSecondary: 'Talk to our team',
    fine: ['Set up in 2 minutes', 'No machine resources needed', 'Per-staff permissions'],

    proof: [
      { v: '1', l: 'One workspace, one shared customer source for the team' },
      { v: '3', l: 'Commerce channels: Facebook · Taobao · 1688' },
      { v: '100%', l: 'Real browsers, human-like behaviour' },
      { v: '24/7', l: 'Real-time visibility into your sales team' },
    ],

    dash: {
      title: 'THG Workspace · Sales war room',
      live: 'Live',
      kpis: [
        { l: 'Leads today', v: '128', d: '+19%' },
        { l: 'In conversation', v: '34', d: 'live' },
        { l: 'Avg. response', v: '2.4m', d: '−38%' },
      ],
      teamTitle: 'Team activity',
      teamMeta: '5 staff',
      team: [
        { n: 'Lan', who: 'Lan · chatting with a lead', g: 'linear-gradient(140deg,#6366f1,#4338ca)', tag: 'In chat', cls: 'tagLive' },
        { n: 'Minh', who: 'Minh · replied to a comment', g: 'linear-gradient(140deg,#0ea5e9,#0369a1)', tag: '4m ago', cls: 'tagWait' },
        { n: 'Ha', who: 'Ha · closing a hot lead', g: 'linear-gradient(140deg,#f43f5e,#be123c)', tag: 'Hot lead', cls: 'tagHot' },
      ],
      speedLabel: 'Response speed · 7 days',
      capLabel: 'Safe daily limit',
      capValue: '72',
      capUnit: '/ 120 actions',
      capNote: 'Auto-paced to protect accounts',
      chip: { b: 'Accounts safe', s: 'Human-like behaviour' },
    },

    solEyebrow: 'Three solution areas',
    solTitle: 'Automate each channel — not a pile of disconnected scrapers.',
    solLead:
      'One workspace activates many solutions. Customer data and workflows are shared; each channel runs its own way. Turn on what you need.',
    solCta: 'See details',
    solSoonCta: 'Notify me',
    statusLive: 'Live',
    statusSoon: 'Coming soon',
    solutions: {
      'facebook-automation': {
        name: 'Facebook Automation',
        tag: 'Find & nurture customers on Facebook',
        body: 'Automatically scan groups for leads and filter real buying signals.',
        points: [
          'Scan groups, filter buying signals with AI',
          'Reply to comments & DMs in your brand voice',
          'Post and engage at a human cadence',
        ],
      },
      'taobao-sourcing': {
        name: 'Taobao Sourcing',
        tag: 'Track sourcing & competitors on Taobao',
        body: 'Track competitors, monitor suppliers, and hand off to your buying flow.',
        points: [
          'Track sellers & competitors by category',
          'Monitor price changes and new products',
          'Hand off into the buy-for / dropship flow',
        ],
      },
      'alibaba-1688': {
        name: '1688 Sourcing',
        tag: 'Discover the best prices on 1688',
        body: 'Surface the best sources and reconcile minimum order quantity (MOQ).',
        points: [
          'Surface suppliers & best price points',
          'Reconcile minimum order quantity (MOQ)',
          'Sync data for both buying and sales teams',
        ],
      },
    } as Record<string, { name: string; tag: string; body: string; points: string[] }>,

    whyEyebrow: 'Why THG',
    whyTitle: 'Built for owners who want their sales team to run smoothly.',
    why: [
      {
        icon: Building2,
        title: 'Centralized, resource-light',
        body: 'The whole team shares one customer database. No complex setup, no drain on each staff member’s computer.',
        viz: 'hub',
      },
      {
        icon: ShieldCheck,
        title: 'Safe & ban-resistant',
        body: 'The system paces behaviour like a real person, with daily action limits to protect your selling accounts.',
        viz: 'shield',
      },
      {
        icon: Eye,
        title: 'Real-time visibility',
        body: 'Owners instantly see who is caring for which customer and how fast — no manual reporting.',
        viz: 'pulse',
      },
    ],

    ctaEyebrow: 'Get started',
    ctaTitle: 'Digitize your sales process in two minutes.',
    ctaSub:
      'Create a workspace, activate Facebook automation, and connect a real browser with a one-time extension. Simple, safe, ready for the whole team.',
    ctaPrimary: 'Create a free workspace',
    ctaSecondary: 'I already have a workspace',
    ctaContact: 'Need a plan for a larger team? ',
    ctaContactLink: 'Contact the THG team',
    steps: [
      { b: 'Create a workspace', s: 'Sign up and set up a shared space for your team in seconds.' },
      { b: 'Activate Facebook', s: 'Turn on Facebook Automation and set your voice and target customers.' },
      { b: 'Install the extension', s: 'Install the extension once to connect a real browser and start operating.' },
    ],

    footTagline: 'The multi-channel sales automation OS for modern businesses.',
    footProduct: 'Product',
    footCompany: 'Company',
    footResources: 'Resources',
    footRights: 'THG Automation Platform. All rights reserved.',
    footPrivacy: 'Privacy',
    footExtension: 'Browser extension',
    footContact: 'Contact',
    footAbout: 'About',
    footStatus: 'All systems operational',
    footLogin: 'Sign in',
  },
} as const;

export default function PlatformLanding({ onLogin, onRegister }: Readonly<PlatformLandingProps>) {
  const { lang } = useLang();
  const copy = COPY[lang];
  const year = 2026;

  const [servicesOpen, setServicesOpen] = useState(false);
  const dropdownRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!servicesOpen) return;
    function onDoc(e: MouseEvent) {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) setServicesOpen(false);
    }
    function onKey(e: KeyboardEvent) { if (e.key === 'Escape') setServicesOpen(false); }
    document.addEventListener('mousedown', onDoc);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onDoc);
      document.removeEventListener('keydown', onKey);
    };
  }, [servicesOpen]);

  return (
    <main className={styles.page}>
      <div className={styles.backdrop} aria-hidden="true" />

      {/* ===================== NAV ===================== */}
      <header className={styles.nav}>
        <div className={styles.navInner}>
          <Link href="/" className={styles.brand}>
            <span className={styles.brandMark}>
              <img src="/assets/thg-pegasus.png" alt="THG" />
            </span>
            <span className={styles.brandText}>
              <strong>THG</strong>
              <span>{lang === 'vi' ? 'Nền tảng tự động hoá' : 'Automation platform'}</span>
            </span>
          </Link>

          <nav className={styles.navLinks} aria-label={lang === 'vi' ? 'Điều hướng' : 'Navigation'}>
            <div className={styles.dropdown} ref={dropdownRef}>
              <button
                type="button"
                className={styles.navTrigger}
                aria-haspopup="menu"
                aria-expanded={servicesOpen}
                onClick={() => setServicesOpen((o) => !o)}
              >
                {copy.navServices}
                <ChevronDown size={13} />
              </button>
              {servicesOpen && (
                <div className={styles.dropMenu} role="menu">
                  {MARKETING_SERVICES.map((svc) => {
                    const s = copy.solutions[svc.slug];
                    const on = svc.status === 'available';
                    return (
                      <Link
                        key={svc.slug}
                        href={`/products/${svc.slug}`}
                        className={styles.dropItem}
                        onClick={() => setServicesOpen(false)}
                      >
                        <span style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                          <span className={`${styles.dropDot} ${on ? styles.on : ''}`} />
                          <span>
                            <strong>{s?.name}</strong>
                            <small>{on ? copy.statusLive : copy.statusSoon}</small>
                          </span>
                        </span>
                        <ArrowRight size={14} color="var(--text-faint)" />
                      </Link>
                    );
                  })}
                </div>
              )}
            </div>
            <a href="#why">{copy.navWhy}</a>
            <a href="#launch">{copy.navLaunch}</a>
          </nav>

          <div className={styles.navRight}>
            <LangSwitch />
            <button type="button" className={styles.navTrigger} onClick={onLogin}>
              {copy.navLogin}
            </button>
            <button type="button" className={`${styles.btnPrimary} ${styles.btnSm}`} onClick={onRegister}>
              {copy.navCta}
            </button>
          </div>
        </div>
      </header>

      {/* ===================== HERO ===================== */}
      <section className={styles.hero}>
        <div className={styles.heroCopy}>
          <span className={`${styles.eyebrow} ${styles.reveal} ${styles.d1}`}>
            <span className={styles.pulse} />
            {copy.heroEyebrow}
          </span>

          <h1 className={`${styles.h1} ${styles.reveal} ${styles.d2}`}>
            {copy.heroTitle[0]}
            <span className={styles.accentWord}>{copy.heroTitle[1]}</span>
            {copy.heroTitle[2]}
          </h1>

          <p className={`${styles.sub} ${styles.reveal} ${styles.d3}`}>{copy.heroSub}</p>

          <div className={`${styles.heroActions} ${styles.reveal} ${styles.d4}`}>
            <button type="button" className={styles.btnPrimary} onClick={onRegister}>
              {copy.heroPrimary}
              <ArrowRight size={17} />
            </button>
            <a href="#launch" className={styles.btnGhost}>
              {copy.heroSecondary}
            </a>
          </div>

          <div className={`${styles.finePrint} ${styles.reveal} ${styles.d5}`}>
            {copy.fine.map((f) => (
              <span key={f}>
                <Check size={14} />
                {f}
              </span>
            ))}
          </div>
        </div>

        {/* ---- hand-built dashboard mock (no AI imagery) ---- */}
        <div className={`${styles.heroVisual} ${styles.reveal} ${styles.d3}`} aria-hidden="true">
          <div className={styles.dash}>
            <div className={styles.dashBar}>
              <span className={styles.dashDots}><i /><i /><i /></span>
              <span className={styles.dashTitle}>{copy.dash.title}</span>
              <span className={styles.dashLive}><i />{copy.dash.live}</span>
            </div>

            <div className={styles.dashBody}>
              <div className={styles.kpis}>
                {copy.dash.kpis.map((k) => (
                  <div key={k.l} className={styles.kpi}>
                    <small>{k.l}</small>
                    <b>{k.v}<em>{k.d}</em></b>
                  </div>
                ))}
              </div>

              <div className={styles.panel}>
                <div className={styles.panelHead}>
                  <strong>{copy.dash.teamTitle}</strong>
                  <span>{copy.dash.teamMeta}</span>
                </div>
                <div className={styles.feed}>
                  {copy.dash.team.map((m) => (
                    <div key={m.n} className={styles.feedRow}>
                      <span className={styles.avatar} style={{ background: m.g }}>
                        {m.n.slice(0, 1)}
                      </span>
                      <span className={styles.feedMain}>
                        <b>{m.who.split(' · ')[0]}</b>
                        <small>{m.who.split(' · ')[1]}</small>
                      </span>
                      <span className={`${styles.tag} ${styles[m.cls as 'tagLive']}`}>{m.tag}</span>
                    </div>
                  ))}
                </div>
              </div>

              <div className={styles.metricRow}>
                <div className={styles.chartCard}>
                  <span className={styles.miniLabel}>{copy.dash.speedLabel}</span>
                  <svg viewBox="0 0 240 56" preserveAspectRatio="none" role="img">
                    <defs>
                      <linearGradient id="spark" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="0%" stopColor="rgba(79,70,229,0.28)" />
                        <stop offset="100%" stopColor="rgba(79,70,229,0)" />
                      </linearGradient>
                    </defs>
                    <path
                      d="M0,44 L24,40 L48,42 L72,30 L96,33 L120,22 L144,26 L168,16 L192,18 L216,9 L240,12 L240,56 L0,56 Z"
                      fill="url(#spark)"
                    />
                    <path
                      d="M0,44 L24,40 L48,42 L72,30 L96,33 L120,22 L144,26 L168,16 L192,18 L216,9 L240,12"
                      fill="none"
                      stroke="var(--accent)"
                      strokeWidth="2"
                      strokeLinejoin="round"
                      strokeLinecap="round"
                    />
                    <circle cx="216" cy="9" r="3.5" fill="var(--accent)" />
                  </svg>
                </div>
                <div className={styles.gaugeCard}>
                  <span className={styles.miniLabel}>{copy.dash.capLabel}</span>
                  <b>{copy.dash.capValue}<span> {copy.dash.capUnit}</span></b>
                  <span className={styles.bar}>
                    <span className={styles.barFill} style={{ '--w': '60%' } as React.CSSProperties} />
                  </span>
                  <small>{copy.dash.capNote}</small>
                </div>
              </div>
            </div>
          </div>

          <div className={styles.floatChip}>
            <span className={styles.ico}><ShieldCheck size={17} /></span>
            <span>
              <b>{copy.dash.chip.b}</b>
              <small>{copy.dash.chip.s}</small>
            </span>
          </div>
        </div>
      </section>

      {/* ===================== PROOF BAND ===================== */}
      <section className={styles.proof}>
        {copy.proof.map((p) => (
          <div key={p.l} className={styles.proofItem}>
            <b>{p.v}</b>
            <span>{p.l}</span>
          </div>
        ))}
      </section>

      {/* ===================== SOLUTIONS ===================== */}
      <section id="solutions" className={styles.section}>
        <div className={styles.sectionHead}>
          <p className={styles.kicker}>{copy.solEyebrow}</p>
          <h2 className={styles.h2}>{copy.solTitle}</h2>
          <p className={styles.lead}>{copy.solLead}</p>
        </div>

        <div className={styles.solGrid}>
          {MARKETING_SERVICES.map((svc) => {
            const s = copy.solutions[svc.slug];
            const Icon = SERVICE_ICON[svc.slug] ?? Sparkles;
            const on = svc.status === 'available';
            return (
              <article key={svc.slug} className={styles.solCard}>
                <div className={styles.solArt}>
                  <SolutionArt slug={svc.slug} />
                </div>
                <div className={styles.solBody}>
                  <div className={styles.statusRow}>
                    <span className={styles.solIcon}><Icon size={19} /></span>
                    {on ? (
                      <span className={`${styles.badge} ${styles.badgeLive}`}><i />{copy.statusLive}</span>
                    ) : (
                      <span className={`${styles.badge} ${styles.badgeSoon}`}>{copy.statusSoon}</span>
                    )}
                  </div>
                  <h3>{s?.name}</h3>
                  <p className={styles.solTag}>{s?.tag}</p>
                  <p>{s?.body}</p>
                  <ul className={styles.solList}>
                    {s?.points.map((pt) => (
                      <li key={pt}><Check size={15} />{pt}</li>
                    ))}
                  </ul>
                  <div className={styles.solFoot}>
                    <Link href={`/products/${svc.slug}`} className={`${styles.solLink} ${on ? '' : styles.muted}`}>
                      {on ? copy.solCta : copy.solSoonCta}
                      <ArrowRight size={15} />
                    </Link>
                  </div>
                </div>
              </article>
            );
          })}
        </div>
      </section>

      {/* ===================== WHY ===================== */}
      <section id="why" className={styles.section}>
        <div className={styles.sectionHead}>
          <p className={styles.kicker}>{copy.whyEyebrow}</p>
          <h2 className={styles.h2}>{copy.whyTitle}</h2>
        </div>

        <div className={styles.whyGrid}>
          {copy.why.map(({ icon: Icon, title, body, viz }) => (
            <article key={title} className={styles.whyCard}>
              <span className={styles.whyIcon}><Icon size={22} /></span>
              <h3>{title}</h3>
              <p>{body}</p>
              <div className={styles.whyViz}>
                <WhyViz kind={viz} />
              </div>
            </article>
          ))}
        </div>
      </section>

      {/* ===================== LAUNCH CTA ===================== */}
      <section id="launch">
        <div className={styles.ctaBand}>
          <div className={styles.ctaGrid}>
            <div>
              <p className={styles.kicker}>{copy.ctaEyebrow}</p>
              <h2>{copy.ctaTitle}</h2>
              <p className={styles.ctaSub}>{copy.ctaSub}</p>
              <div className={styles.ctaActions}>
                <button type="button" className={styles.btnPrimary} onClick={onRegister}>
                  {copy.ctaPrimary}
                  <ArrowRight size={17} />
                </button>
                <button type="button" className={styles.btnGhost} onClick={onLogin}>
                  {copy.ctaSecondary}
                </button>
              </div>
              <p className={styles.ctaContact}>
                {copy.ctaContact}
                <a href="mailto:davidanh98@gmail.com">{copy.ctaContactLink}</a>
              </p>
            </div>

            <div className={styles.steps}>
              {copy.steps.map((st, i) => (
                <div key={st.b} className={styles.step}>
                  <span className={styles.stepNum}>{i + 1}</span>
                  <span>
                    <b>{st.b}</b>
                    <small>{st.s}</small>
                  </span>
                </div>
              ))}
            </div>
          </div>
        </div>
      </section>

      {/* ===================== FOOTER ===================== */}
      <footer className={styles.footerWrap}>
        <div className={styles.footerTop}>
          <div className={styles.footerBrand}>
            <Link href="/" className={styles.brand}>
              <span className={styles.brandMark}>
                <img src="/assets/thg-pegasus.png" alt="THG" />
              </span>
              <span className={styles.brandText}>
                <strong>THG</strong>
                <span>{lang === 'vi' ? 'Nền tảng tự động hoá' : 'Automation platform'}</span>
              </span>
            </Link>
            <p>{copy.footTagline}</p>
          </div>

          <div className={styles.footCol}>
            <h4>{copy.footProduct}</h4>
            {MARKETING_SERVICES.map((svc) => (
              <Link key={svc.slug} href={`/products/${svc.slug}`}>{copy.solutions[svc.slug]?.name}</Link>
            ))}
            <Link href="/extension-beta">{copy.footExtension}</Link>
          </div>

          <div className={styles.footCol}>
            <h4>{copy.footCompany}</h4>
            <a href="#why">{copy.footAbout}</a>
            <a href="mailto:davidanh98@gmail.com">{copy.footContact}</a>
            <button type="button" onClick={onLogin}>{copy.footLogin}</button>
          </div>

          <div className={styles.footCol}>
            <h4>{copy.footResources}</h4>
            <Link href="/privacy">{copy.footPrivacy}</Link>
            <a href="#launch">{copy.ctaEyebrow}</a>
            <a href="#solutions">{copy.solEyebrow}</a>
          </div>
        </div>

        <div className={styles.footBottom}>
          <span>© {year} {copy.footRights}</span>
          <span className={`${styles.badge} ${styles.badgeLive}`}><i />{copy.footStatus}</span>
        </div>
      </footer>
    </main>
  );
}

/* ------------------------------------------------------------------ *
 * Inline SVG illustrations — built by hand per CLAUDE.md (no AI art). *
 * ------------------------------------------------------------------ */

function SolutionArt({ slug }: Readonly<{ slug: string }>) {
  if (slug === 'facebook-automation') {
    // chat threads + signal capture
    return (
      <svg viewBox="0 0 320 152" preserveAspectRatio="xMidYMid slice">
        <rect x="28" y="30" width="150" height="34" rx="10" fill="#fff" stroke="rgba(13,22,41,0.10)" />
        <circle cx="46" cy="47" r="7" fill="#2563eb" />
        <rect x="62" y="40" width="74" height="6" rx="3" fill="rgba(13,22,41,0.18)" />
        <rect x="62" y="52" width="50" height="6" rx="3" fill="rgba(13,22,41,0.10)" />
        <rect x="118" y="78" width="160" height="38" rx="10" fill="#4f46e5" />
        <rect x="132" y="89" width="96" height="6" rx="3" fill="rgba(255,255,255,0.85)" />
        <rect x="132" y="101" width="64" height="6" rx="3" fill="rgba(255,255,255,0.55)" />
        <g>
          <circle cx="266" cy="44" r="16" fill="rgba(22,163,74,0.14)" />
          <path d="M259 44 l5 5 l9 -10" fill="none" stroke="#16a34a" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />
        </g>
      </svg>
    );
  }
  if (slug === 'taobao-sourcing') {
    // product cards + price tracking
    return (
      <svg viewBox="0 0 320 152" preserveAspectRatio="xMidYMid slice">
        {[40, 130, 220].map((x, i) => (
          <g key={x}>
            <rect x={x} y={36} width="70" height="80" rx="10" fill="#fff" stroke="rgba(13,22,41,0.10)" />
            <rect x={x + 12} y={48} width="46" height="30" rx="6" fill={i === 1 ? '#4f46e5' : 'rgba(13,22,41,0.08)'} />
            <rect x={x + 12} y={86} width="40" height="6" rx="3" fill="rgba(13,22,41,0.16)" />
            <rect x={x + 12} y={98} width="26" height="6" rx="3" fill="rgba(22,163,74,0.55)" />
          </g>
        ))}
        <path d="M40 130 L120 118 L210 124 L290 104" fill="none" stroke="#16a34a" strokeWidth="2" strokeLinecap="round" />
      </svg>
    );
  }
  // 1688 — supplier network + MOQ
  return (
    <svg viewBox="0 0 320 152" preserveAspectRatio="xMidYMid slice">
      <circle cx="160" cy="76" r="22" fill="#4f46e5" />
      <text x="160" y="81" textAnchor="middle" fontSize="13" fill="#fff" fontFamily="monospace">MOQ</text>
      {[[60, 40], [60, 112], [260, 40], [260, 112]].map(([x, y]) => (
        <g key={`${x}-${y}`}>
          <line x1="160" y1="76" x2={x} y2={y} stroke="rgba(13,22,41,0.16)" strokeWidth="1.5" />
          <circle cx={x} cy={y} r="13" fill="#fff" stroke="rgba(13,22,41,0.12)" />
          <circle cx={x} cy={y} r="5" fill="rgba(22,163,74,0.6)" />
        </g>
      ))}
    </svg>
  );
}

function WhyViz({ kind }: Readonly<{ kind: string }>) {
  if (kind === 'hub') {
    // shared data hub — many staff, one source
    return (
      <svg viewBox="0 0 300 96" preserveAspectRatio="xMidYMid meet">
        <rect x="118" y="34" width="64" height="28" rx="8" fill="var(--accent-soft)" stroke="var(--accent-line)" />
        <text x="150" y="52" textAnchor="middle" fontSize="11" fill="var(--accent)" fontFamily="monospace">DATA</text>
        {[24, 88, 212, 276].map((x, i) => (
          <g key={x}>
            <line x1="150" y1="48" x2={x} y2={i < 2 ? 20 : 76} stroke="var(--line-strong)" strokeWidth="1.5" />
            <circle cx={x} cy={i < 2 ? 20 : 76} r="11" fill="var(--card)" stroke="var(--line-strong)" />
            <circle cx={x} cy={i < 2 ? 20 : 76} r="4" fill="var(--text-faint)" />
          </g>
        ))}
      </svg>
    );
  }
  if (kind === 'shield') {
    // daily-cap gauge
    return (
      <svg viewBox="0 0 300 96" preserveAspectRatio="xMidYMid meet">
        <path d="M30 20 H270" stroke="var(--line)" strokeWidth="8" strokeLinecap="round" />
        <path d="M30 20 H180" stroke="var(--live)" strokeWidth="8" strokeLinecap="round" />
        <text x="30" y="46" fontSize="11" fill="var(--text-faint)" fontFamily="monospace">0</text>
        <text x="248" y="46" fontSize="11" fill="var(--text-faint)" fontFamily="monospace">MAX</text>
        <path d="M30 70 H270" stroke="var(--line)" strokeWidth="8" strokeLinecap="round" />
        <path d="M30 70 H150" stroke="var(--accent)" strokeWidth="8" strokeLinecap="round" />
        <circle cx="180" cy="20" r="6" fill="var(--card)" stroke="var(--live)" strokeWidth="2" />
        <circle cx="150" cy="70" r="6" fill="var(--card)" stroke="var(--accent)" strokeWidth="2" />
      </svg>
    );
  }
  // pulse — live response monitor
  return (
    <svg viewBox="0 0 300 96" preserveAspectRatio="xMidYMid meet">
      <path
        d="M10 60 H80 L96 30 L116 78 L134 48 L150 60 H290"
        fill="none"
        stroke="var(--accent)"
        strokeWidth="2.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <circle cx="290" cy="60" r="4" fill="var(--live)" />
      <circle cx="290" cy="60" r="9" fill="none" stroke="var(--live)" strokeWidth="1.5" opacity="0.5" />
    </svg>
  );
}
