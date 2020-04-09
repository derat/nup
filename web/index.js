// Copyright 2020 Daniel Erat.
// All rights reserved.

window.addEventListener('DOMContentLoaded', () => {
  document.config = new Config();
  document.dialogManager = new DialogManager();
  document.presentationLayer = new PresentationLayer();
  document.player = new Player();
  document.playlist = new Playlist(document.player);
});
