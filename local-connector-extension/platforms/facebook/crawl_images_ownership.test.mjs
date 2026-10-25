import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
const require = createRequire(import.meta.url);
const images = require('./crawl_images.js');

function imageIn(owner, source) {
  return {
    currentSrc: source,
    getBoundingClientRect: () => ({ width: 300, height: 300 }),
    closest(selector) {
      if (selector.startsWith('[role="comment"]')) return null;
      if (selector === '[role="article"]') return owner;
      if (selector === 'a[href]') return { getAttribute: () => '/photo/?fbid=123' };
      return null;
    },
  };
}

test('a FeedUnit never mixes sibling article photos into the first post', () => {
  const first = { parentElement: { closest: () => null } };
  const sibling = { parentElement: { closest: () => null } };
  const ownPhoto = 'https://scontent.fbcdn.net/own.jpg';
  const wrapper = {
    matches: () => false,
    querySelector: () => first,
    querySelectorAll: () => [imageIn(first, ownPhoto), imageIn(sibling, 'https://scontent.fbcdn.net/other.jpg')],
    contains: node => node === first || node === sibling,
  };
  assert.deepEqual(images.fromArticle(wrapper), [ownPhoto]);
});

test('an article does not adopt its first nested comment article', () => {
  const article = { matches: () => true };
  const comment = { parentElement: { closest: () => article } };
  const ownPhoto = 'https://scontent.fbcdn.net/own.jpg';
  article.querySelector = () => comment;
  article.querySelectorAll = () => [imageIn(article, ownPhoto), imageIn(comment, 'https://scontent.fbcdn.net/comment.jpg')];
  assert.deepEqual(images.fromArticle(article), [ownPhoto]);
});
