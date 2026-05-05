'use client';
import { useLang } from '../../i18n/useLang';

/**
 * Bilingual VI/EN toggle pinned to the right side of the topbar per
 * the design-system spec (§2.12). Persists to localStorage and
 * broadcasts via a window event so multiple mounts stay in sync.
 */
export function LangSwitch() {
  const { lang, setLang } = useLang();
  return (
    <div className="lang-switch" role="group" aria-label="Language">
      <button
        type="button"
        className={lang === 'vi' ? 'is-active' : ''}
        onClick={() => setLang('vi')}
      >
        VI
      </button>
      <button
        type="button"
        className={lang === 'en' ? 'is-active' : ''}
        onClick={() => setLang('en')}
      >
        EN
      </button>
    </div>
  );
}
