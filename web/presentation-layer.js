// Copyright 2017 Daniel Erat.
// All rights reserved.

import {$, createElement, createShadow, formatTime} from './common.js';

class PresentationLayer extends HTMLElement {
  constructor() {
    super();

    this.duration_ = 0; // duration of current song in seconds
    this.shown_ = false;
    this.playNextTrackFunction_ = null;
    this.origOverflowStyle_ = document.body.style.overflow;

    // TODO: Is there a better way to do this?
    this.style.cssText = `
      background-color: black;
      display: none;
      font-family: Arial, Helvetica, sans-serif;
      height: 100%;
      position: absolute;
      width: 100%;
      z-index: 5;
    `;
    this.shadow_ = createShadow(this, 'presentation-layer.css');

    const createDiv = (cl, p, t) => createElement('div', cl, p, t);
    const left = createDiv('left', this.shadow_);
    const right = createDiv('right', this.shadow_);

    this.currentCover_ = createElement('img', 'cover', left);

    const current = createDiv('current', right);
    this.currentArtist_ = createDiv('artist', current);
    this.currentTitle_ = createDiv('title', current);
    this.currentAlbum_ = createElement('div', 'album', current);
    this.progressBorder_ = createDiv('progress-border', current);
    this.progressBar_ = createDiv('progress-bar', this.progressBorder_);
    const times = createDiv('times', current);
    this.timeDiv_ = createDiv(undefined, times);
    this.durationDiv_ = createDiv(undefined, times);

    this.nextDiv_ = createDiv('next', right);
    createDiv('heading', this.nextDiv_, 'Next');
    const details = createDiv('details', this.nextDiv_);
    this.nextCover_ = createElement('img', 'cover', details);
    const text = createDiv('text', details);
    this.nextArtist_ = createDiv('artist', text);
    this.nextTitle_ = createDiv('title', text);
    this.nextAlbum_ = createDiv('album', text);

    this.nextCover_.addEventListener(
      'click',
      () => this.playNextTrackFunction_ && this.playNextTrackFunction_(),
      false,
    );
  }

  updateSongs(currentSong, nextSong) {
    this.currentArtist_.innerText = currentSong ? currentSong.artist : '';
    this.currentTitle_.innerText = currentSong ? currentSong.title : '';
    this.currentAlbum_.innerText = currentSong ? currentSong.album : '';
    this.currentCover_.src =
      currentSong && currentSong.coverUrl
        ? currentSong.coverUrl
        : 'images/missing_cover.png';

    nextSong
      ? this.nextDiv_.classList.add('shown')
      : this.nextDiv_.classList.remove('shown');
    this.nextArtist_.innerText = nextSong ? nextSong.artist : '';
    this.nextTitle_.innerText = nextSong ? nextSong.title : '';
    this.nextAlbum_.innerText = nextSong ? nextSong.album : '';
    this.nextCover_.src =
      nextSong && nextSong.coverUrl
        ? nextSong.coverUrl
        : 'images/missing_cover.png';

    this.progressBorder_.style.display = currentSong ? 'block' : 'none';
    this.progressBar_.style.width = '0px';
    this.timeDiv_.innerText = '';
    this.durationDiv_.innerText = currentSong
      ? formatTime(currentSong.length)
      : '';
    this.duration_ = currentSong ? currentSong.length : 0;
  }

  updatePosition(sec) {
    if (isNaN(sec)) return;

    const percent = Math.min((100 * sec) / this.duration_, 100);
    this.progressBar_.style.width = percent + '%';
    this.timeDiv_.innerText = formatTime(sec);
  }

  isShown() {
    return this.shown_;
  }

  show() {
    document.body.style.overflow = 'hidden';
    this.style.display = 'flex';
    this.shown_ = true;
  }

  hide() {
    document.body.style.overflow = this.origOverflowStyle;
    this.style.display = 'none';
    this.shown_ = false;
  }

  setPlayNextTrackFunction(f) {
    this.playNextTrackFunction_ = f;
  }
}

customElements.define('presentation-layer', PresentationLayer);
