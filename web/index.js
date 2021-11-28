// Copyright 2020 Daniel Erat.
// All rights reserved.

import { $, scaledCoverSize } from './common.js';
import Config from './config.js';

const config = new Config();
const musicPlayer = document.querySelector('music-player');
const musicSearcher = document.querySelector('music-searcher');
const overlayManager = document.querySelector('overlay-manager');

// Wire up dependencies between components.
musicPlayer.config = config;
musicPlayer.overlayManager = overlayManager;
musicSearcher.overlayManager = overlayManager;
musicSearcher.musicPlayer = musicPlayer;

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

// Hide the scrollbar while the presentation layer is shown.
musicPlayer.addEventListener('present', (e) => {
  e.detail.visible
    ? document.body.classList.add('no-scroll')
    : document.body.classList.remove('no-scroll');
});

// Use the cover art as the favicon.
musicPlayer.addEventListener('cover', (e) => {
  const favicon = $('favicon');
  if (e.detail.url) {
    favicon.href = e.detail.url;
    favicon.type = 'image/jpeg';
    favicon.sizes = `${scaledCoverSize}x${scaledCoverSize}`;
  } else {
    favicon.href = 'favicon.ico';
    favicon.type = 'image/png';
    favicon.sizes = '48x48';
  }
});

// Used by browser tests.
document.test = {
  rateAndTag: (songId, rating, tags) =>
    musicPlayer.updater_.rateAndTag(songId, rating, tags),
  reportPlay: (songId, startTime) =>
    musicPlayer.updater_.reportPlay(songId, startTime),
  reset: () => {
    musicPlayer.resetForTesting();
    musicSearcher.resetForTesting();
  },
  setPlayDelayMs: (delayMs) => (musicPlayer.playDelayMs_ = delayMs),
  showOptions: () => musicPlayer.showOptions_(),
  updateTags: async () => await musicPlayer.updateTagsFromServer_(),
};
