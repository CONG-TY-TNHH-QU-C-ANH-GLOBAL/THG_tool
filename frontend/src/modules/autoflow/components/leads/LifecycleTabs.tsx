import type { LifecycleTab } from '../../types';
import { LIFECYCLE_TABS } from '../../types';

// Lifecycle tabs (spec: specs/LEAD_LIFECYCLE_WORK_QUEUE.md, PR-4). The four work-management
// groups. `stale` is intentionally absent — hidden by default. Labels are inlined bilingual
// to avoid growing the i18n god file (strings.ts).
const LABELS: Record<LifecycleTab, { vi: string; en: string }> = {
  active: { vi: 'Cần xử lý', en: 'Needs action' },
  waiting_reply: { vi: 'Chờ phản hồi', en: 'Awaiting reply' },
  followup_due: { vi: 'Đến hạn follow-up', en: 'Follow-up due' },
  archived: { vi: 'Đã lưu trữ', en: 'Archived' },
};

interface Props {
  active: LifecycleTab;
  counts: Record<LifecycleTab, number>;
  onSelect: (tab: LifecycleTab) => void;
  lang: 'vi' | 'en';
}

export function LifecycleTabs({ active, counts, onSelect, lang }: Props) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
      {LIFECYCLE_TABS.map((tab) => (
        <button
          key={tab}
          type="button"
          className={`filter-pill ${active === tab ? 'is-active' : ''}`}
          style={{ display: 'flex', justifyContent: 'space-between', textAlign: 'left' }}
          onClick={() => onSelect(tab)}
        >
          <span>{LABELS[tab][lang]}</span>
          <span style={{ opacity: 0.7 }}>{counts[tab]}</span>
        </button>
      ))}
    </div>
  );
}
