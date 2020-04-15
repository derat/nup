// Copyright 2020 Daniel Erat.
// All rights reserved.

const dialogManager = document.querySelector('dialog-manager');
const musicPlayer = document.querySelector('music-player');
const musicSearcher = document.querySelector('music-searcher');

// Wire up dependencies between components.
musicPlayer.dialogManager = dialogManager;
musicSearcher.dialogManager = dialogManager;
musicSearcher.musicPlayer = musicPlayer;

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
  showOptions: () => musicPlayer.showOptions_(),
  updateTags: () => musicPlayer.updateTagsFromServer_(true /* sync */),
};
