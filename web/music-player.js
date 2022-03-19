// Copyright 2010 Daniel Erat.
// All rights reserved.

import {
  $,
  createShadow,
  createTemplate,
  emptyImg,
  formatTime,
  getCurrentTimeSec,
  getDumpSongUrl,
  getRatingString,
  getScaledCoverUrl,
  getSongUrl,
  handleFetchError,
  moveItem,
  numStarsToRating,
  ratingToNumStars,
  updateTitleAttributeForTruncation,
} from './common.js';
import Config from './config.js';
import OptionsDialog from './options-dialog.js';
import { showSongInfo } from './song-info.js';
import { showStats } from './stats.js';
import Updater from './updater.js';

const template = createTemplate(`
<style>
  @import 'common.css';
  :host {
    display: flex;
    flex-direction: column;
    position: relative; /* needed for menu-button */
  }

  #menu-button {
    cursor: pointer;
    font-size: 22px;
    padding: 0 var(--margin);
    position: absolute;
    right: 0;
    top: 0;
    user-select: none;
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

<div id="menu-button">⋯</div>

<audio-wrapper></audio-wrapper>

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
  <button id="prev" disabled title="Previous song (Alt+P)">⏮</button>
  <button id="play-pause" disabled title="Pause (Space)">⏸</button>
  <button id="next" disabled title="Next song (Alt+N)">⏭</button>
</div>

<div id="update-container">
  <span id="update-close" class="x-icon" title="Close"></span>
  <div id="rating-container">
    Rating: <span id="rating" tabindex="0"></span>
  </div>
  <tag-suggester id="edit-tags-suggester">
    <textarea id="edit-tags" slot="text" placeholder="Tags"></textarea>
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
    static PRELOAD_SEC_ = 20; // seconds from end of song when next song should be loaded

    constructor() {
      super();

      this.updater_ = new Updater();
      this.optionsDialog_ = null;

      this.songs_ = []; // songs in the order in which they should be played
      this.tags_ = []; // available tags loaded from server
      this.currentIndex_ = -1; // index into |songs| of current track
      this.startTime_ = null; // seconds since epoch when current track started
      this.reportedCurrentTrack_ = false; // already reported current as played?
      this.reachedEndOfSongs_ = false; // did we hit end of last song?
      this.updateSong_ = null; // song playing when update div was opened
      this.updatedRating_ = -1.0; // rating set in update div
      this.notification_ = null; // song notification currently shown
      this.closeNotificationTimeoutId_ = null; // for closeNotification_()
      this.playDelayMs_ = this.constructor.PLAY_DELAY_MS_;
      this.playTimeoutId_ = 0; // for playInternal_()
      this.shuffled_ = false; // playlist contains shuffled songs

      this.shadow_ = createShadow(this, template);
      const get = (id) => $(id, this.shadow_);

      this.presentationLayer_ =
        this.shadow_.querySelector('presentation-layer');
      this.presentationLayer_.addEventListener('next', () => {
        this.cycleTrack_(1, false /* delayPlay */);
      });
      this.presentationLayer_.addEventListener('hide', () => {
        this.setPresentationLayerVisible_(false);
      });

      this.menuButton_ = get('menu-button');
      this.menuButton_.addEventListener('click', (e) => {
        const rect = this.menuButton_.getBoundingClientRect();
        const menu = this.overlayManager_.createMenu(
          rect.right + 12, // compensate for right padding
          rect.bottom,
          [
            {
              id: 'present',
              text: 'Presentation',
              cb: () => this.setPresentationLayerVisible_(true),
              hotkey: 'Alt+V',
            },
            {
              id: 'options',
              text: 'Options…',
              cb: () => this.showOptions_(),
              hotkey: 'Alt+O',
            },
            {
              id: 'stats',
              text: 'Stats…',
              cb: () => this.showStats_(),
            },
            {
              id: 'info',
              text: 'Song info…',
              cb: () => {
                const song = this.currentSong_;
                if (song) showSongInfo(this.overlayManager_, song);
              },
              hotkey: 'Alt+I',
            },
            {
              id: 'debug',
              text: 'Debug…',
              cb: () => {
                const song = this.currentSong_;
                if (song) window.open(getDumpSongUrl(song.songId), '_blank');
              },
              hotkey: 'Alt+D',
            },
          ],
          true /* alignRight */
        );
      });

      this.audio_ = this.shadow_.querySelector('audio-wrapper');
      this.audio_.addEventListener('ended', () => this.onEnded_());
      this.audio_.addEventListener('pause', () => this.onPause_());
      this.audio_.addEventListener('play', () => this.onPlay_());
      this.audio_.addEventListener('timeupdate', () => this.onTimeUpdate_());
      this.audio_.addEventListener('error', (e) => this.onError_(e));

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
      this.tagsTextarea_ = get('edit-tags');
      this.tagSuggester_ = get('edit-tags-suggester');

      this.playlistTable_ = get('playlist');
      this.playlistTable_.addEventListener('field', (e) => {
        this.dispatchEvent(new CustomEvent('field', { detail: e.detail }));
      });
      this.playlistTable_.addEventListener('reorder', (e) => {
        this.currentIndex_ = moveItem(
          this.songs_,
          e.detail.fromIndex,
          e.detail.toIndex,
          this.currentIndex_
        );
        this.updatePresentationLayerSongs_();
        this.updateButtonState_();
        // TODO: Preload the next song if needed.
      });
      this.playlistTable_.addEventListener('menu', (e) => {
        if (!this.overlayManager_) throw new Error('No overlay manager');

        const idx = e.detail.index;
        const orig = e.detail.orig;
        orig.preventDefault();

        const menu = this.overlayManager_.createMenu(orig.pageX, orig.pageY, [
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
          { text: '-' },
          {
            id: 'info',
            text: 'Info…',
            cb: () => showSongInfo(this.overlayManager_, this.songs_[idx]),
          },
          {
            id: 'debug',
            text: 'Debug…',
            cb: () => window.open(getDumpSongUrl(e.detail.songId), '_blank'),
          },
        ]);

        // Highlight the row while the menu is open.
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

    set config(config) {
      this.config_ = config;
      this.config_.addCallback((name, value) => {
        if (name === Config.GAIN_TYPE || name === Config.PRE_AMP) {
          this.updateGain_();
        }
      });
    }

    set overlayManager(manager) {
      this.overlayManager_ = manager;
    }

    // Returns true if the update div is currently shown.
    get updateDivShown() {
      return !!this.updateSong_;
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
    get nextSong_() {
      return this.songs_[this.currentIndex_ + 1] || null;
    }

    resetForTesting() {
      this.hideUpdateDiv_(false /* saveChanges */);
      if (this.songs_.length) this.removeSongs_(0, this.songs_.length);
    }

    // Adds |songs| to the playlist.
    // If |clearFirst| is true, the existing playlist is cleared first.
    // If |afterCurrent| is true, |songs| are inserted immediately after the
    // current song. Otherwise, they are appended to the end of the playlist.
    // |shuffled| is used for the 'auto' gain adjustment setting.
    enqueueSongs(songs, clearFirst, afterCurrent, shuffled) {
      if (clearFirst) this.removeSongs_(0, this.songs_.length);

      let index = afterCurrent
        ? Math.min(this.currentIndex_ + 1, this.songs_.length)
        : this.songs_.length;
      songs.forEach((s) => this.songs_.splice(index++, 0, s));

      if (shuffled && songs.length) this.shuffled_ = true;

      this.playlistTable_.setSongs(this.songs_);

      if (this.currentIndex_ === -1) {
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

      if (!this.songs_.length) this.shuffled_ = false;

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
      this.audio_.src = null;
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

      if (index === this.currentIndex_) return;

      this.playlistTable_.setRowActive(this.currentIndex_, false);
      this.playlistTable_.setRowActive(index, true);
      this.playlistTable_.scrollToRow(index);
      this.currentIndex_ = index;
      this.audio_.src = null;

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
        this.dispatchEvent(new CustomEvent('cover', { detail: { url } }));
      } else {
        this.coverImage_.src = emptyImg;
        this.coverDiv_.classList.add('empty');
        this.dispatchEvent(new CustomEvent('cover', { detail: { url: null } }));
      }

      // Cache the scaled cover images for the next song and the one after it.
      // This prevents ugly laggy updates here and in <presentation-layer>.
      // Note that this will probably only work for non-admin users due to an
      // App Engine "feature": https://github.com/derat/nup/issues/1
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

      this.coverImage_.title =
        getRatingString(song.rating, '★', '☆', 'Unrated', 'Rating: ') +
        '\n' +
        (song.tags.length
          ? 'Tags: ' + song.tags.sort().join(' ')
          : '(Alt+R or Alt+T to edit)');
    }

    updateRatingOverlay_() {
      const song = this.currentSong_;
      this.ratingOverlayDiv_.innerText = song
        ? getRatingString(song.rating, '★', '', '', '')
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
      if (!this.currentSong_) return;

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
      if (!song) return;

      // Get an absolute URL since that's what we'll get from the <audio>
      // element: https://stackoverflow.com/a/44547904
      const url = getSongUrl(song.filename);
      if (this.audio_.src != url || this.reachedEndOfSongs_) {
        console.log(`Starting ${song.songId} (${url})`);
        this.audio_.src = url;
        this.audio_.currentTime = 0;

        this.startTime_ = getCurrentTimeSec();
        this.reportedCurrentTrack_ = false;
        this.reachedEndOfSongs_ = false;
        this.pausedForOfflineTime_ = -1;
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
      this.playPauseButton_.title = 'Play (Space)';
    }

    onPlay_() {
      this.playPauseButton_.innerText = '⏸';
      this.playPauseButton_.title = 'Pause (Space)';
    }

    onTimeUpdate_() {
      const song = this.currentSong_;
      if (song === null) return;

      const pos = this.audio_.currentTime;
      const played = this.audio_.playtime;
      const dur = song.length;

      if (!this.reportedCurrentTrack_ && (played >= 240 || played > dur / 2)) {
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
        this.nextSong_ &&
        !this.audio_.preloadSrc
      ) {
        const url = getSongUrl(this.nextSong_.filename);
        console.log(`Preloading ${this.nextSong_.songId} (${url})`);
        this.audio_.preloadSrc = url;
      }
    }

    onError_(e) {
      this.cycleTrack_(1, false /* delay */);
    }

    showUpdateDiv_() {
      const song = this.currentSong_;
      if (!song) return false;

      // Already shown.
      if (this.updateSong_) return true;

      this.setRating_(song.rating);
      this.tagsTextarea_.value = song.tags.length
        ? song.tags.sort().join(' ') + ' ' // append space to ease editing
        : '';
      this.updateSong_ = song;
      this.updateDiv_.classList.add('shown');
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
          i <= numStars ? '★' : '☆';
      }
    }

    showOptions_() {
      if (this.optionsDialog_) return;
      if (!this.config_) throw new Error('No config');
      if (!this.overlayManager_) throw new Error('No overlay manager');

      this.optionsDialog_ = new OptionsDialog(
        this.config_,
        this.overlayManager_,
        () => {
          this.optionsDialog_ = null;
        }
      );
    }

    showStats_() {
      if (!this.overlayManager_) throw new Error('No overlay manager');
      showStats(this.overlayManager_);
    }

    // Shows or hides the presentation layer.
    setPresentationLayerVisible_(visible) {
      if (this.presentationLayer_.visible == visible) return;
      this.presentationLayer_.visible = visible;
      this.dispatchEvent(new CustomEvent('present', { detail: { visible } }));
    }

    // Adjusts |audio|'s gain appropriately for the current song and settings.
    // This implements the approach described at
    // https://wiki.hydrogenaud.io/index.php?title=ReplayGain_specification.
    updateGain_() {
      let adj = this.config_.get(Config.PRE_AMP); // decibels

      const song = this.currentSong_;
      if (song) {
        let gainType = this.config_.get(Config.GAIN_TYPE);
        if (gainType === Config.GAIN_AUTO) {
          gainType = this.shuffled_ ? Config.GAIN_TRACK : Config.GAIN_ALBUM;
        }

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

      console.log(`Scaling amplitude by ${scale.toFixed(3)}`);
      this.audio_.gain = scale;
    }

    processAccelerator_(e) {
      if (this.overlayManager_ && this.overlayManager_.numChildren) {
        return false;
      }

      if (e.altKey && e.key == 'd') {
        const song = this.currentSong_;
        if (song) window.open(getDumpSongUrl(song.songId), '_blank');
        this.setPresentationLayerVisible_(false);
        return true;
      } else if (e.altKey && e.key == 'i') {
        const song = this.currentSong_;
        if (song) showSongInfo(this.overlayManager_, song);
        this.setPresentationLayerVisible_(false);
        return true;
      } else if (e.altKey && e.key == 'n') {
        this.cycleTrack_(1, true /* delay */);
        return true;
      } else if (e.altKey && e.key == 'o') {
        this.showOptions_();
        this.setPresentationLayerVisible_(false);
        return true;
      } else if (e.altKey && e.key == 'p') {
        this.cycleTrack_(-1, true /* delay */);
        return true;
      } else if (e.altKey && e.key == 'r') {
        if (this.showUpdateDiv_()) this.ratingSpan_.focus();
        this.setPresentationLayerVisible_(false);
        return true;
      } else if (e.altKey && e.key == 't') {
        if (this.showUpdateDiv_()) this.tagsTextarea_.focus();
        this.setPresentationLayerVisible_(false);
        return true;
      } else if (e.altKey && e.key == 'v') {
        this.setPresentationLayerVisible_(!this.presentationLayer_.visible);
        if (this.updateSong_) this.hideUpdateDiv_(false);
        return true;
      } else if (e.key == ' ' && !this.updateSong_) {
        this.togglePause_();
        return true;
      } else if (e.key == 'Enter' && this.updateSong_) {
        this.hideUpdateDiv_(true);
        return true;
      } else if (e.key == 'Escape' && this.presentationLayer_.visible) {
        this.setPresentationLayerVisible_(false);
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
  }
);
