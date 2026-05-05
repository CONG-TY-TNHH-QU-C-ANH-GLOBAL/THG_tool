var THGContentMeta = globalThis.THGContentMeta || (() => {
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
      if (label.length > 2 && label.length <= 80 && !/^(home|watch|marketplace|groups|friends|menu|notifications|messenger)$/i.test(label)) {
        profileUrl = href;
        displayName = label;
        username = handle;
        break;
      }
    }
    return {
      fb_display_name: displayName,
      fb_username: username,
      fb_profile_url: profileUrl,
      login_email: String(emailInput?.value || '').trim()
    };
  }

  return { collectFacebookMeta };
})();
globalThis.THGContentMeta = THGContentMeta;
