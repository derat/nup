// Copyright 2020 Daniel Erat.
// All rights reserved.

import { $, scaledCoverSize } from './common.js';

const dialogManager = document.querySelector('dialog-manager');
const musicPlayer = document.querySelector('music-player');
const musicSearcher = document.querySelector('music-searcher');

// Wire up dependencies between components.
musicPlayer.dialogManager = dialogManager;
musicSearcher.dialogManager = dialogManager;
musicSearcher.musicPlayer = musicPlayer;

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
  updateTags: () => musicPlayer.updateTagsFromServer_(true /* sync */),
};
