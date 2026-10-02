import i18next from 'i18next';
import { initReactI18next } from 'react-i18next';

import { LanguageManager } from '@/utils';
import enUS from '../../../internal/web/translation/en-US.json';

const FALLBACK = 'en-US';

const lazyModules = import.meta.glob([
  '../../../internal/web/translation/*.json',
  '!../../../internal/web/translation/en-US.json',
]);

function moduleKeyFor(code: string): string {
  return `../../../internal/web/translation/${code}.json`;
}

let active: string = LanguageManager.getLanguage();
if (active !== FALLBACK && !Object.prototype.hasOwnProperty.call(lazyModules, moduleKeyFor(active))) {
  active = FALLBACK;
}

function applyDocumentLanguage(code: string) {
  if (typeof document === 'undefined') return;
  // Lets the stylesheet pick the right font stack (Vazirmatn-first for
  // Persian/Arabic) and keeps assistive tech informed. Direction is left
  // untouched on purpose: the panel's layouts are LTR-only today.
  document.documentElement.setAttribute('lang', code);
}

export async function readyI18n() {
  applyDocumentLanguage(active);
  await i18next.use(initReactI18next).init({
    lng: active,
    fallbackLng: FALLBACK,
    resources: { [FALLBACK]: { translation: enUS } },
    interpolation: { escapeValue: false, prefix: '{', suffix: '}' },
    returnNull: false,
  });
  if (active !== FALLBACK) {
    const loader = lazyModules[moduleKeyFor(active)] as (() => Promise<{ default: Record<string, unknown> }>) | undefined;
    if (loader) {
      const mod = await loader();
      const messages = (mod.default ?? mod) as Record<string, unknown>;
      i18next.addResourceBundle(active, 'translation', messages, true, true);
      await i18next.changeLanguage(active);
    }
  }
  return i18next;
}

export { i18next as i18n };
