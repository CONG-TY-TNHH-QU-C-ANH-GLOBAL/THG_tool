var THGContentMeta = globalThis.THGContentMeta || (() => {
  // normLabel strips diacritics + lowercases so Vietnamese/English UI labels match
  // the suspicious list regardless of accents ("Ảnh bìa" -> "anh bia").
  const normLabel = (s) => String(s || '')
    .normalize('NFD').replace(/[̀-ͯ]/g, '').replace(/[đĐ]/g, 'd')
    .trim().toLowerCase();

  // Facebook renders profile-LINKED anchors whose aria-label is a UI affordance
  // ("Cover photo", "Profile picture", "See more", …) — these link to the profile
  // but are NOT the person's name. The old extractor took the first such label and
  // saved it as fb_display_name (the "Cover photo" bug). isSuspiciousIdentityLabel
  // rejects them so only a real name survives. (Identity TRUTH is the c_user
  // cookie, captured in the background; display name is metadata — see
  // specs/FACEBOOK_AUTOMATION_RELIABILITY_TRACK.md PR-B.)
  const SUSPICIOUS_LABELS = [
    'cover photo', 'anh bia', 'profile picture', 'anh dai dien',
    'see more', 'xem them', 'menu', 'home', 'watch', 'marketplace', 'groups',
    'friends', 'notifications', 'messenger', 'photo', 'photos', 'anh', 'video',
    'videos', 'reel', 'reels', 'story', 'stories', 'add friend', 'them ban be',
    'message', 'nhan tin', 'follow', 'theo doi', 'like', 'thich', 'share', 'chia se',
    'comment', 'binh luan', 'settings', 'cai dat', 'create', 'tao',
  ];
  function isSuspiciousIdentityLabel(label) {
    const n = normLabel(label);
    if (!n) return true;
    return SUSPICIOUS_LABELS.some((s) => n === s);
  }

  function collectFacebookMeta() {
    const emailInput = document.querySelector('input[name="email"], input[type="email"], input#email');
    let profileUrl = '';
    let displayName = '';
    let username = '';
    const candidates = Array.from(document.querySelectorAll('a[href]'));
    for (const a of candidates) {
      const href = THGContentShared.normalizeHref(a.getAttribute('href'));
      if (!href || !THGContentShared.FACEBOOK_URL_RE.test(href)) continue;
      const label = (a.getAttribute('aria-label') || THGContentShared.textOf(a)).trim();
      const handle = THGContentShared.usernameFromProfileUrl(href);
      if (!label || !handle) continue;
      if (label.length <= 2 || label.length > 80) continue;
      // Skip UI-affordance labels (Cover photo / Ảnh bìa / Profile picture / …);
      // keep scanning for a real name instead of saving the first match.
      if (isSuspiciousIdentityLabel(label)) continue;
      profileUrl = href;
      displayName = label;
      username = handle;
      break;
    }
    return {
      fb_display_name: displayName,
      fb_username: username,
      fb_profile_url: profileUrl,
      login_email: String(emailInput?.value || '').trim()
    };
  }

  return { collectFacebookMeta, isSuspiciousIdentityLabel };
})();
globalThis.THGContentMeta = THGContentMeta;
