'use client';
import { useRouter } from 'next/navigation';
import { ArrowRight, Sparkles, Globe, Cpu, Layers } from 'lucide-react';
import { useLang } from '../../modules/autoflow/i18n/useLang';
import { useAuthStore } from '../../modules/autoflow/stores/authStore';
import { usePlatformServices } from './usePlatformServices';
import { resolveWorkspacePresentation, type PresentationTone } from './resolveWorkspacePresentation';
import type { PlatformService } from './types';
import styles from '../onboarding.module.css';

const TONE_BADGE: Record<PresentationTone, string> = {
  success: styles.badgeLive,
  info: styles.badgeInfo,
  warning: styles.badgeWarn,
  danger: styles.badgeWarn,
  neutral: styles.badgeSoon,
};

function ServiceCard({ svc, lang, onNavigate }: Readonly<{ svc: PlatformService; lang: 'vi' | 'en'; onNavigate: (href: string) => void }>) {
  const ux = resolveWorkspacePresentation(svc, lang);
  const action = ux.primaryAction;
  const clickable = action !== null && action.enabled;
  const available = svc.status === 'available';

  return (
    <div
      className={`${styles.card} ${clickable ? styles.cardClickable : styles.cardDim}`}
      onClick={() => clickable && action && onNavigate(action.href)}
      role={clickable ? 'button' : undefined}
      tabIndex={clickable ? 0 : undefined}
      onKeyDown={(e) => { if (clickable && action && (e.key === 'Enter' || e.key === ' ')) { e.preventDefault(); onNavigate(action.href); } }}
    >
      <div className={styles.cardTop}>
        <span className={`${styles.svcIcon} ${available ? '' : styles.muted}`}>
          <Sparkles size={20} />
        </span>
        <span className={`${styles.badge} ${TONE_BADGE[ux.badge.tone]}`}>
          {ux.badge.tone === 'success' && <i />}
          {ux.badge.label}
        </span>
      </div>

      <h3 className={styles.cardTitle}>{svc.label}</h3>
      {ux.description && <p className={styles.cardDesc}>{ux.description}</p>}

      <div className={styles.chips}>
        {svc.capabilities.browserAutomation && (
          <span className={styles.chip}><Globe size={12} />{lang === 'vi' ? 'Trình duyệt thật' : 'Real browser'}</span>
        )}
        {svc.capabilities.aiAgents && (
          <span className={styles.chip}><Cpu size={12} />{lang === 'vi' ? 'Trợ lý AI' : 'AI agents'}</span>
        )}
        {svc.capabilities.multiWorkspace && (
          <span className={styles.chip}><Layers size={12} />{lang === 'vi' ? 'Đa workspace' : 'Multi-workspace'}</span>
        )}
      </div>

      <div className={styles.cardFoot}>
        {action ? (
          <button
            type="button"
            className={`${styles.btnPrimary} ${styles.btnBlock} ${styles.btnSpread}`}
            onClick={(e) => { e.stopPropagation(); if (action.enabled) onNavigate(action.href); }}
            disabled={!action.enabled}
          >
            <span>{action.label}</span>
            <ArrowRight size={15} />
          </button>
        ) : (
          <div className={styles.cardNote}>
            {lang === 'vi' ? 'Dịch vụ này sắp ra mắt.' : 'This service is coming soon.'}
          </div>
        )}
      </div>
    </div>
  );
}

function firstName(name: string | undefined | null): string {
  const trimmed = (name ?? '').trim();
  if (!trimmed) return '';
  return trimmed.split(/\s+/)[0];
}

export default function PlatformServicesPage() {
  const router = useRouter();
  const { lang } = useLang();
  const services = usePlatformServices();
  const user = useAuthStore(s => s.user);

  const greeting = lang === 'vi' ? 'Xin chào' : 'Welcome';
  const display = firstName(user?.name) || user?.email || (lang === 'vi' ? 'bạn' : 'there');
  const availableCount = services.filter(s => s.status === 'available').length;
  const totalCount = services.length;

  return (
    <div className={styles.canvas}>
      <div className={styles.wrap}>
        <div className={styles.eyebrow}>
          <span className={styles.dot} />{lang === 'vi' ? 'Bảng điều khiển' : 'Control center'}
        </div>
        <h1 className={styles.h1}>
          {greeting}, <span className={styles.mono}>{display}</span>.
        </h1>
        <p className={styles.lead}>
          {lang === 'vi'
            ? 'Mỗi dịch vụ là một mảng tự động hoá riêng. Khởi tạo workspace cho dịch vụ bạn muốn dùng — các dịch vụ khác vẫn ở đây khi cần.'
            : 'Each service is an independent automation domain. Initialise a workspace for the one you want — the others stay here for later.'}
        </p>
        {user?.email && (
          <p className={styles.meta}>
            {user.email}
            {totalCount > 0 && (
              <>
                {'  ·  '}
                {lang === 'vi'
                  ? `${availableCount}/${totalCount} dịch vụ đang khả dụng`
                  : `${availableCount}/${totalCount} services available`}
              </>
            )}
          </p>
        )}

        <div className={styles.grid}>
          {services.map(svc => (
            <ServiceCard key={svc.slug} svc={svc} lang={lang} onNavigate={(href) => router.push(href)} />
          ))}
        </div>
      </div>
    </div>
  );
}
