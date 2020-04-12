// Copyright 2020 Daniel Erat.
// All rights reserved.

import {$} from './common.js';
import Config from './config.js';

const config = new Config();
const dialogManager = document.querySelector('dialog-manager');
const musicPlayer = document.querySelector('music-player');
const searchForm = document.querySelector('search-form');

musicPlayer.config = config;
musicPlayer.dialogManager = dialogManager;
musicPlayer.searchForm = searchForm;

searchForm.dialogManager = dialogManager;
searchForm.musicPlayer = musicPlayer;

// Used by browser tests.
document.test = {
  rateAndTag: (songId, rating, tags) =>
    musicPlayer.updater_.rateAndTag(songId, rating, tags),
  reportPlay: (songId, startTime) =>
    musicPlayer.updater_.reportPlay(songId, startTime),
  reset: () => {
    musicPlayer.resetForTesting();
    searchForm.reset(null, null, true /* clearResults */);
  },
  showOptions: () => musicPlayer.showOptions_(),
  updateTags: () => musicPlayer.updateTagsFromServer_(true /* sync */),
};
