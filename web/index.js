// Copyright 2020 Daniel Erat.
// All rights reserved.

// Needed by common.js for Firefox and Safari.
import './construct-style-sheets-polyfill.js';

import { $, commonStyles, handleFetchError, smallCoverSize } from './common.js';
import Config, { getConfig } from './config.js';
import { isDialogShown } from './dialog.js';
import { isMenuShown } from './menu.js';

document.adoptedStyleSheets = [commonStyles];

// Import web components so they'll be included in the bundle.
// If we weren't bundling, it'd be faster to load these from index.html.
import './audio-wrapper.js';
import './music-player.js';
import './music-searcher.js';
import './presentation-layer.js';
import './song-table.js';
import './tag-suggester.js';

const config = getConfig();
const player = document.querySelector('music-player');
const searcher = document.querySelector('music-searcher');

// Watch for theme changes.
const darkMediaQuery = '(prefers-color-scheme: dark)';
const updateTheme = (theme) => {
  let dark = false;
  switch (theme) {
    case Config.THEME_AUTO:
      dark = window.matchMedia(darkMediaQuery).matches;
      break;
    case Config.THEME_LIGHT:
      break;
    case Config.THEME_DARK:
      dark = true;
      break;
  }
  if (dark) document.documentElement.setAttribute('data-theme', 'dark');
  else document.documentElement.removeAttribute('data-theme');
};
config.addCallback((k, v) => k === Config.THEME && updateTheme(v));
window.matchMedia(darkMediaQuery).addListener((e) => updateTheme());
updateTheme(config.get(Config.THEME));

// Tags known by the server.
let serverTags = [];

// Returns a promise that will be resolved once tags are fetched.
const fetchServerTags = () =>
  fetch('tags', { method: 'GET' })
    .then((res) => handleFetchError(res))
    .then((res) => res.json())
    .then((tags) => {
      console.log(`Fetched ${tags.length} tag(s)`);
      serverTags = player.tags = searcher.tags = tags;
    })
    .catch((err) => {
      console.error(`Failed fetching tags: ${err}`);
    });
fetchServerTags();

// Use the cover art as the favicon.
player.addEventListener('cover', (e) => {
  const favicon = $('favicon');
  if (e.detail.url) {
    favicon.href = e.detail.url;
    // The server can fall back to JPEG here if it doesn't have a WebP image
    // at the requested size, but I'm guessing that the browser will sniff the
    // type anyway.
    favicon.type = 'image/webp';
    favicon.sizes = `${smallCoverSize}x${smallCoverSize}`;
  } else {
    favicon.href = 'favicon.ico';
    favicon.type = 'image/png';
    favicon.sizes = '48x48';
  }
});

// Wire up components.
player.addEventListener('field', (e) => {
  searcher.resetFields(e.detail.artist, e.detail.album, e.detail.albumId);
});
player.addEventListener('newtags', (e) => {
  serverTags = serverTags.concat(e.detail.tags);
  player.tags = searcher.tags = serverTags;
});
searcher.addEventListener('enqueue', (e) => {
  player.enqueueSongs(
    e.detail.songs,
    e.detail.clearFirst,
    e.detail.afterCurrent,
    e.detail.shuffled
  );
});

// Used by web tests.
document.test = {
  reset: () => {
    player.resetForTesting();
    searcher.resetForTesting();
    // Make a hacky attempt to close any modal dialogs.
    [...document.querySelectorAll('dialog')].forEach((d) => d.close());
  },
  setPlayDelayMs: (delayMs) => (player.playDelayMs_ = delayMs),
  updateTags: async () => await fetchServerTags(),
  dragElement: (src, dest, offsetX, offsetY) => {
    const dataTransfer = { setDragImage: () => {} };
    let dropEffect = 'none';
    Object.defineProperty(dataTransfer, 'dropEffect', {
      get: () => dropEffect,
      set: (v) => (dropEffect = v),
    });

    const makeEvent = (type, clientX, clientY) => {
      const ev = new DragEvent(type, {
        bubbles: true,
        cancelable: true,
        composed: true, // trigger listeners outside of shadow root
      });
      // https://stackoverflow.com/a/39066443
      Object.defineProperty(ev, 'dataTransfer', { value: dataTransfer });
      Object.defineProperty(ev, 'clientX', { value: clientX });
      Object.defineProperty(ev, 'clientY', { value: clientY });
      return ev;
    };

    const srcRect = src.getBoundingClientRect();
    const srcX = srcRect.x + srcRect.width / 2;
    const srcY = srcRect.y + srcRect.height / 2;
    const destRect = dest.getBoundingClientRect();
    const destX = destRect.x + destRect.width / 2 + (offsetX ?? 0);
    const destY = destRect.y + destRect.height / 2 + (offsetY ?? 0);

    src.dispatchEvent(makeEvent('dragstart', srcX, srcY));
    dest.dispatchEvent(makeEvent('dragenter', destX, destY));
    dest.dispatchEvent(makeEvent('dragover', destX, destY));
    dest.dispatchEvent(makeEvent('dragend', destX, destY));
  },
};
