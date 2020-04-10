// Copyright 2020 Daniel Erat.
// All rights reserved.

import Config from './config.js';
import DialogManager from './dialog_manager.js';
import Player from './player.js';
import Playlist from './playlist.js';
import PresentationLayer from './presentation_layer.js';

document.config = new Config();
document.dialogManager = new DialogManager();
document.presentationLayer = new PresentationLayer();
document.player = new Player();
document.playlist = new Playlist(document.player);
