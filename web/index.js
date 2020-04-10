// Copyright 2020 Daniel Erat.
// All rights reserved.

import Config from './config.js';
import DialogManager from './dialog_manager.js';
import Player from './player.js';
import Playlist from './playlist.js';

document.config = new Config();
document.dialogManager = new DialogManager();
document.player = new Player();
document.playlist = new Playlist(document.player);
