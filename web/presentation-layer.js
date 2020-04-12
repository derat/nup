// Copyright 2017 Daniel Erat.
// All rights reserved.

import {$, createShadow, createTemplate, formatTime} from './common.js';

const template = createTemplate(`
<style>
  :host {
    background-color: black;
    display: none;
    font-family: Arial, Helvetica, sans-serif;
    height: 100%;
    position: fixed;
    width: 100%;
    z-index: 5;
  }

  #left {
    width: calc(60% - 40px);
    height: calc(100% - 40px);
    max-height: calc(100% - 40px);
    margin: 20px;
  }
  #current-cover {
    width: 100%;
    height: 100%;
    object-fit: contain;
  }

  #right {
    width: calc(40% - 41px);
    display: flex;
    flex-direction: column;
    justify-content: space-between;
    margin: 60px 20px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  #right .artist {
    font-weight: bold;
  }
  #right .album {
    font-style: italic;
  }
  #current div {
    color: white;
    font-size: 24px;
    margin-bottom: 8px;
  }
  #progress-border {
    width: 80%;
    height: 6px;
    margin-top: 16px;
    border: solid white 1px;
  }
  #progress-bar {
    height: 6px;
    background-color: white;
  }
  #current-times {
    display: flex;
    font-size: 16px;
    justify-content: space-between;
    width: 80%;
  }
  #next {
    display: none;
  }
  #next.visible {
    display: block;
  }
  #next div {
    margin-top: 4px;
  }
  #next-heading {
    color: #999;
    font-size: 16px;
    font-weight: bold;
    margin-bottom: 10px;
  }
  #next-details {
    color: white;
    display: flex;
    font-size: 18px;
  }
  #next-cover {
    cursor: pointer;
    height: 80px;
    margin-right: 16px;
    object-fit: contain;
    width: 80px;
  }
</style>

<div id="left">
  <img id="current-cover" />
</div>

<div id="right">
  <div id="current">
    <div id="current-artist" class="artist"></div>
    <div id="current-title" class="title"></div>
    <div id="current-album" class="album"></div>
    <div id="progress-border">
      <div id="progress-bar"></div>
    </div>
    <div id="current-times">
      <div id="current-time"></div>
      <div id="current-duration"></div>
    </div>
  </div>
  <div id="next">
    <div id="next-heading">Next</div>
    <div id="next-details">
      <img id="next-cover" />
      <div>
        <div id="next-artist"></div>
        <div id="next-title"></div>
        <div id="next-album"></div>
      </div>
    </div>
  </div>
</div>
`);

// <presentation-layer> is a simple fullscreen overlay displaying information
// about the current and next song.
//
// When the next track's cover image is clicked, a 'next' event is emitted.
customElements.define(
  'presentation-layer',
  class extends HTMLElement {
    constructor() {
      super();

      this.duration_ = 0; // duration of current song in seconds
      this.visible_ = false;
      this.playNextTrackFunction_ = null;
      this.origOverflowStyle_ = document.body.style.overflow;

      this.shadow_ = createShadow(this, template);
      this.currentCover_ = $('current-cover', this.shadow_);
      this.currentArtist_ = $('current-artist', this.shadow_);
      this.currentTitle_ = $('current-title', this.shadow_);
      this.currentAlbum_ = $('current-album', this.shadow_);
      this.progressBorder_ = $('progress-border', this.shadow_);
      this.progressBar_ = $('progress-bar', this.shadow_);
      this.timeDiv_ = $('current-time', this.shadow_);
      this.durationDiv_ = $('current-duration', this.shadow_);
      this.nextDiv_ = $('next', this.shadow_);
      this.nextCover_ = $('next-cover', this.shadow_);
      this.nextArtist_ = $('next-artist', this.shadow_);
      this.nextTitle_ = $('next-title', this.shadow_);
      this.nextAlbum_ = $('next-album', this.shadow_);

      this.nextCover_.addEventListener('click', () => {
        this.dispatchEvent(new Event('next'));
        this.playNextTrackFunction_ && this.playNextTrackFunction_();
      });

      this.updateSongs(null, null);
    }

    updateSongs(currentSong, nextSong) {
      this.currentCover_.src = currentSong ? currentSong.coverUrl : '';
      this.currentCover_.style.display = currentSong ? 'block' : 'none';
      this.currentArtist_.innerText = currentSong ? currentSong.artist : '';
      this.currentTitle_.innerText = currentSong ? currentSong.title : '';
      this.currentAlbum_.innerText = currentSong ? currentSong.album : '';

      nextSong
        ? this.nextDiv_.classList.add('visible')
        : this.nextDiv_.classList.remove('visible');
      this.nextArtist_.innerText = nextSong ? nextSong.artist : '';
      this.nextTitle_.innerText = nextSong ? nextSong.title : '';
      this.nextAlbum_.innerText = nextSong ? nextSong.album : '';
      this.nextCover_.src = nextSong ? nextSong.coverUrl : '';

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

    get visible() {
      return this.visible_;
    }
    set visible(visible) {
      document.body.style.overflow = visible
        ? 'hidden'
        : this.origOverflowStyle;
      this.style.display = visible ? 'flex' : 'none';
      this.visible_ = visible;
    }

    setPlayNextTrackFunction(f) {
      this.playNextTrackFunction_ = f;
    }
  },
);
