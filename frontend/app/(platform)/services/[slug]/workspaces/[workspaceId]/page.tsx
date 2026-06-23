'use client';
import { useParams, useRouter } from 'next/navigation';
import { useEffect } from 'react';
import { ArrowRight } from 'lucide-react';
import { getService } from '@/src/platform/services/registry';
import { bootstrapServices } from '@/src/platform/services/bootstrap';
import { usePlatformService } from '@/src/platform/services/usePlatformServices';
import { ServiceProvider } from '@/src/platform/services/ServiceContext';
import { ServiceBoundary } from '@/src/platform/services/ServiceBoundary';
import { resolveWorkspacePresentation, type PresentationTone } from '@/src/platform/services/resolveWorkspacePresentation';
import type { PlatformService } from '@/src/platform/services/types';
import { useLang } from '@/src/modules/autoflow/i18n/useLang';

bootstrapServices();

const TONE_COLORS: Record<PresentationTone, string> = {
  success: 'var(--accent)',
  info: 'var(--text-mute)',
  warning: '#d28a4b',
  danger: '#d9534f',
  neutral: 'var(--text-faint)',
};

function NonReadyCard({ svc, lang, onAction }: Readonly<{ svc: PlatformService; lang: 'vi' | 'en'; onAction: (href: string) => void }>) {
  const ux = resolveWorkspacePresentation(svc, lang);
  return (
    <div style={{ flex: 1, display: 'grid', placeItems: 'center', padding: 24 }}>
      <div className="card" style={{ maxWidth: 460, padding: 28, textAlign: 'center' }}>
        <div style={{ marginBottom: 10, fontSize: 11, color: TONE_COLORS[ux.badge.tone], textTransform: 'uppercase', letterSpacing: '0.05em' }}>
          {ux.badge.label}
        </div>
        <h2 style={{ fontSize: 18, marginBottom: 8 }}>{svc.label}</h2>
        {ux.description && (
          <p style={{ color: 'var(--text-mute)', fontSize: 13, marginBottom: 14 }}>{ux.description}</p>
        )}
        {ux.primaryAction && (
          <button
            type="button"
            className="btn btn-primary btn-sm"
            style={{ width: '100%', justifyContent: 'space-between' }}
            onClick={() => ux.primaryAction!.enabled && onAction(ux.primaryAction!.href)}
            disabled={!ux.primaryAction.enabled}
          >
            <span>{ux.primaryAction.label}</span>
            <ArrowRight size={13} />
          </button>
        )}
      </div>
    </div>
  );
}

export default function ServiceWorkspaceRoute() {
  const router = useRouter();
  const params = useParams<{ slug: string; workspaceId: string }>();
  const { lang } = useLang();
  const slug = String(params?.slug ?? '');
  const workspaceId = String(params?.workspaceId ?? '');
  const mod = getService(slug);
  const svc = usePlatformService(slug);

  useEffect(() => {
    if (!svc) return;
    // If user has no workspace yet, route them to create.
    if (svc.workspaceState === 'none' && svc.access !== 'invite_required') {
      router.replace(`/services/${slug}/workspaces/new`);
    }
  }, [svc, slug, router]);

  if (!mod || !svc) {
    return (
      <div style={{ flex: 1, display: 'grid', placeItems: 'center', padding: 24 }}>
        <div className="card" style={{ maxWidth: 420, padding: 28, textAlign: 'center' }}>
          <h2 style={{ fontSize: 18, marginBottom: 8 }}>
            {lang === 'vi' ? 'Service không tồn tại' : 'Service not found'}
          </h2>
          <p style={{ color: 'var(--text-mute)', fontSize: 13 }}>
            {lang === 'vi' ? `Không có service "${slug}".` : `No service named "${slug}".`}
          </p>
        </div>
      </div>
    );
  }

  // Renderer never branches on lifecycle enums directly. Presentation resolver
  // owns that mapping. Anything that isn't `canEnter` falls through to NonReadyCard.
  const ux = resolveWorkspacePresentation(svc, lang);

  if (!ux.canEnter) {
    return (
      <ServiceProvider service={svc} module={mod} workspaceId={workspaceId}>
        <ServiceBoundary>
          <NonReadyCard svc={svc} lang={lang} onAction={(href) => router.push(href)} />
        </ServiceBoundary>
      </ServiceProvider>
    );
  }

  const View = mod.views.workspace;
  return (
    <ServiceProvider service={svc} module={mod} workspaceId={workspaceId}>
      <ServiceBoundary>
        <View workspaceId={workspaceId} />
      </ServiceBoundary>
    </ServiceProvider>
  );
}
