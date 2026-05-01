chrome.runtime.sendMessage({
  type: 'facebook_page_seen',
  url: location.href,
  title: document.title
}).catch(() => {});
