import type { CommentEligibility } from '../../types';

export interface EligibilityLine {
  text: string;
  tone: 'ok' | 'warn' | 'mute';
}

// eligibilityLine turns the backend §6 projection into one display line. The
// backend is the source of truth for WHY a lead is (in)eligible — the UI only
// composes the dynamic "Có thể comment bằng <n>…" line for the eligible case and
// otherwise renders the backend's precise Vietnamese reason. No gate is
// re-derived here, so the dashboard can never disagree with comment_all_leads.
export function eligibilityLine(e?: CommentEligibility): EligibilityLine | null {
  if (!e) return null;
  if (e.eligibility_state === 'eligible') {
    return {
      text: `Có thể comment bằng ${e.eligible_actor_count} tài khoản Facebook sẵn sàng.`,
      tone: 'ok',
    };
  }
  return {
    text: e.ineligibility_message_vi || 'Chưa đủ điều kiện comment.',
    tone: e.eligibility_state === 'no_ready_account' ? 'warn' : 'mute',
  };
}
