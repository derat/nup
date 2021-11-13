// Copyright 2010 Daniel Erat.
// All rights reserved.

import {
  $,
  createShadow,
  createTemplate,
  emptyImg,
  formatTime,
  getCurrentTimeSec,
  getScaledCoverUrl,
  getSongUrl,
  handleFetchError,
  updateTitleAttributeForTruncation,
} from './common.js';
import Config from './config.js';
import OptionsDialog from './options-dialog.js';
import Updater from './updater.js';

const template = createTemplate(`
<style>
  @import 'common.css';
  :host {
    display: flex;
    flex-direction: column;
  }
  #song-info {
    display: flex;
    margin: var(--margin);
  }

  #cover-div {
    align-items: center;
    display: flex;
    justify-content: center;
    margin-right: var(--margin);
  }
  #cover-div.empty {
    background-color: var(--cover-missing-color);
    outline: solid 1px var(--border-color);
    outline-offset: -1px;
  }
  #cover-img {
    cursor: pointer;
    height: 70px;
    object-fit: cover;
    user-select: none;
    width: 70px;
  }
  #cover-div.empty #cover-img {
    /* Make fully transparent rather than using visibility: hidden so the
     * image will still be clickable even if the cover is missing. */
    opacity: 0;
  }
  #rating-overlay {
    color: #fff;
    font-family: var(--icon-font-family);
    font-size: 12px;
    left: calc(var(--margin) + 2px);
    letter-spacing: 2px;
    pointer-events: none;
    position: absolute;
    text-shadow: 0 0 8px #000;
    top: calc(var(--margin) + 55px);
    user-select: none;
  }

  #details {
    line-height: 1.3;
    overflow: hidden;
    white-space: nowrap;
  }
  #artist {
    font-weight: bold;
  }
  #title {
    font-style: italic;
  }
  #controls {
    margin: var(--margin);
    margin-top: 0;
    user-select: none;
  }
  #controls button {
    font-family: var(--icon-font-family);
    font-size: 10px;
    width: 44px;
  }
  #controls > *:not(:first-child) {
    margin-left: var(--button-spacing);
  }
  #update-container {
    background-color: var(--bg-color);
    border: solid 1px var(--frame-border-color);
    border-radius: 4px;
    box-shadow: 0 1px 4px 1px rgba(0, 0, 0, 0.3);
    display: none;
    left: var(--margin);
    padding: 8px;
    position: absolute;
    top: var(--margin);
    z-index: 2;
  }
  #update-container.shown {
    display: block;
  }
  #update-close {
    cursor: pointer;
    position: absolute;
    right: 5px;
    top: 5px;
  }
  #rating {
    font-family: var(--icon-font-family);
    font-size: 16px;
  }
  #rating a.star {
    color: var(--text-color);
    cursor: pointer;
    display: inline-block;
    min-width: 17px; /* black and white stars have different sizes :-/ */
    opacity: 0.6;
  }
  #rating a.star:hover {
    opacity: 0.9;
  }
  a.debug-link {
    color: var(--text-color);
    font-family: Arial, Helvetica, sans-serif;
    font-size: 10px;
    opacity: 0.7;
    text-decoration: none;
  }
  #edit-tags {
    font-family: Arial, Helvetica, sans-serif;
    height: 48px;
    margin-bottom: -4px;
    margin-top: 8px;
    resize: none;
    width: 220px;
  }
  #edit-tags-suggester {
    bottom: 52px;
    left: 4px;
    max-height: 26px;
    max-width: 210px;
    position: absolute;
  }
</style>

<presentation-layer></presentation-layer>

<audio type="audio/mpeg" preload="auto">
  Your browser doesn't support the audio element.
</audio>

<div id="song-info">
  <div id="cover-div">
    <img id="cover-img" />
    <div id="rating-overlay"></div>
  </div>
  <div id="details">
    <div id="artist"></div>
    <div id="title"></div>
    <div id="album"></div>
    <div id="time"></div>
  </div>
</div>

<div id="controls">
  <button id="prev" disabled title="Previous song">⏮</button>
  <button id="play-pause" disabled title="Pause">⏸</button>
  <button id="next" disabled title="Next song">⏭</button>
</div>

<div id="update-container">
  <span id="update-close" class="x-icon" title="Close"></span>
  <div id="rating-container">
    Rating: <span id="rating" tabindex="0"></span>
    <a id="dump-song" class="debug-link" target="_blank">[d]</a>
  </div>
  <tag-suggester id="edit-tags-suggester">
    <textarea id="edit-tags" slot="text"></textarea>
  </tag-suggester>
</div>

<song-table id="playlist"></song-table>
`);

// <music-player> plays and displays information about songs. It also maintains
// and displays a playlist. Songs can be enqueued by calling enqueueSongs().
//
// When the list of available tags changes, a 'tags' CustomEvent with a
// 'detail.tags' property containing a string array of the new tags is emitted.
//
// When an artist or album field in the playlist is clicked, a 'field'
// CustomEvent is emitted. See <song-table> for more details.
//
// When the <presentation-layer>'s visibility changes, a 'present' CustomEvent
// is emitted with a 'detail.visible' boolean property. The document's scrollbar
// should be hidden while the layer is visible.
//
// When the current cover art changes due to a song change, a 'cover'
// CustomEvent is emitted with a 'detail.url' string property corresponding to a
// URL to the scaled JPEG image. This property is null if no cover art is
// available.
customElements.define(
  'music-player',
  class extends HTMLElement {
    static SEEK_SEC_ = 10; // seconds skipped by seeking forward or back
    static MAX_RETRIES_ = 2; // number of consecutive playback errors to reply
    static NOTIFICATION_SEC_ = 3; // duration for song-change notification
    static PLAY_DELAY_MS_ = 500; // delay before playing when cycling track
    static GAIN_CHANGE_SEC_ = 0.1; // duration for audio gain change between songs
    static PRELOAD_SEC_ = 20; // seconds from end of song when next song should be loaded

    constructor() {
      super();

      this.updater_ = new Updater();
      this.optionsDialog_ = null;

      this.songs_ = []; // songs in the order in which they should be played
      this.tags_ = []; // available tags loaded from server
      this.currentIndex_ = -1; // index into |songs| of current track
      this.lastUpdateTime_ = -1; // seconds since epoch for last onTimeUpdate_()
      this.lastUpdatePosition_ = 0; // playback position at last onTimeUpdate_()
      this.lastUpdateSong_ = null; // current song at last onTimeUpdate_()
      this.numErrors_ = 0; // consecutive playback errors
      this.startTime_ = -1; // seconds since epoch when current track started
      this.totalPlayedSec_ = 0; // total seconds playing current song
      this.reportedCurrentTrack_ = false; // already reported current as played?
      this.reachedEndOfSongs_ = false; // did we hit end of last song?
      this.updateSong_ = null; // song playing when update div was opened
      this.updatedRating_ = -1.0; // rating set in update div
      this.notification_ = null; // song notification currently shown
      this.closeNotificationTimeoutId_ = null; // for closeNotification_()
      this.playDelayMs_ = this.constructor.PLAY_DELAY_MS_;
      this.playTimeoutId_ = 0; // for playInternal_()

      this.shadow_ = createShadow(this, template);
      const get = (id) => $(id, this.shadow_);

      this.presentationLayer_ =
        this.shadow_.querySelector('presentation-layer');
      this.presentationLayer_.addEventListener('next', () => {
        this.cycleTrack_(1, false /* delayPlay */);
      });

      this.audioCtx_ = new AudioContext();
      this.gainNode_ = this.audioCtx_.createGain();
      this.gainNode_.connect(this.audioCtx_.destination);

      this.audio_ = this.shadow_.querySelector('audio');
      this.configureAudio_();
      this.nextAudio_ = null;

      this.coverDiv_ = get('cover-div');
      this.coverImage_ = get('cover-img');
      this.coverImage_.addEventListener('click', () => this.showUpdateDiv_());
      this.coverImage_.addEventListener('load', () =>
        this.updateMediaSessionMetadata_(true /* imageLoaded */)
      );

      this.ratingOverlayDiv_ = get('rating-overlay');
      this.artistDiv_ = get('artist');
      this.titleDiv_ = get('title');
      this.albumDiv_ = get('album');
      this.timeDiv_ = get('time');

      this.prevButton_ = get('prev');
      this.prevButton_.addEventListener('click', () =>
        this.cycleTrack_(-1, true /* delayPlay */)
      );
      this.nextButton_ = get('next');
      this.nextButton_.addEventListener('click', () =>
        this.cycleTrack_(1, true /* delayPlay */)
      );
      this.playPauseButton_ = get('play-pause');
      this.playPauseButton_.addEventListener('click', () =>
        this.togglePause_()
      );

      this.updateDiv_ = get('update-container');
      get('update-close').addEventListener('click', () =>
        this.hideUpdateDiv_(true)
      );
      this.ratingSpan_ = get('rating');
      this.ratingSpan_.addEventListener('keydown', (e) =>
        this.handleRatingSpanKeyDown_(e)
      );
      this.dumpSongLink_ = get('dump-song');
      this.tagsTextarea_ = get('edit-tags');
      this.tagSuggester_ = get('edit-tags-suggester');

      this.playlistTable_ = get('playlist');
      this.playlistTable_.addEventListener('field', (e) => {
        this.dispatchEvent(new CustomEvent('field', { detail: e.detail }));
      });
      this.playlistTable_.addEventListener('menu', (e) => {
        if (!this.dialogManager_) throw new Error('No <dialog-manager>');

        const idx = e.detail.index;
        const orig = e.detail.orig;
        orig.preventDefault();

        const menu = this.dialogManager_.createMenu(orig.pageX, orig.pageY, [
          {
            id: 'play',
            text: 'Play',
            cb: () => this.selectTrack_(idx),
          },
          {
            id: 'remove',
            text: 'Remove',
            cb: () => this.removeSongs_(idx, 1),
          },
          {
            id: 'truncate',
            text: 'Truncate',
            cb: () => this.removeSongs_(idx, this.songs_.length - idx),
          },
          {
            id: 'debug',
            text: 'Debug',
            cb: () => window.open(getDumpSongUrl(e.detail.songId), '_blank'),
          },
        ]);

        // Highlight the playlist row, and then remove the highlighting when the
        // menu is closed.
        this.playlistTable_.setRowMenuShown(idx, true);
        menu.addEventListener('close', () => {
          this.playlistTable_.setRowMenuShown(idx, false);
        });
      });

      if ('mediaSession' in navigator) {
        const ms = navigator.mediaSession;
        ms.setActionHandler('play', () => this.play_(false /* delay */));
        ms.setActionHandler('pause', () => this.pause_());
        ms.setActionHandler('seekbackward', () =>
          this.seek_(-this.constructor.SEEK_SEC_)
        );
        ms.setActionHandler('seekforward', () =>
          this.seek_(this.constructor.SEEK_SEC_)
        );
        ms.setActionHandler('previoustrack', () =>
          this.cycleTrack_(-1, true /* delayPlay */)
        );
        ms.setActionHandler('nexttrack', () =>
          this.cycleTrack_(1, true /* delayPlay */)
        );
      }

      document.body.addEventListener('keydown', (e) => {
        if (this.processAccelerator_(e)) {
          e.preventDefault();
          e.stopPropagation();
        }
      });
      window.addEventListener('beforeunload', () => {
        this.closeNotification_();
        return null;
      });

      this.updateTagsFromServer_();
      this.updateSongDisplay_();
    }

    // Adds event handlers to |audio_| and routes it through |gainNode_|.
    configureAudio_() {
      this.audio_.addEventListener('ended', () => this.onEnded_());
      this.audio_.addEventListener('pause', () => this.onPause_());
      this.audio_.addEventListener('play', () => this.onPlay_());
      this.audio_.addEventListener('timeupdate', () => this.onTimeUpdate_());
      this.audio_.addEventListener('error', (e) => this.onError_(e));

      this.audioSrc_ = this.audioCtx_.createMediaElementSource(this.audio_);
      this.audioSrc_.connect(this.gainNode_);
    }

    // Replaces |audio_| with |audio|.
    swapAudio_(audio) {
      this.audioSrc_.disconnect(this.gainNode_);
      this.audio_.removeAttribute('src');
      this.audio_.parentNode.replaceChild(audio, this.audio_);
      this.audio_ = audio;
      this.configureAudio_(); // resets |audioSrc_|
    }

    set config(config) {
      this.config_ = config;
      this.config_.addCallback((name, value) => {
        if (name === Config.GAIN_TYPE || name === Config.PRE_AMP) {
          this.updateGain_();
        }
      });
    }

    set dialogManager(manager) {
      this.dialogManager_ = manager;
    }

    // Requests known tags from the server and updates the internal list.
    // Returns a promise for completion of the task.
    updateTagsFromServer_() {
      return fetch('tags', { method: 'GET' })
        .then((res) => handleFetchError(res))
        .then((res) => res.json())
        .then((tags) => {
          this.updateTags_(tags);
          console.log(`Loaded ${tags.length} tag(s)`);
        })
        .catch((err) => {
          console.error(`Failed loading tags: ${err}`);
        });
    }

    get currentSong_() {
      return this.songs_[this.currentIndex_] || null;
    }

    resetForTesting() {
      this.hideUpdateDiv_(false /* saveChanges */);
      this.enqueueSongs([], true);
    }

    // Adds |songs| to the playlist.
    // If |clearFirst| is true, the existing playlist is cleared first.
    // If |afterCurrent| is true, |songs| are inserted immediately after the
    // current song. Otherwise, they are appended to the end of the playlist.
    enqueueSongs(songs, clearFirst, afterCurrent) {
      if (clearFirst) this.removeSongs_(0, this.songs_.length);

      let index = afterCurrent
        ? Math.min(this.currentIndex_ + 1, this.songs_.length)
        : this.songs_.length;
      songs.forEach((s) => this.songs_.splice(index++, 0, s));

      this.playlistTable_.setSongs(this.songs_);

      if (this.currentIndex_ == -1) {
        this.selectTrack_(0, false /* delayPlay */);
      } else if (this.reachedEndOfSongs_) {
        this.cycleTrack_(1, false /* delayPlay */);
      } else {
        this.updateButtonState_();
        this.updatePresentationLayerSongs_();
      }
    }

    // Removes |len| songs starting at index |start| from the playlist.
    removeSongs_(start, len) {
      if (start < 0 || len <= 0 || start + len > this.songs_.length) return;

      this.songs_.splice(start, len);
      this.playlistTable_.setSongs(this.songs_);

      // If we're keeping the current song, things are pretty simple.
      const end = start + len - 1;
      if (start > this.currentIndex_ || end < this.currentIndex_) {
        // If we're removing songs before the current one, we need to update the
        // index and highlighting.
        if (end < this.currentIndex_) {
          this.playlistTable_.setRowActive(this.currentIndex_, false);
          this.currentIndex_ -= len;
          this.playlistTable_.setRowActive(this.currentIndex_, true);
        }
        this.updateButtonState_();
        this.updatePresentationLayerSongs_();
        return;
      }

      // Stop playing the (just-removed) current song and choose a new one.
      this.audio_.pause();
      this.audio_.removeAttribute('src');
      this.playlistTable_.setRowActive(this.currentIndex_, false);
      this.currentIndex_ = -1;

      // If there are songs after the last-removed one, switch to the first of
      // them.
      if (this.songs_.length > start) {
        this.selectTrack_(start, false /* delayPlay */);
        return;
      }

      // Otherwise, we truncated the playlist, i.e. we deleted all songs from
      // the currently-playing one to the end. Jump to the last song.
      // TODO: Pausing is hokey. It'd probably be better to act as if we'd
      // actually reached the end of the last song, but that'd probably require
      // waiting for its duration to be loaded so we can seek.
      this.selectTrack_(this.songs_.length, false /* delayPlay */);
      this.pause_();
    }

    // Plays the song at |offset| in the playlist relative to the current song.
    // If |delayPlay| is true, waits a bit before actually playing the audio
    // (in case the user might be about to select a different track).
    cycleTrack_(offset, delayPlay) {
      this.selectTrack_(this.currentIndex_ + offset, delayPlay);
    }

    // Plays the song at |index| in the playlist.
    // If |delayPlay| is true, waits a bit before actually playing the audio
    // (in case the user might be about to select a different track).
    selectTrack_(index, delayPlay) {
      if (!this.songs_.length) {
        this.currentIndex_ = -1;
        this.updateSongDisplay_();
        this.updateButtonState_();
        this.updatePresentationLayerSongs_();
        this.reachedEndOfSongs_ = false;
        return;
      }

      if (index < 0) index = 0;
      else if (index >= this.songs_.length) index = this.songs_.length - 1;

      if (index == this.currentIndex_) return;

      this.playlistTable_.setRowActive(this.currentIndex_, false);
      this.playlistTable_.setRowActive(index, true);
      this.playlistTable_.scrollToRow(index);
      this.currentIndex_ = index;

      this.updateSongDisplay_();
      this.updatePresentationLayerSongs_();
      this.updateButtonState_();
      this.play_(delayPlay);

      if (!document.hasFocus()) this.showNotification_();
    }

    updateTags_(tags) {
      this.tags_ = tags;
      this.tagSuggester_.words = tags;
      this.dispatchEvent(new CustomEvent('tags', { detail: { tags } }));
    }

    updateButtonState_() {
      this.prevButton_.disabled = this.currentIndex_ <= 0;
      this.nextButton_.disabled =
        this.currentIndex_ < 0 || this.currentIndex_ >= this.songs_.length - 1;
      this.playPauseButton_.disabled = this.currentIndex_ < 0;
    }

    updateSongDisplay_() {
      const song = this.currentSong_;
      document.title = song ? `${song.artist} - ${song.title}` : 'Player';

      this.artistDiv_.innerText = song ? song.artist : '';
      this.titleDiv_.innerText = song ? song.title : '';
      this.albumDiv_.innerText = song ? song.album : '';
      this.timeDiv_.innerText = '';

      updateTitleAttributeForTruncation(
        this.artistDiv_,
        song ? song.artist : ''
      );
      updateTitleAttributeForTruncation(this.titleDiv_, song ? song.title : '');
      updateTitleAttributeForTruncation(this.albumDiv_, song ? song.album : '');

      if (song && song.coverFilename) {
        const url = getScaledCoverUrl(song.coverFilename);
        this.coverImage_.src = url;
        this.coverDiv_.classList.remove('empty');
        this.emitCoverEvent_(url);
      } else {
        this.coverImage_.src = emptyImg;
        this.coverDiv_.classList.add('empty');
        this.emitCoverEvent_(null);
      }

      // Cache the scaled cover images for the next song and the one after it.
      // This prevents ugly laggy updates here and in <presentation-layer>.
      // TODO: Except this probably won't work due to App Engine caching:
      // https://github.com/derat/nup/issues/1
      const precacheCover = (s) => {
        if (!s || !s.coverFilename) return;
        new Image().src = getScaledCoverUrl(s.coverFilename);
      };
      precacheCover(this.songs_[this.currentIndex_ + 1]);
      precacheCover(this.songs_[this.currentIndex_ + 2]);

      this.updateCoverTitleAttribute_();
      this.updateRatingOverlay_();
      // Metadata will be updated again after |coverImage_| is loaded.
      this.updateMediaSessionMetadata_(false /* imageLoaded */);
    }

    updateCoverTitleAttribute_() {
      const song = this.currentSong_;
      if (!song) {
        this.coverImage_.title = '';
        return;
      }

      let text = getRatingString(song.rating, true, true);
      if (song.tags.length > 0) text += '\nTags: ' + song.tags.sort().join(' ');
      this.coverImage_.title = text;
    }

    updateRatingOverlay_() {
      const song = this.currentSong_;
      this.ratingOverlayDiv_.innerText =
        song && song.rating >= 0.0
          ? getRatingString(song.rating, false, false)
          : '';
    }

    updateMediaSessionMetadata_(imageLoaded) {
      if (!('mediaSession' in navigator)) return;

      const song = this.currentSong_;
      if (!song) {
        navigator.mediaSession.metadata = null;
        return;
      }

      const data = {
        title: song.title,
        artist: song.artist,
        album: song.album,
      };
      if (imageLoaded) {
        const img = this.coverImage_;
        data.artwork = [
          {
            src: img.src,
            sizes: `${img.naturalWidth}x${img.naturalHeight}`,
            type: 'image/jpeg',
          },
        ];
      }
      navigator.mediaSession.metadata = new MediaMetadata(data);
    }

    updatePresentationLayerSongs_() {
      this.presentationLayer_.updateSongs(
        this.currentSong_,
        this.songs_[this.currentIndex_ + 1] || null
      );
    }

    showNotification_() {
      if (!('Notification' in window)) return;

      if (Notification.permission !== 'granted') {
        if (Notification.permission !== 'denied') {
          Notification.requestPermission();
        }
        return;
      }

      this.closeNotification_();
      if (this.closeNotificationTimeoutId_ !== null) {
        window.clearTimeout(this.closeNotificationTimeoutId_);
        this.closeNotificationTimeoutId_ = null;
      }

      const song = this.currentSong_;
      if (!song) return;

      const options = {
        body: `${song.title}\n${song.album}\n${formatTime(song.length)}`,
      };
      if (song.coverFilename) {
        options.icon = getScaledCoverUrl(song.coverFilename);
      }
      this.notification_ = new Notification(`${song.artist}`, options);
      this.closeNotificationTimeoutId_ = window.setTimeout(() => {
        this.closeNotificationTimeoutId_ = null;
        this.closeNotification_();
      }, this.constructor.NOTIFICATION_SEC_ * 1000);
    }

    closeNotification_() {
      if (this.notification_) this.notification_.close();
      this.notification_ = null;
    }

    // Starts playback. If |currentSong_| isn't being played, switches to it
    // even if we were already playing. Also restarts playback if we were
    // stopped at the end of the last song in the playlist.
    //
    // If |delay| is true, waits a bit before loading media and playing;
    // otherwise starts playing immediately.
    play_(delay) {
      if (this.playTimeoutId_ !== undefined) {
        window.clearTimeout(this.playTimeoutId_);
        this.playTimeoutId_ = undefined;
      }

      if (delay) {
        console.log(`Playing in ${this.playDelayMs_} ms`);
        this.playTimeoutId_ = window.setTimeout(() => {
          this.playTimeoutId_ = undefined;
          this.playInternal_();
        }, this.playDelayMs_);
      } else {
        this.playInternal_();
      }
    }

    // Internal method called by play_().
    playInternal_() {
      const song = this.currentSong_;
      // Get an absolute URL since that's what we'll get from the <audio>
      // element: https://stackoverflow.com/a/44547904
      const url = getSongUrl(song.filename);
      if (this.audio_.src != url || this.reachedEndOfSongs_) {
        // Deal with "The AudioContext was not allowed to start. It must be
        // resumed (or created) after a user gesture on the page.":
        // https://developers.google.com/web/updates/2017/09/autoplay-policy-changes#webaudio
        const ctx = this.gainNode_.context;
        if (ctx.state === 'suspended') ctx.resume();

        if (
          this.nextAudio_ &&
          this.nextAudio_.src === url &&
          this.nextAudio_.error === null
        ) {
          console.log(`Starting preloaded ${song.songId} (${url})`);
          this.swapAudio_(this.nextAudio_);
        } else {
          console.log(`Starting ${song.songId} (${url})`);
          this.audio_.src = url;
          this.audio_.currentTime = 0;
        }
        this.nextAudio_ = null;

        this.lastUpdateTime_ = -1;
        this.lastUpdatePosition_ = 0;
        this.lastUpdateSong_ = null;
        this.numErrors_ = 0;
        this.startTime_ = getCurrentTimeSec();
        this.totalPlayedSec_ = 0;
        this.reportedCurrentTrack_ = false;
        this.reachedEndOfSongs_ = false;
        this.updateGain_();
      }

      console.log('Playing');
      this.audio_.play().catch((e) => {
        // play() actually returns a promise that is resolved after playback
        // actually starts. If we change the <audio>'s src or call its pause()
        // method while in the preparatory state, it complains. Ignore those
        // errors.
        // https://developers.google.com/web/updates/2017/06/play-request-was-interrupted
        if (
          e.name == 'AbortError' &&
          (e.message.match(/interrupted by a new load request/) ||
            e.message.match(/interrupted by a call to pause/))
        ) {
          return;
        }
        throw e;
      });
    }

    // Pauses playback. Safe to call if already paused or stopped.
    pause_() {
      console.log('Pausing');
      this.audio_.pause();
    }

    togglePause_() {
      this.audio_.paused ? this.play_(false /* delay */) : this.pause_();
    }

    seek_(seconds) {
      if (!this.audio_.seekable) return;

      const newTime = Math.max(this.audio_.currentTime + seconds, 0);
      if (newTime < this.audio_.duration) this.audio_.currentTime = newTime;
    }

    onEnded_() {
      if (this.currentIndex_ >= this.songs_.length - 1) {
        this.reachedEndOfSongs_ = true;
      } else {
        this.cycleTrack_(1, false /* delay */);
      }
    }

    onPause_() {
      this.playPauseButton_.innerText = '▶';
      this.playPauseButton_.title = 'Play';
      this.lastUpdateTime_ = -1;
    }

    onPlay_() {
      this.playPauseButton_.innerText = '⏸';
      this.playPauseButton_.title = 'Pause';
      this.lastUpdateTime_ = getCurrentTimeSec();
    }

    onTimeUpdate_() {
      const song = this.currentSong_;
      const pos = this.audio_.currentTime;
      const dur = song ? song.length : this.audio_.duration;

      // Avoid resetting |numErrors_| if we get called repeatedly without making
      // any progress.
      if (song == this.lastUpdateSong_ && pos == this.lastUpdatePosition_) {
        return;
      }

      const now = getCurrentTimeSec();
      if (this.lastUpdateTime_ > 0) {
        // Playback can hang if the network is flaky, so make sure that we don't
        // incorrectly increment the total played time by the wall time if the
        // position didn't move as much: https://github.com/derat/nup/issues/20
        const timeDiff = now - this.lastUpdateTime_;
        const posDiff = pos - this.lastUpdatePosition_;
        this.totalPlayedSec_ += Math.max(Math.min(timeDiff, posDiff), 0);
      }

      this.lastUpdateTime_ = now;
      this.lastUpdatePosition_ = pos;
      this.lastUpdateSong_ = song;
      this.numErrors_ = 0;

      if (
        !this.reportedCurrentTrack_ &&
        (this.totalPlayedSec_ >= 240 || this.totalPlayedSec_ > dur / 2)
      ) {
        this.updater_.reportPlay(song.songId, this.startTime_);
        this.reportedCurrentTrack_ = true;
      }

      this.timeDiv_.innerText = dur
        ? `[ ${formatTime(pos)} / ${formatTime(dur)} ]`
        : '';
      this.presentationLayer_.updatePosition(pos);

      // Preload the next song once we're nearing the end of this one.
      if (
        pos >= dur - this.constructor.PRELOAD_SEC_ &&
        !this.nextAudio_ &&
        this.currentIndex_ < this.songs_.length - 1
      ) {
        this.preloadSong_(this.songs_[this.currentIndex_ + 1]);
      }
    }

    // Configures |nextAudio_| to play |song|.
    preloadSong_(song) {
      const url = getSongUrl(song.filename);
      console.log(`Preloading ${song.songId} (${url})`);
      this.nextAudio_ = this.audio_.cloneNode(true);
      this.nextAudio_.src = url;
    }

    onError_(e) {
      this.numErrors_++;

      const error = e.target.error;
      console.log(`Got playback error ${error.code} (${error.message})`);
      switch (error.code) {
        case error.MEDIA_ERR_ABORTED: // 1
          break;
        case error.MEDIA_ERR_NETWORK: // 2
        case error.MEDIA_ERR_DECODE: // 3
        case error.MEDIA_ERR_SRC_NOT_SUPPORTED: // 4
          if (this.numErrors_ <= this.constructor.MAX_RETRIES_) {
            console.log(`Retrying from position ${this.lastUpdatePosition_}`);
            this.audio_.load();
            this.audio_.currentTime = this.lastUpdatePosition_;
            this.audio_.play();
          } else {
            console.log(`Giving up after ${this.numErrors_} errors`);
            this.cycleTrack_(1, false /* delay */);
          }
          break;
      }
    }

    showUpdateDiv_() {
      const song = this.currentSong_;
      if (!song) return false;

      // Already shown.
      if (this.updateSong_) return true;

      this.setRating_(song.rating);
      this.dumpSongLink_.href = getDumpSongUrl(song.songId);
      this.tagsTextarea_.value = song.tags.length
        ? song.tags.sort().join(' ') + ' ' // append space to ease editing
        : '';
      this.updateDiv_.classList.add('shown');
      this.updateSong_ = song;
      return true;
    }

    hideUpdateDiv_(saveChanges) {
      this.updateDiv_.classList.remove('shown');
      this.ratingSpan_.blur();
      this.tagsTextarea_.blur();

      const song = this.updateSong_;
      this.updateSong_ = null;

      if (!song || !saveChanges) return;

      const ratingChanged = this.updatedRating_ != song.rating;

      const newRawTags = this.tagsTextarea_.value.trim().split(/\s+/);
      let newTags = [];
      const createdTags = [];
      for (let i = 0; i < newRawTags.length; ++i) {
        let tag = newRawTags[i].toLowerCase();
        if (tag === '') continue;
        if (this.tags_.indexOf(tag) != -1 || song.tags.indexOf(tag) != -1) {
          newTags.push(tag);
        } else if (tag[0] == '+' && tag.length > 1) {
          tag = tag.substring(1);
          newTags.push(tag);
          if (this.tags_.indexOf(tag) == -1) createdTags.push(tag);
        } else {
          console.log(`Skipping unknown tag "${tag}"`);
        }
      }
      // Remove duplicates.
      newTags = newTags
        .sort()
        .filter((item, pos, self) => self.indexOf(item) == pos);
      const tagsChanged = newTags.join(' ') != song.tags.sort().join(' ');

      if (createdTags.length > 0) {
        this.updateTags_(this.tags_.concat(createdTags));
      }

      if (!ratingChanged && !tagsChanged) return;

      this.updater_.rateAndTag(
        song.songId,
        ratingChanged ? this.updatedRating_ : null,
        tagsChanged ? newTags : null
      );

      song.rating = this.updatedRating_;
      song.tags = newTags;

      this.updateCoverTitleAttribute_();
      if (ratingChanged) this.updateRatingOverlay_();
    }

    setRating_(rating) {
      this.updatedRating_ = rating;

      // Initialize the stars the first time we show them.
      if (!this.ratingSpan_.hasChildNodes()) {
        for (let i = 1; i <= 5; ++i) {
          const anchor = document.createElement('a');
          const rating = numStarsToRating(i);
          anchor.addEventListener(
            'click',
            () => this.setRating_(rating),
            false
          );
          anchor.className = 'star';
          this.ratingSpan_.appendChild(anchor);
        }
      }

      const numStars = ratingToNumStars(rating);
      for (let i = 1; i <= 5; ++i) {
        this.ratingSpan_.childNodes[i - 1].innerText =
          i <= numStars ? '\u2605' : '\u2606';
      }
    }

    showOptions_() {
      if (this.optionsDialog_) return;
      if (!this.config_) throw new Error('No config');
      if (!this.dialogManager_) throw new Error('No <dialog-manager>');

      this.optionsDialog_ = new OptionsDialog(
        this.config_,
        this.dialogManager_,
        () => {
          this.optionsDialog_ = null;
        }
      );
    }

    // Adjusts |gainNode_|'s gain appropriately for the current song and
    // settings. This implements the approach described at
    // https://wiki.hydrogenaud.io/index.php?title=ReplayGain_specification.
    updateGain_() {
      let adj = this.config_.get(Config.PRE_AMP); // decibels

      const song = this.currentSong_;
      if (song) {
        const gainType = this.config_.get(Config.GAIN_TYPE);
        if (gainType === Config.GAIN_ALBUM) {
          adj += song.albumGain || 0;
        } else if (gainType === Config.GAIN_TRACK) {
          adj += song.trackGain || 0;
        }
      }

      let scale = 10 ** (adj / 20);

      // TODO: Add an option to prevent clipping instead of always doing this?
      if (song && song.peakAmp !== undefined) {
        scale = Math.min(scale, 1 / song.peakAmp);
      }

      // Per https://developer.mozilla.org/en-US/docs/Web/API/GainNode:
      // "If modified, the new gain is instantly applied, causing unaesthetic
      // 'clicks' in the resulting audio. To prevent this from happening, never
      // change the value directly but use the exponential interpolation methods
      // on the AudioParam interface."
      console.log(`Scaling amplitude by ${scale.toFixed(3)}`);
      this.gainNode_.gain.exponentialRampToValueAtTime(
        scale,
        this.audioCtx_.currentTime + this.constructor.GAIN_CHANGE_SEC_
      );
    }

    processAccelerator_(e) {
      if (this.dialogManager_ && this.dialogManager_.numChildren) return false;

      if (e.altKey && e.key == 'd') {
        const song = this.currentSong_;
        if (song) window.open(getDumpSongUrl(song.songId), '_blank');
        return true;
      } else if (e.altKey && e.key == 'n') {
        this.cycleTrack_(1, true /* delay */);
        return true;
      } else if (e.altKey && e.key == 'o') {
        this.showOptions_();
        return true;
      } else if (e.altKey && e.key == 'p') {
        this.cycleTrack_(-1, true /* delay */);
        return true;
      } else if (e.altKey && e.key == 'r') {
        if (this.showUpdateDiv_()) this.ratingSpan_.focus();
        return true;
      } else if (e.altKey && e.key == 't') {
        if (this.showUpdateDiv_()) this.tagsTextarea_.focus();
        return true;
      } else if (e.altKey && e.key == 'v') {
        this.presentationLayer_.visible = !this.presentationLayer_.visible;
        this.emitPresentEvent_(this.presentationLayer_.visible);
        return true;
      } else if (e.key == ' ' && !this.updateSong_) {
        this.togglePause_();
        return true;
      } else if (e.key == 'Enter' && this.updateSong_) {
        this.hideUpdateDiv_(true);
        return true;
      } else if (e.key == 'Escape' && this.presentationLayer_.visible) {
        this.presentationLayer_.visible = false;
        this.emitPresentEvent_(false);
        return true;
      } else if (e.key == 'Escape' && this.updateSong_) {
        this.hideUpdateDiv_(false);
        return true;
      } else if (e.key == 'ArrowLeft' && !this.updateSong_) {
        this.seek_(-this.constructor.SEEK_SEC_);
        return true;
      } else if (e.key == 'ArrowRight' && !this.updateSong_) {
        this.seek_(this.constructor.SEEK_SEC_);
        return true;
      }

      return false;
    }

    handleRatingSpanKeyDown_(e) {
      if (['0', '1', '2', '3', '4', '5'].indexOf(e.key) != -1) {
        this.setRating_(numStarsToRating(parseInt(e.key)));
        e.preventDefault();
        e.stopPropagation();
      } else if (e.key == 'ArrowLeft' || e.key == 'ArrowRight') {
        const oldStars = ratingToNumStars(this.updatedRating_);
        const newStars = oldStars + (e.key == 'ArrowLeft' ? -1 : 1);
        this.setRating_(numStarsToRating(newStars));
        e.preventDefault();
        e.stopPropagation();
      }
    }

    emitCoverEvent_(url) {
      this.dispatchEvent(new CustomEvent('cover', { detail: { url } }));
    }

    emitPresentEvent_(visible) {
      this.dispatchEvent(new CustomEvent('present', { detail: { visible } }));
    }
  }
);

function numStarsToRating(numStars) {
  return numStars <= 0 ? -1.0 : (Math.min(numStars, 5) - 1) / 4.0;
}

function ratingToNumStars(rating) {
  return rating < 0.0 ? 0 : 1 + Math.round(Math.min(rating, 1.0) * 4.0);
}

function getRatingString(rating, withLabel, includeEmpty) {
  if (rating < 0.0) return 'Unrated';

  let ratingString = withLabel ? 'Rating: ' : '';
  const numStars = ratingToNumStars(rating);
  for (let i = 1; i <= 5; ++i) {
    if (i <= numStars) ratingString += '★';
    else if (includeEmpty) ratingString += '☆';
    else break;
  }
  return ratingString;
}

function getDumpSongUrl(songId) {
  return `/dump_song?songId=${songId}`;
}
