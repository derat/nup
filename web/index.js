// Copyright 2020 Daniel Erat.
// All rights reserved.

import { $, smallCoverSize } from './common.js';
import { isDialogShown } from './dialog.js';
import { isMenuShown } from './menu.js';
import Config from './config.js';

const config = new Config();
const player = document.querySelector('music-player');
const searcher = document.querySelector('music-searcher');

player.config = config;

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
player.addEventListener('tags', (e) => {
  searcher.tags = e.detail.tags;
});
searcher.addEventListener('enqueue', (e) => {
  player.enqueueSongs(
    e.detail.songs,
    e.detail.clearFirst,
    e.detail.afterCurrent,
    e.detail.shuffled
  );
});

// Listening for this here feels gross, but less gross than injecting
// music-player into music-searcher.
document.body.addEventListener('keydown', (e) => {
  if (
    e.key === '/' &&
    !isDialogShown() &&
    !isMenuShown() &&
    !player.updateDivShown
  ) {
    searcher.focusKeywords();
    e.preventDefault();
    e.stopPropagation();
  }
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
  updateTags: async () => await player.updateTagsFromServer_(),
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
