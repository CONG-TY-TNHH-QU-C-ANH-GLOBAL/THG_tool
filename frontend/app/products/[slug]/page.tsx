'use client';
import { useParams, useRouter } from 'next/navigation';
import { notFound } from 'next/navigation';
import FacebookProductLanding from '@/src/modules/autoflow/components/FacebookProductLanding';
import ComingSoonLanding from '@/src/marketing/ComingSoonLanding';
import { MARKETING_SERVICES } from '@/src/marketing/MarketingNav';
import '@/src/modules/autoflow/autoflow.css';

// /products/<slug> — public marketing pages, one per service. The slug
// catalog is owned by MARKETING_SERVICES (see MarketingNav.tsx). Each slug
// renders either a bespoke per-service landing (Facebook today) or the
// generic ComingSoonLanding fallback for not-yet-shipped services.

export default function ProductDetailRoute() {
  const params = useParams<{ slug: string }>();
  const router = useRouter();
  const slug = String(params?.slug ?? '');

  const known = MARKETING_SERVICES.find(s => s.slug === slug);
  if (!known) {
    notFound();
  }

  const onLogin = () => router.push('/auth?mode=login');
  const onRegister = () => router.push('/auth?mode=register');
  const onAdmin = () => router.push('/auth?mode=login');

  if (slug === 'facebook-automation') {
    return <FacebookProductLanding onLogin={onLogin} onRegister={onRegister} onAdmin={onAdmin} />;
  }

  // Generic coming-soon for Taobao / 1688 (and any future stub).
  return <ComingSoonLanding slug={slug} onLogin={onLogin} onRegister={onRegister} />;
}
