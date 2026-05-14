'use client';
import { useParams } from 'next/navigation';
import { getService } from '@/src/platform/services/registry';
import { bootstrapServices } from '@/src/platform/services/bootstrap';
import { usePlatformService } from '@/src/platform/services/usePlatformServices';
import { ServiceProvider } from '@/src/platform/services/ServiceContext';
import { ServiceBoundary } from '@/src/platform/services/ServiceBoundary';
import { useLang } from '@/src/modules/autoflow/i18n/useLang';

bootstrapServices();

export default function ServiceCreateWorkspaceRoute() {
  const params = useParams<{ slug: string }>();
  const { lang } = useLang();
  const slug = String(params?.slug ?? '');
  const mod = getService(slug);
  const svc = usePlatformService(slug);

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

  const View = mod.views.createWorkspace;
  return (
    <ServiceProvider service={svc} module={mod}>
      <ServiceBoundary>
        <View />
      </ServiceBoundary>
    </ServiceProvider>
  );
}
