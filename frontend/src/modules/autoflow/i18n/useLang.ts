'use client';
import { useEffect, useSyncExternalStore } from 'react';
import { STRINGS, type Lang, type DashboardStrings } from './strings';

const STORAGE_KEY = 'autoflow_lang';
const EVENT = 'autoflow:lang';

function readLang(): Lang {
  if (typeof window === 'undefined') return 'vi';
  const saved = window.localStorage.getItem(STORAGE_KEY);
  if (saved === 'vi' || saved === 'en') return saved;
  // Mirror the browser preference on first run; we still default to VI for
  // operators who haven't expressed a preference because the product copy
  // is Vietnamese-first.
  if (typeof navigator !== 'undefined' && navigator.language?.toLowerCase().startsWith('en')) {
    return 'en';
  }
  return 'vi';
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

export function useLang(): { lang: Lang; t: DashboardStrings; setLang: (l: Lang) => void } {
  const lang = useSyncExternalStore<Lang>(subscribe, readLang, () => 'vi');

  // Mirror language onto <html lang> for accessibility and screen-reader
  // pronunciation hints. SSR-safe via the typeof window guard above.
  useEffect(() => {
    if (typeof document !== 'undefined') {
      document.documentElement.lang = lang;
    }
  }, [lang]);

  return {
    lang,
    t: STRINGS[lang],
    setLang(next) {
      if (typeof window === 'undefined') return;
      window.localStorage.setItem(STORAGE_KEY, next);
      window.dispatchEvent(new Event(EVENT));
    },
  };
}

export type { Lang } from './strings';
