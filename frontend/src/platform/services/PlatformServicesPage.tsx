'use client';
import { useRouter } from 'next/navigation';
import { ArrowRight } from 'lucide-react';
import { useLang } from '../../modules/autoflow/i18n/useLang';
import { useAuthStore } from '../../modules/autoflow/stores/authStore';
import { usePlatformServices } from './usePlatformServices';
import { resolveWorkspacePresentation, type PresentationTone } from './resolveWorkspacePresentation';
import type { PlatformService } from './types';

const TONE_COLORS: Record<PresentationTone, string> = {
  success: 'var(--accent)',
  info: 'var(--text-mute)',
  warning: '#d28a4b',
  danger: '#d9534f',
  neutral: 'var(--text-faint)',
};

function ServiceCard({ svc, lang, onNavigate }: { svc: PlatformService; lang: 'vi' | 'en'; onNavigate: (href: string) => void }) {
  const ux = resolveWorkspacePresentation(svc, lang);
  const action = ux.primaryAction;
  const clickable = action !== null && action.enabled;

  return (
    <div
      className="card"
      style={{
        padding: 20,
        display: 'flex',
        flexDirection: 'column',
        gap: 14,
        opacity: clickable ? 1 : 0.65,
        cursor: clickable ? 'pointer' : 'default',
      }}
      onClick={() => clickable && action && onNavigate(action.href)}
    >
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 10 }}>
        <div style={{ fontSize: 16, fontWeight: 600 }}>{svc.label}</div>
        <span style={{ fontSize: 11, color: TONE_COLORS[ux.badge.tone], textTransform: 'uppercase', letterSpacing: '0.05em' }}>
          {ux.badge.label}
        </span>
      </div>

      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
        {svc.capabilities.browserAutomation && (
          <span className="badge" style={{ fontSize: 10 }}>Browser automation</span>
        )}
        {svc.capabilities.aiAgents && (
          <span className="badge" style={{ fontSize: 10 }}>AI agents</span>
        )}
        {svc.capabilities.multiWorkspace && (
          <span className="badge" style={{ fontSize: 10 }}>{lang === 'vi' ? 'Đa workspace' : 'Multi-workspace'}</span>
        )}
      </div>

      {ux.description && (
        <p style={{ fontSize: 12, color: 'var(--text-faint)', margin: 0 }}>{ux.description}</p>
      )}

      <div style={{ flex: 1 }} />

      {action ? (
        <button
          type="button"
          className="btn btn-primary btn-sm"
          style={{ width: '100%', justifyContent: 'space-between' }}
          onClick={(e) => { e.stopPropagation(); if (action.enabled) onNavigate(action.href); }}
          disabled={!action.enabled}
        >
          <span>{action.label}</span>
          <ArrowRight size={13} />
        </button>
      ) : (
        <div style={{ fontSize: 11, color: 'var(--text-faint)' }}>
          {lang === 'vi' ? 'Service này chưa khả dụng.' : 'Not available.'}
        </div>
      )}
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
    <div style={{ flex: 1, overflow: 'auto', padding: '32px 24px' }}>
      <div style={{ maxWidth: 1100, margin: '0 auto' }}>
        <div className="eyebrow" style={{ marginBottom: 8 }}>
          <span className="dot" />PLATFORM
        </div>
        <h1 style={{ fontSize: 28, marginBottom: 6 }}>
          {greeting}, <span className="title-mono">{display}</span>.
        </h1>
        <p style={{ color: 'var(--text-mute)', marginBottom: 6, fontSize: 14, maxWidth: 720 }}>
          {lang === 'vi'
            ? 'Mỗi service là một mảng tự động hoá độc lập. Khởi tạo workspace cho service nào bạn muốn dùng — các service khác vẫn ở đây khi cần.'
            : 'Each service is an independent automation domain. Initialise a workspace for the one you want to use — the others stay here for later.'}
        </p>
        {user?.email && (
          <p className="mono" style={{ color: 'var(--text-faint)', marginBottom: 28, fontSize: 12 }}>
            {user.email}
            {totalCount > 0 && (
              <>
                {' · '}
                {lang === 'vi'
                  ? `${availableCount}/${totalCount} service đang khả dụng`
                  : `${availableCount}/${totalCount} services available`}
              </>
            )}
          </p>
        )}

        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))', gap: 16 }}>
          {services.map(svc => (
            <ServiceCard key={svc.slug} svc={svc} lang={lang} onNavigate={(href) => router.push(href)} />
          ))}
        </div>
      </div>
    </div>
  );
}
