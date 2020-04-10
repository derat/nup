// Copyright 2020 Daniel Erat.
// All rights reserved.

import Config from './config.js';
import Player from './player.js';
import Playlist from './playlist.js';

const config = new Config();
const player = new Player(config);
const playlist = new Playlist(player);
player.setPlaylist(playlist);
