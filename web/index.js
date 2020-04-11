// Copyright 2020 Daniel Erat.
// All rights reserved.

import {$} from './common.js';
import Config from './config.js';
import Playlist from './playlist.js';

const config = new Config();
const dialogManager = document.querySelector('dialog-manager');
const presentationLayer = document.querySelector('presentation-layer');

const player = document.querySelector('music-player');
player.config = config;
player.dialogManager = dialogManager;
player.presentationLayer = presentationLayer;
player.favicon = $('favicon');

const playlist = new Playlist(player, dialogManager);
player.playlist = playlist; // ugly circular dependency

// Used by browser tests.
document.test = {
  rateAndTag: (songId, rating, tags) =>
    player.updater_.rateAndTag(songId, rating, tags),
  reportPlay: (songId, startTime) =>
    player.updater_.reportPlay(songId, startTime),
  reset: () => {
    player.resetForTesting();
    playlist.resetForTesting();
  },
  showOptions: () => player.showOptions_(),
  updateTags: () => player.updateTagsFromServer_(false),
};
