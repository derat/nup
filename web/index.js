// Copyright 2020 Daniel Erat.
// All rights reserved.

import {$} from './common.js';
import Config from './config.js';
import Player from './player.js';
import Playlist from './playlist.js';

const dialogManager = $('dialogManager');

const config = new Config();
const player = new Player(config, dialogManager, $('presentationLayer'));
const playlist = new Playlist(player, dialogManager);
player.setPlaylist(playlist); // ugly circular dependency

// Used by browser tests.
document.test = {
  rateAndTag: (songId, rating, tags) =>
    player.updater.rateAndTag(songId, rating, tags),
  reportPlay: (songId, startTime) =>
    player.updater.reportPlay(songId, startTime),
  reset: () => playlist.resetForTesting(),
  showOptions: () => player.showOptions(),
  updateTags: () => player.updateTagsFromServer(false),
};
