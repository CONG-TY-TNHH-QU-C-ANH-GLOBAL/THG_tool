'use client';
import { useEffect, useRef, useState } from 'react';
import Link from 'next/link';
import { ChevronDown } from 'lucide-react';
import { LangSwitch } from '../modules/autoflow/components/ds/LangSwitch';
import { useLang } from '../modules/autoflow/i18n/useLang';
import styles from './landing.module.css';

// Catalog of services surfaced in the public marketing nav.
// Mirrors the platform registry (internal/platform/services/bootstrap.go,
// frontend/src/platform/services/bootstrap.ts) but is a separate list on
// purpose: marketing surface = public products, not the per-user resolved
// service list. Status here is rollout status, not per-user availability.
export interface MarketingServiceEntry {
  slug: string;             // matches the /products/<slug> route
  labelVi: string;
  labelEn: string;
  status: 'available' | 'coming_soon';
}

export const MARKETING_SERVICES: MarketingServiceEntry[] = [
  { slug: 'facebook-automation', labelVi: 'Facebook Automation', labelEn: 'Facebook Automation', status: 'available' },
  { slug: 'taobao-sourcing',     labelVi: 'Taobao Sourcing',     labelEn: 'Taobao Sourcing',     status: 'coming_soon' },
  { slug: 'alibaba-1688',        labelVi: '1688 Sourcing',       labelEn: '1688 Sourcing',       status: 'coming_soon' },
];

export interface SectionLink {
  href: string;
  label: string;
}

interface MarketingNavProps {
  onLogin: () => void;
  onRegister: () => void;
  // Slug of the service currently being viewed (or undefined on platform landing).
  // Highlights that service in the dropdown.
  currentServiceSlug?: string;
  // Per-page section anchors (e.g. #features). Optional — only the FB product
  // page exposes these; the platform landing leaves it empty.
  sectionLinks?: SectionLink[];
}

export default function MarketingNav({
  onLogin,
  onRegister,
  currentServiceSlug,
  sectionLinks = [],
}: Readonly<MarketingNavProps>) {
  const { lang } = useLang();
  const [servicesOpen, setServicesOpen] = useState(false);
  const dropdownRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!servicesOpen) return;
    function handleClick(e: MouseEvent) {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setServicesOpen(false);
      }
    }
    function handleKey(e: KeyboardEvent) {
      if (e.key === 'Escape') setServicesOpen(false);
    }
    document.addEventListener('mousedown', handleClick);
    document.addEventListener('keydown', handleKey);
    return () => {
      document.removeEventListener('mousedown', handleClick);
      document.removeEventListener('keydown', handleKey);
    };
  }, [servicesOpen]);

  return (
    <header className={styles.navWrap}>
      <div className={styles.nav}>
        <Link href="/" className={styles.brand} style={{ textDecoration: 'none' }}>
          <div className={styles.brandMark}>
            <img src="/assets/thg-pegasus.png" alt="THG" style={{ width: 28, height: 28, objectFit: 'contain' }} />
          </div>
          <div>
            <strong>THG</strong>
            <span>{lang === 'vi' ? 'Nền tảng tự động hoá' : 'Automation platform'}</span>
          </div>
        </Link>

        <nav className={styles.navLinks} aria-label={lang === 'vi' ? 'Điều hướng' : 'Navigation'}>
          <div ref={dropdownRef} style={{ position: 'relative' }}>
            <button
              type="button"
              onClick={() => setServicesOpen(o => !o)}
              aria-haspopup="menu"
              aria-expanded={servicesOpen}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 4,
                background: 'transparent',
                border: 0,
                color: 'inherit',
                cursor: 'pointer',
                font: 'inherit',
                padding: 0,
              }}
            >
              {lang === 'vi' ? 'Dịch vụ' : 'Services'}
              <ChevronDown size={12} style={{ marginTop: 1 }} />
            </button>
            {servicesOpen && (
              <div
                role="menu"
                style={{
                  position: 'absolute',
                  top: 'calc(100% + 8px)',
                  left: -12,
                  minWidth: 260,
                  padding: 6,
                  borderRadius: 14,
                  background: 'rgba(14, 14, 17, 0.95)',
                  border: '1px solid var(--line-strong, rgba(255,255,255,0.14))',
                  backdropFilter: 'blur(22px) saturate(145%)',
                  boxShadow: '0 24px 60px rgba(0,0,0,0.45)',
                  zIndex: 50,
                }}
              >
                {MARKETING_SERVICES.map(svc => {
                  const label = lang === 'vi' ? svc.labelVi : svc.labelEn;
                  const isCurrent = svc.slug === currentServiceSlug;
                  const isComingSoon = svc.status === 'coming_soon';
                  const href = `/products/${svc.slug}`;
                  return (
                    <Link
                      key={svc.slug}
                      href={href}
                      onClick={() => setServicesOpen(false)}
                      style={{
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'space-between',
                        padding: '10px 12px',
                        borderRadius: 10,
                        color: isCurrent ? 'var(--accent)' : 'var(--text)',
                        background: isCurrent ? 'var(--accent-soft, rgba(160,255,80,0.08))' : 'transparent',
                        textDecoration: 'none',
                        fontSize: 13.5,
                      }}
                    >
                      <span>{label}</span>
                      {isComingSoon && (
                        <span style={{ fontSize: 10, color: 'var(--text-faint)', letterSpacing: '0.04em', textTransform: 'uppercase' }}>
                          {lang === 'vi' ? 'Sắp ra mắt' : 'Soon'}
                        </span>
                      )}
                    </Link>
                  );
                })}
              </div>
            )}
          </div>
          {sectionLinks.map(link => (
            <a key={link.href} href={link.href}>{link.label}</a>
          ))}
        </nav>

        <div className={styles.navActions}>
          <LangSwitch />
          <button type="button" className="btn btn-ghost" onClick={onLogin}>
            {lang === 'vi' ? 'Đăng nhập' : 'Sign in'}
          </button>
          <button type="button" className="btn btn-primary" onClick={onRegister}>
            {lang === 'vi' ? 'Tạo workspace' : 'Get started'}
          </button>
        </div>
      </div>
    </header>
  );
}
