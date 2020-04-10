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
