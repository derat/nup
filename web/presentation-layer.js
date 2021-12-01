// Copyright 2017 Daniel Erat.
// All rights reserved.

import {
  $,
  createShadow,
  createTemplate,
  emptyImg,
  formatTime,
  getFullCoverUrl,
  getScaledCoverUrl,
} from './common.js';

const template = createTemplate(`
<style>
  :host {
    background-color: black;
    display: none;
    height: 100%;
    left: 0;
    position: fixed;
    top: 0;
    width: 100%;
    z-index: 5;
  }
  :host(.visible) {
    display: flex;
  }

  .bg-img {
    height: 100%;
    width: 100%;
    object-fit: cover;
    position: absolute;
  }
  .bg-img.hidden {
    visibility: hidden;
  }

  @keyframes fade-out {
    0% { opacity: 1 }
    100% { opacity: 0 }
  }
  #old-cover {
    animation: fade-out 0.5s ease-in-out;
    opacity: 0;
  }

  #bottom {
    align-items: flex-end;
    bottom: 40px;
    color: white;
    display: flex;
    justify-content: space-between;
    left: 0;
    position: absolute;
    text-shadow: 0 0 8px black;
    white-space: nowrap;
    width: 100%;
    z-index: 1;
  }
  #bottom .artist {
    font-weight: bold;
  }
  #bottom .title {
    font-style: italic;
  }

  #current-details {
    margin-left: 40px;
    max-width: 600px;
  }
  #current-details.hidden {
    visibility: hidden;
  }
  #current-artist, #current-title, #current-album {
    font-size: 24px;
    overflow: hidden;
    padding: 4px 8px;
    text-overflow: ellipsis;
  }
  #progress-border {
    background-color: rgba(0, 0, 0, 0.2);
    border: solid white 1px;
    box-shadow: 0 0 8px rgba(0, 0, 0, 0.5);
    height: 6px;
    margin: 8px;
    width: 360px;
  }
  #progress-bar {
    background-color: white;
    height: 6px;
  }
  #current-times {
    display: flex;
    font-size: 16px;
    justify-content: space-between;
    margin: 0 8px;
    width: 360px;
  }
  #current-times div {
    padding: 2px 4px;
  }

  #next {
    margin-right: 40px;
    max-width: 360px;
  }
  #next.hidden {
    display: none;
  }
  #next-heading {
    font-size: 16px;
    font-weight: bold;
    margin-bottom: 8px;
    padding: 2px 6px;
  }
  #next-body {
    display: flex;
    font-size: 18px;
  }
  #next-details {
    overflow: hidden; /* needed to elide artist/title/album */
  }
  #next-artist, #next-title, #next-album {
    overflow: hidden;
    padding: 3px 6px;
    text-overflow: ellipsis;
  }
  #next-cover {
    box-shadow: 0 0 8px rgba(0, 0, 0, 0.5);
    cursor: pointer;
    height: 80px;
    margin-right: 8px;
    object-fit: cover;
    width: 80px;
  }
  #next-cover.hidden {
    display: none;
  }
</style>

<img id="current-cover" class="bg-img" />
<img id="old-cover" class="bg-img" />

<div id="bottom">
  <div id="current-details">
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
    <div id="next-body">
      <img id="next-cover" />
      <div id="next-details">
        <div id="next-artist" class="artist"></div>
        <div id="next-title" class="title"></div>
        <div id="next-album" class="album"></div>
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

      this.shadow_ = createShadow(this, template);
      this.currentCover_ = $('current-cover', this.shadow_);
      this.oldCover_ = $('old-cover', this.shadow_);
      this.currentDetails_ = $('current-details', this.shadow_);
      this.currentArtist_ = $('current-artist', this.shadow_);
      this.currentTitle_ = $('current-title', this.shadow_);
      this.currentAlbum_ = $('current-album', this.shadow_);
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
      const updateImg = (img, url) => {
        if (url) {
          img.src = url;
          img.classList.remove('hidden');
        } else {
          img.src = emptyImg;
          img.classList.add('hidden');
        }
      };

      if (!currentSong) {
        this.currentDetails_.classList.add('hidden');
      } else {
        this.currentDetails_.classList.remove('hidden');
        this.currentArtist_.innerText = currentSong.artist;
        this.currentTitle_.innerText = currentSong.title;
        this.currentAlbum_.innerText = currentSong.album;

        this.progressBar_.style.width = '0px';
        this.timeDiv_.innerText = '';
        this.durationDiv_.innerText = formatTime(currentSong.length);
        this.duration_ = currentSong.length;
      }

      // Make the "old" cover image display the image that we were just
      // displaying, if any, and then fade out. We swap in a new image to
      // retrigger the fade-out animation.
      const el = this.currentCover_.cloneNode(true);
      el.id = 'old-cover';
      this.oldCover_.parentNode.replaceChild(el, this.oldCover_);
      this.oldCover_ = el;

      // Use the full-resolution cover image.
      updateImg(
        this.currentCover_,
        currentSong ? getFullCoverUrl(currentSong.coverFilename) : ''
      );

      if (!nextSong) {
        this.nextDiv_.classList.add('hidden');
      } else {
        this.nextDiv_.classList.remove('hidden');
        this.nextArtist_.innerText = nextSong.artist;
        this.nextTitle_.innerText = nextSong.title;
        this.nextAlbum_.innerText = nextSong.album;
      }
      updateImg(
        this.nextCover_,
        nextSong ? getScaledCoverUrl(nextSong.coverFilename) : ''
      );

      // Preload the next track's full-resolution cover.
      if (this.visible && nextSong && nextSong.coverFilename) {
        new Image().src = getFullCoverUrl(nextSong.coverFilename);
      }
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
      visible
        ? this.classList.add('visible')
        : this.classList.remove('visible');
      this.visible_ = visible;
    }

    setPlayNextTrackFunction(f) {
      this.playNextTrackFunction_ = f;
    }
  }
);
