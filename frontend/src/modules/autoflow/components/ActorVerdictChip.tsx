'use client';

// Shared Verified-Actor chip — the SINGLE place that renders an actor verdict, so
// the Comment tab and the Lead tab agree on identity. Takes the raw verdict +
// blocked flag (from execution_attempts / account_runtime_state); business copy
// only, the raw code never shows.
interface Props {
  actorVerdict?: string; // verified | mismatch | unknown | ''
  actorBlocked?: boolean;
  lang?: 'vi' | 'en';
}

export function ActorVerdictChip({ actorVerdict, actorBlocked, lang = 'vi' }: Props) {
  if (actorBlocked) {
    return (
      <span className="tag tag-hot" title={lang === 'vi' ? 'Tài khoản bị chặn auto do sai danh tính FB — cần operator gỡ' : 'Account blocked from auto-execute on actor mismatch'}>
        {lang === 'vi' ? '⚠ Sai actor — đã chặn' : '⚠ Actor mismatch — blocked'}
      </span>
    );
  }
  switch (actorVerdict) {
    case 'verified':
      return <span className="tag tag-ok" title={lang === 'vi' ? 'Danh tính FB khớp tài khoản kỳ vọng' : 'FB identity matched the expected account'}>{lang === 'vi' ? '✅ Đúng actor' : '✅ Verified actor'}</span>;
    case 'mismatch':
      return <span className="tag tag-hot">{lang === 'vi' ? '⚠ Sai actor' : '⚠ Actor mismatch'}</span>;
    case 'unknown':
      return <span className="tag tag-mute" title={lang === 'vi' ? 'Chưa xác minh được danh tính FB' : 'FB identity not yet verifiable'}>{lang === 'vi' ? '❔ Chưa xác minh' : '❔ Unverified'}</span>;
    default:
      return null;
  }
}
