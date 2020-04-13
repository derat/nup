// Copyright 2020 Daniel Erat.
// All rights reserved.

const dialogManager = document.querySelector('dialog-manager');
const musicPlayer = document.querySelector('music-player');
const searchForm = document.querySelector('search-form');

// Wire up dependencies between components.
musicPlayer.dialogManager = dialogManager;
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
    searchForm.resetForTesting();
  },
  showOptions: () => musicPlayer.showOptions_(),
  updateTags: () => musicPlayer.updateTagsFromServer_(true /* sync */),
};
