var THGContentMeta = globalThis.THGContentMeta || (() => {
  // normLabel strips diacritics + lowercases so Vietnamese/English markers match
  // regardless of accents ("Ảnh bìa" -> "anh bia").
  const normLabel = (s) => String(s || '')
    .normalize('NFD').replace(/[̀-ͯ]/g, '').replace(/[đĐ]/g, 'd')
    .trim().toLowerCase();

  // EXACT UI-affordance labels Facebook puts on profile-linked anchors that are
  // NOT a person's name ("Cover photo", "See more", …). Matched whole-string.
  const SUSPICIOUS_LABELS = [
    'cover photo', 'anh bia', 'profile picture', 'anh dai dien',
    'see more', 'xem them', 'menu', 'home', 'watch', 'marketplace', 'groups',
    'friends', 'notifications', 'messenger', 'photo', 'photos', 'anh', 'video',
    'videos', 'reel', 'reels', 'story', 'stories', 'add friend', 'them ban be',
    'message', 'nhan tin', 'follow', 'theo doi', 'like', 'thich', 'share', 'chia se',
    'comment', 'binh luan', 'settings', 'cai dat', 'create', 'tao',
  ];
  // SUBSTRING markers of Facebook AUTO-GENERATED image alt-text — these leak as a
  // "name" when a /photo/ anchor's aria-label is the image description, e.g.
  // "May be an image of text that says ...". Matched as contains() since the tail
  // is dynamic. This is the 0.5.30 miss: B1 only did exact-match.
  const ALTTEXT_MARKERS = [
    'may be an image', 'may be a ', 'image of text', 'image may contain',
    'no photo description', 'co the la hinh anh', 'co the la anh',
    'khong co mo ta anh', 'khong co mo ta nao', 'hinh anh co the co',
  ];
  // Reserved first-path segments that are NOT a profile handle. A /photo/ or
  // /story.php/ link parses to handle "photo"/"story.php" — never a username.
  const RESERVED_HANDLES = new Set([
    'photo', 'photo.php', 'story.php', 'permalink.php', 'watch', 'reel', 'reels',
    'groups', 'media', 'events', 'marketplace', 'pages', 'pg', 'profile.php',
    'sharer', 'sharer.php', 'login.php', 'help', 'settings', 'bookmarks',
    'notifications', 'messages', 'gaming', 'live', 'business', 'ads',
  ]);

  function isReservedHandle(handle) {
    return !handle || RESERVED_HANDLES.has(normLabel(handle));
  }
  // isLikelyPersonName rejects empty, too-short/long, exact UI labels, and
  // auto-generated image alt-text — so only a genuine name survives.
  function isLikelyPersonName(label) {
    const n = normLabel(label);
    if (!n || n.length <= 2 || n.length > 80) return false;
    if (SUSPICIOUS_LABELS.some((s) => n === s)) return false;
    if (ALTTEXT_MARKERS.some((m) => n.includes(m))) return false;
    return true;
  }
  // Kept for the golden smoke + back-compat: "is this label NOT a usable name?"
  function isSuspiciousIdentityLabel(label) {
    return !isLikelyPersonName(label);
  }

  function currentCUser() {
    const m = (document.cookie || '').match(/(?:^|;\s*)c_user=(\d+)/);
    return m ? m[1] : '';
  }

  function collectFacebookMeta() {
    const emailInput = document.querySelector('input[name="email"], input[type="email"], input#email');
    const myId = currentCUser();
    const candidates = Array.from(document.querySelectorAll('a[href]'));

    const pick = (href, label, handle) => ({
      fb_display_name: label,
      fb_username: isReservedHandle(handle) ? '' : handle,
      fb_profile_url: href,
      login_email: String(emailInput?.value || '').trim(),
    });

    // Pass 1 (RELIABLE): the anchor that links to the LOGGED-IN user's OWN
    // profile — its href carries c_user (profile.php?id=<c_user> or /<c_user>).
    // Its label is the real name. Identity TRUTH is the c_user cookie; this just
    // attaches a trustworthy display name to it.
    if (myId) {
      for (const a of candidates) {
        const href = THGContentShared.normalizeHref(a.getAttribute('href'));
        if (!href || !THGContentShared.FACEBOOK_URL_RE.test(href)) continue;
        if (!(href.includes('id=' + myId) || href.includes('/' + myId))) continue;
        const label = (a.getAttribute('aria-label') || THGContentShared.textOf(a)).trim();
        if (isLikelyPersonName(label)) {
          return pick(href, label, THGContentShared.usernameFromProfileUrl(href));
        }
      }
    }

    // Pass 2 (FALLBACK): the first GENUINE profile anchor (real handle, not a
    // /photo//story link) whose label looks like a person name. Skips image
    // alt-text and UI affordances.
    for (const a of candidates) {
      const href = THGContentShared.normalizeHref(a.getAttribute('href'));
      if (!href || !THGContentShared.FACEBOOK_URL_RE.test(href)) continue;
      const handle = THGContentShared.usernameFromProfileUrl(href);
      if (isReservedHandle(handle)) continue;
      const label = (a.getAttribute('aria-label') || THGContentShared.textOf(a)).trim();
      if (!isLikelyPersonName(label)) continue;
      return pick(href, label, handle);
    }

    // Nothing trustworthy — leave the name EMPTY. The backend/UI shows the
    // fb_user_id (the truth) instead of garbage. Never emit alt-text.
    return { fb_display_name: '', fb_username: '', fb_profile_url: '', login_email: String(emailInput?.value || '').trim() };
  }

  return { collectFacebookMeta, isSuspiciousIdentityLabel, isLikelyPersonName, isReservedHandle };
})();
globalThis.THGContentMeta = THGContentMeta;
