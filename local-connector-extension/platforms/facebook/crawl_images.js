// Original post-photo references only. No downloads, cookies, avatar or link
// preview scraping. Missing/uncertain media is left out rather than guessed.
globalThis.THGCrawlImages = globalThis.THGCrawlImages || (() => {
  function validURL(raw) {
    try {
      const url = new URL(raw);
      return raw.length <= 4096 && url.protocol === 'https:' && !url.username && !url.password && !url.port && !url.hash &&
        /(^|\.)(fbcdn\.net|fbsbx\.com)$/i.test(url.hostname);
    } catch { return false; }
  }

  function fromArticle(article) {
    const result = [];
    for (const img of article.querySelectorAll('a[href*="/photo"] img')) {
      if (img.closest('[role="comment"], h2, h3, header')) continue;
      if (!isPhotoLink(img.closest('a[href]')?.getAttribute('href'))) continue;
      if (!belongsToPost(img, article)) continue;
      const rect = img.getBoundingClientRect();
      if (rect.width < 120 || rect.height < 120) continue;
      const raw = img.currentSrc || img.src || '';
      if (!validURL(raw) || result.includes(raw)) continue;
      result.push(raw);
      if (result.length === 3) break;
    }
    return result;
  }

  function belongsToPost(img, article) {
    const owner = img.closest('[role="article"]');
    if (!owner || owner === article) return true;
    if (article.matches?.('[role="article"]')) return false;
    // A FeedUnit may also contain sibling articles (comments/related posts).
    // Only its first real article can supply the primary post's photos.
    return owner === article.querySelector?.('[role="article"]');
  }

  function isPhotoLink(raw) {
    try {
      const url = new URL(raw || '', 'https://www.facebook.com');
      return url.protocol === 'https:' && /(^|\.)facebook\.com$/i.test(url.hostname) &&
        /^\/photos?(?:\/|\.php|$)/i.test(url.pathname);
    } catch { return false; }
  }

  return Object.freeze({ fromArticle });
})();
if (typeof module !== 'undefined' && module.exports) module.exports = globalThis.THGCrawlImages;
