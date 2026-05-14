'use client';

import Link from 'next/link';
import {
  ArrowRight,
  Activity,
  Eye,
  Layers,
  Network,
  ShieldCheck,
  Sparkles,
} from 'lucide-react';
import { useLang } from '../modules/autoflow/i18n/useLang';
import MarketingNav, { MARKETING_SERVICES } from './MarketingNav';
import styles from './landing.module.css';

// Root platform landing — introduces THG as a multi-service automation
// platform. The per-service marketing pages (e.g. FacebookProductLanding)
// live at /products/<slug> and are linked from the services nav dropdown
// + the services grid below.

interface PlatformLandingProps {
  onLogin: () => void;
  onRegister: () => void;
}

const COPY = {
  vi: {
    heroKicker: 'THG · Automation platform',
    heroTitle: 'Một workspace cho cả đội bán hàng trên mọi kênh.',
    heroBody:
      'THG tổng hợp browser thật, intelligence chung và execution có chủ ý — để team sales vận hành Facebook, Taobao và 1688 từ cùng một workspace. Không phải scraper, không phải bot tool.',
    heroPrimary: 'Tạo workspace',
    heroSecondary: 'Đăng nhập',
    heroFinePrint: 'Browser của nhân viên · Extension cài 1 lần · Phân quyền theo workspace.',
    valuePills: [
      'Shared intelligence',
      'Coordinated execution',
      'Battlefield visibility',
      'Multi-platform',
    ],
    servicesEyebrow: 'Services',
    servicesTitle: 'Mỗi service là một mảng tự động hoá độc lập.',
    servicesBody:
      'Một workspace có thể kích hoạt nhiều service. Intelligence + ledger + coordination dùng chung; execution per-service. Bạn bật service nào, workspace có service đó.',
    servicesCta: 'Xem chi tiết',
    servicesComingSoon: 'Sắp ra mắt',
    serviceCards: {
      'facebook-automation': {
        tagline: 'Facebook sales intelligence workspace.',
        body: 'Crawl group thật bằng browser của staff, Market Signal Gate lọc tín hiệu, AI agent comment / inbox / posting có ngữ điệu doanh nghiệp.',
      },
      'taobao-sourcing': {
        tagline: 'Tìm nguồn sản phẩm Taobao tự động.',
        body: 'Theo dõi seller, đối thủ, danh mục — kèm bộ lọc chất lượng và chuyển sang quy trình mua hộ / dropship trong workspace.',
      },
      'alibaba-1688': {
        tagline: 'Sourcing & supplier intelligence 1688.',
        body: 'Phát hiện supplier mới, giám sát giá, đối chiếu MOQ — đồng bộ với workspace fulfillment để đội mua hàng và sales cùng nhìn 1 nguồn.',
      },
    } as Record<string, { tagline: string; body: string }>,
    pillarsEyebrow: 'Why THG',
    pillarsTitle: 'Architecture cho đội bán hàng phối hợp — không phải bot độc lập.',
    pillars: [
      {
        icon: Layers,
        title: 'Shared Intelligence',
        body: 'Crawl một lần, share workspace. Leads, signals, market data tách khỏi execution per-account để không tốn browser thật.',
      },
      {
        icon: Network,
        title: 'Coordination Plane',
        body: 'Action ledger ghi mọi hành vi của từng account. Behaviour profile + daily caps + risk score ngăn account bị nóng.',
      },
      {
        icon: Eye,
        title: 'Battlefield Visibility',
        body: 'Lead engagement state surface "Alice inbox 4 phút trước" — team self-coordinate bằng visibility, không cần CRM lock.',
      },
      {
        icon: ShieldCheck,
        title: 'Execution Identity',
        body: 'Mỗi staff sở hữu account của mình. HTTP / WebSocket / skill path đều enforced bằng assigned_user_id — không cross-staff hijack.',
      },
    ],
    pillarsTail:
      'Coordination Plane PR-1/2/4 đã shipped: Action Ledger, Behaviour Profile, Lead Engagement State. Execution Verification (PR-2.5) và Account Orchestrator (PR-5) trên roadmap.',
    finalEyebrow: 'Launch',
    finalTitle: 'Workspace của đội bạn — sẵn sàng trong 2 phút.',
    finalBody:
      'Tạo workspace, kích hoạt service đầu tiên (Facebook Automation), kết nối browser thật bằng Extension. Các service Taobao + 1688 mở khi roadmap landing.',
    finalPrimary: 'Tạo workspace',
    finalSecondary: 'Tôi đã có workspace',
    footer: 'THG Automation Platform',
    footerCopy: 'Workspace dùng chung cho đội sales vận hành nhiều kênh thương mại.',
  },
  en: {
    heroKicker: 'THG · Automation platform',
    heroTitle: 'One workspace for sales teams across every commerce channel.',
    heroBody:
      'THG fuses real browser sessions, shared intelligence, and deliberate execution — so sales teams operate Facebook, Taobao, and 1688 from a single workspace. Not a scraper. Not a bot kit.',
    heroPrimary: 'Get started',
    heroSecondary: 'Sign in',
    heroFinePrint: 'Real staff browsers · One extension install · Workspace-scoped permissions.',
    valuePills: [
      'Shared intelligence',
      'Coordinated execution',
      'Battlefield visibility',
      'Multi-platform',
    ],
    servicesEyebrow: 'Services',
    servicesTitle: 'Each service is an independent automation domain.',
    servicesBody:
      'A single workspace can activate multiple services. Intelligence, ledger, and coordination are shared; execution is per-service. Turn on what you need.',
    servicesCta: 'See details',
    servicesComingSoon: 'Coming soon',
    serviceCards: {
      'facebook-automation': {
        tagline: 'Facebook sales intelligence workspace.',
        body: 'Crawl real groups via the staff browser, Market Signal Gate filters the noise, AI agents comment / inbox / post with your sales voice.',
      },
      'taobao-sourcing': {
        tagline: 'Automated Taobao sourcing intelligence.',
        body: 'Track sellers, competitors, and categories — with quality filters and a handoff into the workspace buying / dropship flow.',
      },
      'alibaba-1688': {
        tagline: '1688 sourcing & supplier intelligence.',
        body: 'Surface new suppliers, monitor pricing, reconcile MOQ — synced to the workspace fulfillment layer so sourcing and sales share one truth.',
      },
    } as Record<string, { tagline: string; body: string }>,
    pillarsEyebrow: 'Why THG',
    pillarsTitle: 'Architecture for coordinated sales teams — not standalone bots.',
    pillars: [
      {
        icon: Layers,
        title: 'Shared Intelligence',
        body: 'Crawl once, share the workspace. Leads, signals, and market data are decoupled from per-account execution.',
      },
      {
        icon: Network,
        title: 'Coordination Plane',
        body: 'Action ledger records every account action. Behaviour profile + daily caps + risk score keep accounts cool.',
      },
      {
        icon: Eye,
        title: 'Battlefield Visibility',
        body: 'Lead engagement surfaces "Alice inboxed 4m ago" — the team self-coordinates by visibility, without CRM record-locking.',
      },
      {
        icon: ShieldCheck,
        title: 'Execution Identity',
        body: 'Each staff owns their account. HTTP / WebSocket / skill paths all enforce assigned_user_id — no cross-staff hijack.',
      },
    ],
    pillarsTail:
      'Coordination Plane PR-1/2/4 shipped: Action Ledger, Behaviour Profile, Lead Engagement State. Execution Verification (PR-2.5) and Account Orchestrator (PR-5) on the roadmap.',
    finalEyebrow: 'Launch',
    finalTitle: 'Your team’s workspace — ready in two minutes.',
    finalBody:
      'Create the workspace, activate your first service (Facebook Automation), and connect the real browser via the Extension. Taobao + 1688 services open as the roadmap lands.',
    finalPrimary: 'Get started',
    finalSecondary: 'I already have a workspace',
    footer: 'THG Automation Platform',
    footerCopy: 'A shared workspace for sales teams operating across commerce channels.',
  },
} as const;

export default function PlatformLanding({ onLogin, onRegister }: PlatformLandingProps) {
  const { lang } = useLang();
  const copy = COPY[lang];

  return (
    <main className={styles.page}>
      <div className={styles.backdrop} aria-hidden="true" />

      <MarketingNav onLogin={onLogin} onRegister={onRegister} />

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

          <p className={styles.heroNote}>{copy.heroFinePrint}</p>

          <div className={styles.valuePills}>
            {copy.valuePills.map((pill) => (
              <span key={pill}>{pill}</span>
            ))}
          </div>
        </div>

        {/* Right-hand visual: services preview surface */}
        <div className={styles.heroSurface} aria-hidden="true">
          <div className={styles.windowBar}>
            <div className={styles.windowDots}>
              <span />
              <span />
              <span />
            </div>
            <p>THG Workspace · Services</p>
          </div>

          <div className={styles.surfaceMain} style={{ gap: 12 }}>
            {MARKETING_SERVICES.map((svc) => {
              const label = lang === 'vi' ? svc.labelVi : svc.labelEn;
              const card = copy.serviceCards[svc.slug];
              const tagline = card ? card.tagline : '';
              const isAvailable = svc.status === 'available';
              return (
                <article
                  key={svc.slug}
                  style={{
                    padding: '14px 16px',
                    border: '1px solid var(--line)',
                    borderRadius: 14,
                    background: isAvailable ? 'rgba(160, 255, 80, 0.04)' : 'rgba(255,255,255,0.02)',
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8, color: 'var(--text)' }}>
                      <Sparkles size={14} style={{ color: isAvailable ? 'var(--accent)' : 'var(--text-faint)' }} />
                      <strong style={{ fontSize: 13.5, fontWeight: 600 }}>{label}</strong>
                    </div>
                    <span
                      style={{
                        fontFamily: 'var(--font-mono)',
                        fontSize: 10,
                        textTransform: 'uppercase',
                        letterSpacing: '0.06em',
                        color: isAvailable ? 'var(--accent)' : 'var(--text-faint)',
                      }}
                    >
                      {isAvailable ? (lang === 'vi' ? 'Đang hoạt động' : 'Available') : copy.servicesComingSoon}
                    </span>
                  </div>
                  <p style={{ marginTop: 8, fontSize: 12.5, color: 'var(--text-mute)', lineHeight: 1.55 }}>
                    {tagline}
                  </p>
                </article>
              );
            })}
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 4, color: 'var(--text-faint)', fontSize: 11, fontFamily: 'var(--font-mono)' }}>
              <Activity size={12} />
              {lang === 'vi' ? 'Workspace · Intelligence + Coordination + Execution' : 'Workspace · Intelligence + Coordination + Execution'}
            </div>
          </div>
        </div>
      </section>

      <section id="services" className={styles.section}>
        <div className={styles.sectionIntro}>
          <p className="eyebrow">
            <span className="dot" />
            {copy.servicesEyebrow}
          </p>
          <h2>{copy.servicesTitle}</h2>
          <p>{copy.servicesBody}</p>
        </div>

        <div className={styles.featureGrid}>
          {MARKETING_SERVICES.map((svc) => {
            const card = copy.serviceCards[svc.slug];
            const label = lang === 'vi' ? svc.labelVi : svc.labelEn;
            const isAvailable = svc.status === 'available';
            return (
              <article key={svc.slug} className={styles.featureCard}>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 10 }}>
                  <div className={styles.featureIcon}>
                    <Sparkles size={16} />
                  </div>
                  <span
                    style={{
                      fontFamily: 'var(--font-mono)',
                      fontSize: 10,
                      textTransform: 'uppercase',
                      letterSpacing: '0.06em',
                      color: isAvailable ? 'var(--accent)' : 'var(--text-faint)',
                    }}
                  >
                    {isAvailable ? (lang === 'vi' ? 'Đang hoạt động' : 'Available') : copy.servicesComingSoon}
                  </span>
                </div>
                <h3>{label}</h3>
                <p>{card?.body}</p>
                <Link
                  href={`/products/${svc.slug}`}
                  style={{
                    marginTop: 14,
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: 6,
                    fontSize: 13,
                    color: isAvailable ? 'var(--accent)' : 'var(--text-faint)',
                    textDecoration: 'none',
                    fontWeight: 500,
                  }}
                >
                  {copy.servicesCta} <ArrowRight size={13} />
                </Link>
              </article>
            );
          })}
        </div>
      </section>

      <section id="why" className={styles.section}>
        <div className={styles.sectionIntro}>
          <p className="eyebrow">
            <span className="dot" />
            {copy.pillarsEyebrow}
          </p>
          <h2>{copy.pillarsTitle}</h2>
        </div>

        <div className={styles.featureGrid}>
          {copy.pillars.map(({ icon: Icon, title, body }) => (
            <article key={title} className={styles.featureCard}>
              <div className={styles.featureIcon}>
                <Icon size={16} />
              </div>
              <h3>{title}</h3>
              <p>{body}</p>
            </article>
          ))}
        </div>
        <p style={{ marginTop: 18, fontSize: 12.5, color: 'var(--text-faint)', maxWidth: 720 }}>
          {copy.pillarsTail}
        </p>
      </section>

      <section className={styles.finalBand}>
        <div>
          <p className="eyebrow">
            <span className="dot" />
            {copy.finalEyebrow}
          </p>
          <h2>{copy.finalTitle}</h2>
          <p>{copy.finalBody}</p>
        </div>

        <div className={styles.finalActions}>
          <button type="button" className="btn btn-primary btn-lg" onClick={onRegister}>
            {copy.finalPrimary}
          </button>
          <button type="button" className="btn btn-ghost btn-lg" onClick={onLogin}>
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
          {lang === 'vi' ? 'Đăng nhập' : 'Sign in'}
        </button>
      </footer>
    </main>
  );
}
