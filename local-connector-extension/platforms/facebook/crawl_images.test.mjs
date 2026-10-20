import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
const require = createRequire(import.meta.url);
const images = require('./crawl_images.js');

function photo(owner, src, opts = {}) {
  return {
    currentSrc: src, naturalWidth: 800,
    getBoundingClientRect: () => ({ width: opts.width || 300, height: 300 }),
    closest(selector) {
      if (selector.startsWith('[role="comment"]')) return opts.comment ? {} : null;
      if (selector === '[role="article"]') return opts.nested ? { parentElement: { closest: () => owner } } : owner;
      if (selector === 'a[href]') return { getAttribute: () => opts.link || '/photo/?fbid=123&set=gm.456' };
      return null;
    },
  };
}

test('keeps original post photos; excludes avatars, comments, foreign links and invalid hosts', () => {
  const article = {};
  const good = 'https://scontent.fbcdn.net/good.jpg?token=reference';
  article.querySelectorAll = () => [
    photo(article, good, { width: 40 }),
    photo(article, good, { comment: true }),
    photo(article, good, { nested: true }),
    photo(article, good, { link: 'https://foreign.example/photo/1' }),
    photo(article, 'https://fbcdn.net.evil.example/image.jpg'),
    photo(article, good), photo(article, good),
  ];
  assert.deepEqual(images.fromArticle(article), [good]);
});

test('caps original photo references at three', () => {
  const article = {};
  article.querySelectorAll = () => [1, 2, 3, 4].map(id => photo(article, `https://scontent.fbcdn.net/${id}.jpg`));
  assert.equal(images.fromArticle(article).length, 3);
});

test('FeedUnit wrapper can expose its first real article without accepting nested comments', () => {
  const post = { parentElement: { closest: () => null } };
  const wrapper = { contains: node => node === post };
  const good = 'https://scontent.fbcdn.net/post.jpg';
  const commentOwner = { parentElement: { closest: () => post } };
  const commentPhoto = photo(commentOwner, 'https://scontent.fbcdn.net/comment.jpg');
  wrapper.querySelectorAll = () => [photo(post, good), commentPhoto];
  assert.deepEqual(images.fromArticle(wrapper), [good]);
});
