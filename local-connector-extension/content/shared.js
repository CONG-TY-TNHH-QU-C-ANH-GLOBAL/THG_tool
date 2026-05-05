var THGContentShared = globalThis.THGContentShared || (() => {
  const FACEBOOK_URL_RE = /^https:\/\/([^/]+\.)?facebook\.com\//i;

  function textOf(node) {
    return String(node?.innerText || node?.textContent || '').replace(/\s+/g, ' ').trim();
  }

  function normalizeHref(href) {
    try {
      return new URL(href, location.origin).toString();
    } catch {
      return '';
    }
  }

  function usernameFromProfileUrl(url) {
    try {
      const parsed = new URL(url);
      if (!/facebook\.com$/i.test(parsed.hostname) && !/\.facebook\.com$/i.test(parsed.hostname)) return '';
      if (parsed.pathname.includes('profile.php')) return '';
      const first = parsed.pathname.split('/').filter(Boolean)[0] || '';
      if (!first || ['groups', 'pages', 'watch', 'marketplace', 'friends', 'messages', 'notifications', 'reel', 'story.php'].includes(first)) return '';
      return first;
    } catch {
      return '';
    }
  }

  return {
    FACEBOOK_URL_RE,
    normalizeHref,
    textOf,
    usernameFromProfileUrl
  };
})();
globalThis.THGContentShared = THGContentShared;
