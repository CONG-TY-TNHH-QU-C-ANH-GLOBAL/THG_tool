'use client';

import Link from 'next/link';
import { ArrowLeft, Clock } from 'lucide-react';
import { useLang } from '../modules/autoflow/i18n/useLang';
import MarketingNav, { MARKETING_SERVICES } from './MarketingNav';
import styles from './landing.module.css';

// Per-service "Coming soon" marketing page. Same chrome as the platform
// landing (MarketingNav + page backdrop) so navigation feels continuous.
// Renders the service's marketing label + a short rationale + a back link.

interface ComingSoonLandingProps {
  slug: string;
  onLogin: () => void;
  onRegister: () => void;
}

const RATIONALES: Record<string, { vi: string; en: string }> = {
  'taobao-sourcing': {
    vi: 'Module Taobao Sourcing đang được xây dựng trên cùng Coordination Plane và Behaviour Profile substrate đã shipped cho Facebook. Khi mở, bạn sẽ thấy service này xuất hiện trực tiếp trong workspace hiện tại — không cần migration.',
    en: 'Taobao Sourcing is being built on the same Coordination Plane and Behaviour Profile substrate already shipped for Facebook. When it opens, the service will appear directly inside your current workspace — no migration required.',
  },
  'alibaba-1688': {
    vi: '1688 Sourcing chia sẻ pipeline supplier intelligence với Taobao và đồng bộ với workspace fulfillment. Module ưu tiên mở sau khi Taobao Sourcing đi vào sản xuất ổn định.',
    en: '1688 Sourcing shares the supplier intelligence pipeline with Taobao and syncs with the workspace fulfillment layer. Opens after Taobao Sourcing reaches stable production.',
  },
};

export default function ComingSoonLanding({ slug, onLogin, onRegister }: ComingSoonLandingProps) {
  const { lang } = useLang();
  const svc = MARKETING_SERVICES.find(s => s.slug === slug);
  const rationale = RATIONALES[slug];
  const label = svc ? (lang === 'vi' ? svc.labelVi : svc.labelEn) : slug;

  return (
    <main className={styles.page}>
      <div className={styles.backdrop} aria-hidden="true" />

      <MarketingNav
        onLogin={onLogin}
        onRegister={onRegister}
        currentServiceSlug={slug}
      />

      <section className={styles.hero} style={{ gridTemplateColumns: '1fr', paddingTop: 64 }}>
        <div className={styles.heroCopy} style={{ maxWidth: 720 }}>
          <Link
            href="/"
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: 6,
              color: 'var(--text-faint)',
              fontSize: 12.5,
              textDecoration: 'none',
              marginBottom: 18,
            }}
          >
            <ArrowLeft size={13} /> {lang === 'vi' ? 'Tất cả services' : 'All services'}
          </Link>
          <p className="eyebrow">
            <span className="dot" />
            {lang === 'vi' ? 'SẮP RA MẮT' : 'COMING SOON'}
          </p>
          <h1 className={styles.heroTitle} style={{ fontSize: 'clamp(28px, 4.2vw, 40px)' }}>
            {label}
          </h1>
          <p className={styles.heroBody}>
            {rationale
              ? rationale[lang]
              : (lang === 'vi'
                ? 'Module này đang được phát triển trên cùng platform.'
                : 'This module is being built on the same platform.')}
          </p>

          <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginTop: 18, color: 'var(--text-faint)', fontSize: 12 }}>
            <Clock size={13} />
            {lang === 'vi'
              ? 'Workspace sẽ tự động surface service này khi mở.'
              : 'Your workspace will surface this service automatically when it ships.'}
          </div>

          <div className={styles.heroActions}>
            <Link href="/products/facebook-automation" className="btn btn-primary btn-lg">
              {lang === 'vi' ? 'Khám phá Facebook Automation' : 'Explore Facebook Automation'}
            </Link>
            <button type="button" className="btn btn-ghost btn-lg" onClick={onLogin}>
              {lang === 'vi' ? 'Vào dashboard' : 'Open dashboard'}
            </button>
          </div>
        </div>
      </section>
    </main>
  );
}
