'use client';
import { useEffect, useSyncExternalStore } from 'react';
import { useLang } from '../../i18n/useLang';

type Density = 'compact' | 'balanced' | 'airy';
const STORAGE_KEY = 'autoflow_density';
const EVENT = 'autoflow:density';

function read(): Density {
  if (typeof window === 'undefined') return 'balanced';
  const saved = window.localStorage.getItem(STORAGE_KEY);
  if (saved === 'compact' || saved === 'balanced' || saved === 'airy') return saved;
  return 'balanced';
}

function subscribe(listener: () => void) {
  if (typeof window === 'undefined') return () => {};
  window.addEventListener(EVENT, listener);
  window.addEventListener('storage', listener);
  return () => {
    window.removeEventListener(EVENT, listener);
    window.removeEventListener('storage', listener);
  };
}

/**
 * Three-step density toggle (compact / balanced / airy). Sets
 * `data-density` on <html>, which `tokens.css` reads to swap the
 * `--section-pad` and `--row-pad` variables — no per-component code
 * needs to know about density modes.
 */
export function DensitySwitch() {
  const density = useSyncExternalStore(subscribe, read, () => 'balanced' as Density);
  const { t } = useLang();

  useEffect(() => {
    if (typeof document === 'undefined') return;
    document.documentElement.dataset.density = density;
  }, [density]);

  function set(next: Density) {
    if (typeof window === 'undefined') return;
    window.localStorage.setItem(STORAGE_KEY, next);
    window.dispatchEvent(new Event(EVENT));
  }

  return (
    <div className="lang-switch" role="group" aria-label={t.density.label} title={t.density.label}>
      <button type="button" className={density === 'compact' ? 'is-active' : ''} onClick={() => set('compact')}>
        {t.density.compact}
      </button>
      <button type="button" className={density === 'balanced' ? 'is-active' : ''} onClick={() => set('balanced')}>
        {t.density.balanced}
      </button>
      <button type="button" className={density === 'airy' ? 'is-active' : ''} onClick={() => set('airy')}>
        {t.density.airy}
      </button>
    </div>
  );
}
