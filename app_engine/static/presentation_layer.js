// Copyright 2017 Daniel Erat <dan@erat.org>
// All rights reserved.

function initPresentationLayer() {
  document.presentationLayer = new PresentationLayer();
}

function PresentationLayer() {
  this.overlay = $('presentationOverlay');

  this.currentArtist = $('presentationCurrentArtist');
  this.currentTitle = $('presentationCurrentTitle');
  this.currentAlbum = $('presentationCurrentAlbum');
  this.currentCover = $('presentationCurrentCoverImg');

  this.nextDiv = $('presentationNextDiv');
  this.nextArtist = $('presentationNextArtist');
  this.nextTitle = $('presentationNextTitle');
  this.nextAlbum = $('presentationNextAlbum');
  this.nextCover = $('presentationNextCover');

  this.shown = false;
}

PresentationLayer.prototype.update = function(currentSong, nextSong) {
  this.currentArtist.innerText = currentSong ? currentSong.artist : '';
  this.currentTitle.innerText = currentSong ? currentSong.title : '';
  this.currentAlbum.innerText = currentSong ? currentSong.album : '';
  this.currentCover.src = currentSong && currentSong.coverUrl ?
      currentSong.coverUrl : 'images/missing_cover.png';

  this.nextDiv.className = nextSong ? 'shown' : '';
  this.nextArtist.innerText = nextSong ? nextSong.artist : '';
  this.nextTitle.innerText = nextSong ? nextSong.title : '';
  this.nextAlbum.innerText = nextSong ? nextSong.album : '';
  this.nextCover.src = nextSong && nextSong.coverUrl ?
      nextSong.coverUrl : 'images/missing_cover.png';
};

PresentationLayer.prototype.isShown = function() {
  return this.shown;
};

PresentationLayer.prototype.show = function() {
  addClassName(document.body, 'presenting');
  this.overlay.className = 'shown';
  this.shown = true;
};

PresentationLayer.prototype.hide = function() {
  removeClassName(document.body, 'presenting');
  this.overlay.className = '';
  this.shown = false;
};
