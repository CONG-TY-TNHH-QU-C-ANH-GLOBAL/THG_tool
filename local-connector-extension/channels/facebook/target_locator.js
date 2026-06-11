// Facebook target parsing (ADAPTER-owned; platform CORE never parses URLs). Maps a Facebook URL
// to a neutral target { channel, action_hint, kind, id, url }. Facebook-specific id extraction
// lives HERE — the regexes mirror content/outbound.js extractPostIdFromUrl; a later cleanup PR
// points the executor at this canonical parser instead of keeping its own copy.
var THGChannelFacebook = globalThis.THGChannelFacebook || (globalThis.THGChannelFacebook = {});
THGChannelFacebook.targetLocator = THGChannelFacebook.targetLocator || (() => {
  function idFromUrl(rawUrl) {
    try {
      const url = new URL(rawUrl, 'https://www.facebook.com');
      const path = url.pathname || '';
      const m = path.match(/\/posts\/(\d+)/) || path.match(/\/permalink\/(\d+)/)
        || path.match(/\/videos\/(\d+)/) || path.match(/\/reel\/(\d+)/);
      if (m) return m[1];
      const story = url.searchParams.get('story_fbid'); if (story && /^\d+$/.test(story)) return story;
      const v = url.searchParams.get('v'); if (v && /^\d+$/.test(v)) return v;
      const fbid = url.searchParams.get('fbid'); if (fbid && /^\d+$/.test(fbid)) return fbid;
      return '';
    } catch (e) { return ''; }
  }
  function kindOf(rawUrl) {
    const s = String(rawUrl || '');
    if (s.indexOf('/groups/') !== -1) return 'group_post';
    if (s.indexOf('/watch') !== -1 || s.indexOf('/videos/') !== -1 || s.indexOf('/reel/') !== -1) return 'video';
    if (s.indexOf('story_fbid=') !== -1 || s.indexOf('/permalink/') !== -1 || s.indexOf('/posts/') !== -1) return 'permalink';
    return 'unknown';
  }
  function parse(rawUrl, actionType) {
    return { channel: 'facebook', action_hint: actionType || 'comment', kind: kindOf(rawUrl), id: idFromUrl(rawUrl), url: String(rawUrl || '') };
  }
  return { parse, idFromUrl, kindOf };
})();
if (typeof module !== 'undefined' && module.exports) module.exports = THGChannelFacebook.targetLocator;
