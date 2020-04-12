// Copyright 2020 Daniel Erat.
// All rights reserved.

const musicPlayer = document.querySelector('music-player');
const searchForm = document.querySelector('search-form');

musicPlayer.searchForm = searchForm;
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
