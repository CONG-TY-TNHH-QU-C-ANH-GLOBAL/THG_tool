'use client';
import { useLang } from '../../autoflow/i18n/useLang';
import { KNOWLEDGE_STRINGS, type KnowledgeStrings } from './strings';

export function useKnowledgeStrings(): { t: KnowledgeStrings; lang: 'vi' | 'en' } {
  const { lang } = useLang();
  return { t: KNOWLEDGE_STRINGS[lang], lang };
}
