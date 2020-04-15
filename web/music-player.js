// Copyright 2010 Daniel Erat.
// All rights reserved.

import {
  $,
  createShadow,
  createTemplate,
  formatTime,
  getCurrentTimeSec,
  updateTitleAttributeForTruncation,
} from './common.js';
import Config from './config.js';
import OptionsDialog from './options-dialog.js';
import Updater from './updater.js';

const template = createTemplate(`
<style>
  @import 'common.css';
  :host {
    display: block;
  }
  #song-info {
    display: flex;
    margin: 8px;
  }

  #cover-div {
    align-items: center;
    display: flex;
    justify-content: center;
    margin-right: 6px;
  }
  #cover-div.empty {
    background-color: #f5f5f5;
    outline: solid 1px #ddd;
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
    visibility: hidden;
  }
  #rating-overlay {
    color: white;
    font-family: var(--icon-font-family);
    font-size: 12px;
    left: 11px;
    letter-spacing: 2px;
    pointer-events: none;
    position: absolute;
    text-shadow: 0 0 8px black;
    top: 63px;
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
    margin: 8px;
    user-select: none;
  }
  #controls button {
    font-family: var(--icon-font-family);
    font-size: 10px;
    width: 44px;
  }
  #controls > *:not(:first-child) {
    margin-left: 4px;
  }
  #update-container {
    background-color: white;
    border-radius: 4px;
    box-shadow: 0 1px 4px 1px rgba(0, 0, 0, 0.3);
    display: none;
    left: 12px;
    padding: 8px;
    position: absolute;
    top: 12px;
    z-index: 1;
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
    color: #666;
    cursor: pointer;
    display: inline-block;
    min-width: 17px; /* black and white stars have different sizes :-/ */
  }
  #rating a.star:hover {
    color: #888;
  }
  a.debug-link {
    color: #aaa;
    font-family: Arial, Helvetica, sans-serif;
    font-size: 10px;
    text-decoration: none;
  }
  #dump-song-link {
    margin-left: 40px;
  }
  #edit-tags {
    border: solid 1px #ddd;
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
  #playlist {
    margin-top: 2px;
  }
</style>

<presentation-layer></presentation-layer>

<audio type="audio/mpeg">
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
    <a id="dump-song-cache" class="debug-link" target="_blank">[c]</a>
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
customElements.define(
  'music-player',
  class extends HTMLElement {
    SEEK_SECONDS = 10; // seconds skipped by seeking forward or back
    MAX_RETRIES = 2; // number of consecutive playback errors to reply
    NOTIFICATION_SECONDS = 3; // duration for song-change notification

    constructor() {
      super();

      this.config_ = new Config();
      this.config_.addCallback((name, value) => {
        if (name == this.config_.VOLUME) this.audio_.volume = value;
      });

      this.updater_ = new Updater();
      this.optionsDialog_ = null;
      this.favicon_ = getFavicon();

      this.songs_ = []; // songs in the order in which they should be played
      this.tags_ = []; // available tags loaded from server
      this.currentIndex_ = -1; // index into |songs| of current track
      this.lastTimeUpdatePosition_ = 0; // playback position at last onTimeUpdate_()
      this.lastTimeUpdateSong_ = null; // current song at last onTimeUpdate_()
      this.numErrors_ = 0; // consecutive playback errors
      this.startTime_ = -1; // seconds since epoch when current track started
      this.totalPlayedSec_ = 0; // total seconds playing current song
      this.lastUpdateTime_ = -1; // seconds since epoch for last onTimeUpdate_()
      this.reportedCurrentTrack_ = false; // already reported current as played?
      this.reachedEndOfSongs_ = false; // did we hit end of last song?
      this.updateSong_ = null; // song playing when update div was opened
      this.updatedRating_ = -1.0; // rating set in update div
      this.notification_ = null; // song notification currently shown
      this.closeNotificationTimeoutId_ = 0; // for closeNotification_()

      this.shadow_ = createShadow(this, template);
      const get = id => $(id, this.shadow_);

      this.presentationLayer_ = this.shadow_.querySelector(
        'presentation-layer',
      );
      this.presentationLayer_.addEventListener('next', () => {
        this.cycleTrack_(1);
      });

      this.audio_ = this.shadow_.querySelector('audio');
      this.audio_.addEventListener('ended', () => this.onEnded_());
      this.audio_.addEventListener('pause', () => this.onPause_());
      this.audio_.addEventListener('play', () => this.onPlay_());
      this.audio_.addEventListener('timeupdate', () => this.onTimeUpdate_());
      this.audio_.addEventListener('error', e => this.onError_(e));
      this.audio_.volume = this.config_.get(this.config_.VOLUME);

      this.coverDiv_ = get('cover-div');
      this.coverImage_ = get('cover-img');
      this.coverImage_.addEventListener('click', () => this.showUpdateDiv_());
      this.coverImage_.addEventListener('load', () =>
        this.updateMediaSessionMetadata_(true /* imageLoaded */),
      );

      this.ratingOverlayDiv_ = get('rating-overlay');
      this.artistDiv_ = get('artist');
      this.titleDiv_ = get('title');
      this.albumDiv_ = get('album');
      this.timeDiv_ = get('time');

      this.prevButton_ = get('prev');
      this.prevButton_.addEventListener('click', () => this.cycleTrack_(-1));
      this.nextButton_ = get('next');
      this.nextButton_.addEventListener('click', () => this.cycleTrack_(1));
      this.playPauseButton_ = get('play-pause');
      this.playPauseButton_.addEventListener('click', () =>
        this.togglePause_(),
      );

      this.updateDiv_ = get('update-container');
      get('update-close').addEventListener('click', () =>
        this.hideUpdateDiv_(true),
      );
      this.ratingSpan_ = get('rating');
      this.ratingSpan_.addEventListener('keydown', e =>
        this.handleRatingSpanKeyDown_(e),
      );
      this.dumpSongLink_ = get('dump-song');
      this.dumpSongCacheLink_ = get('dump-song-cache');
      this.tagsTextarea_ = get('edit-tags');
      this.tagSuggester_ = get('edit-tags-suggester');

      this.playlistTable_ = get('playlist');
      this.playlistTable_.addEventListener('field', e => {
        this.dispatchEvent(new CustomEvent('field', {detail: e.detail}));
      });

      if ('mediaSession' in navigator) {
        const ms = navigator.mediaSession;
        ms.setActionHandler('play', () => this.play_());
        ms.setActionHandler('pause', () => this.pause_());
        ms.setActionHandler('seekbackward', () =>
          this.seek_(-this.SEEK_SECONDS),
        );
        ms.setActionHandler('seekforward', () => this.seek_(this.SEEK_SECONDS));
        ms.setActionHandler('previoustrack', () => this.cycleTrack_(-1));
        ms.setActionHandler('nexttrack', () => this.cycleTrack_(1));
      }

      document.body.addEventListener('keydown', e => {
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

    set dialogManager(manager) {
      this.dialogManager_ = manager;
    }

    updateTagsFromServer_(sync) {
      const req = new XMLHttpRequest();
      req.open('GET', 'list_tags', !sync);
      req.setRequestHeader('Content-type', 'application/x-www-form-urlencoded');
      req.onreadystatechange = () => {
        if (req.readyState == 4) {
          if (req.status == 200) {
            const tags = JSON.parse(req.responseText);
            this.updateTags_(tags);
            console.log('Loaded ' + tags.length + ' tag(s)');
          } else {
            console.log('Got ' + req.status + ' while loading tags');
          }
        }
      };
      req.send(null);
    }

    get currentSong_() {
      return this.currentIndex_ >= 0 && this.currentIndex_ < this.songs_.length
        ? this.songs_[this.currentIndex_]
        : null;
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
      if (clearFirst) {
        this.audio_.pause();
        this.audio_.src = '';
        this.playlistTable_.highlightRow(this.currentIndex_, false);
        this.songs_ = [];
        this.selectTrack_(0);
      }

      let index = afterCurrent
        ? Math.min(this.currentIndex_ + 1, this.songs_.length)
        : this.songs_.length;
      songs.forEach(s => this.songs_.splice(index++, 0, s));

      this.playlistTable_.setSongs(this.songs_);

      if (this.currentIndex_ == -1) this.selectTrack_(0);
      else if (this.reachedEndOfSongs_) this.cycleTrack_(1);
      else this.updateButtonState_();
    }

    // Plays the song at |offset| in the playlist relative to the current song.
    cycleTrack_(offset) {
      this.selectTrack_(this.currentIndex_ + offset);
    }

    // Plays the song at |index| in the playlist.
    selectTrack_(index) {
      if (!this.songs_.length) {
        this.currentIndex_ = -1;
        this.updateSongDisplay_();
        this.updateButtonState_();
        this.reachedEndOfSongs_ = false;
        return;
      }

      if (index < 0) index = 0;
      else if (index >= this.songs_.length) index = this.songs_.length - 1;

      if (index == this.currentIndex_) return;

      this.playlistTable_.highlightRow(this.currentIndex_, false);
      this.playlistTable_.highlightRow(index, true);
      this.currentIndex_ = index;

      this.updateSongDisplay_();
      this.updatePresentationLayerSongs_();
      this.play_();
      this.updateButtonState_();
      if (!document.hasFocus()) this.showNotification_();
    }

    updateTags_(tags) {
      this.tags_ = tags;
      this.tagSuggester_.words = tags;
      this.dispatchEvent(new CustomEvent('tags', {detail: {tags}}));
    }

    updateButtonState_() {
      this.prevButton_.disabled = this.currentIndex_ <= 0;
      this.nextButton_.disabled =
        this.currentIndex_ < 0 || this.currentIndex_ >= this.songs_.length - 1;
      this.playPauseButton_.disabled = this.currentIndex_ < 0;
    }

    updateSongDisplay_() {
      const song = this.currentSong_;
      document.title = song ? song.artist + ' - ' + song.title : 'Player';

      this.artistDiv_.innerText = song ? song.artist : '';
      this.titleDiv_.innerText = song ? song.title : '';
      this.albumDiv_.innerText = song ? song.album : '';
      this.timeDiv_.innerText = '';

      updateTitleAttributeForTruncation(
        this.artistDiv_,
        song ? song.artist : '',
      );
      updateTitleAttributeForTruncation(this.titleDiv_, song ? song.title : '');
      updateTitleAttributeForTruncation(this.albumDiv_, song ? song.album : '');

      if (song && song.coverUrl) {
        this.coverDiv_.classList.remove('empty');
        this.coverImage_.src = song.coverUrl;
        if (this.favicon_) this.favicon_.href = song.coverUrl;
      } else {
        this.coverDiv_.classList.add('empty');
        if (this.favicon_) this.favicon_.href = 'images/missing_cover_icon.png';
      }

      this.updateCoverTitleAttribute_();
      this.updateRatingOverlay_();
      // Metadata will be updated again after |coverImage| is loaded.
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
      let nextSong = null;
      if (
        this.currentIndex_ >= 0 &&
        this.currentIndex_ + 1 < this.songs_.length
      ) {
        nextSong = this.songs_[this.currentIndex_ + 1];
      }

      this.presentationLayer_.updateSongs(this.currentSong_, nextSong);
    }

    showNotification_() {
      if (!('Notification' in window)) return;

      if (Notification.permission !== 'granted') {
        if (Notification.permission !== 'denied') {
          Notification.requestPermission();
        }
        return;
      }

      if (this.notification_) {
        window.clearTimeout(this.closeNotificationTimeoutId_);
        this.closeNotification_();
      }

      const song = this.currentSong_;
      if (!song) return;

      this.notification_ = new Notification(song.artist + '\n' + song.title, {
        body: song.album + '\n' + formatTime(song.length),
        icon: song.coverUrl,
      });
      this.closeNotificationTimeoutId_ = window.setTimeout(
        () => this.closeNotification_(),
        this.NOTIFICATION_SECONDS * 1000,
      );
    }

    closeNotification_() {
      if (!this.notification_) return;

      this.notification_.close();
      this.notification_ = null;
      this.closeNotificationTimeoutId_ = 0;
    }

    // Starts playback. If |currentSong_| isn't being played, switches to it
    // even if we were already playing. Also restarts playback if we were
    // stopped at the end of the last song in the playlist.
    play_() {
      const song = this.currentSong_;
      if (this.audio_.src != song.url || this.reachedEndOfSongs_) {
        console.log('Starting ' + song.songId + ' (' + song.url + ')');
        this.audio_.src = song.url;
        this.audio_.currentTime = 0;
        this.lastTimeUpdatePosition_ = 0;
        this.lastTimeUpdateSong_ = null;
        this.numErrors_ = 0;
        this.startTime_ = getCurrentTimeSec();
        this.totalPlayedSec_ = 0;
        this.lastUpdateTime_ = -1;
        this.reportedCurrentTrack_ = false;
        this.reachedEndOfSongs_ = false;
      }

      console.log('Playing');
      this.audio_.play().catch(e => {
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
      this.audio_.paused ? this.play_() : this.pause_();
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
        this.cycleTrack_(1);
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
      const position = this.audio_.currentTime;

      // Avoid resetting |numErrors_| if we get called repeatedly without making
      // any progress.
      if (
        song == this.lastTimeUpdateSong_ &&
        position == this.lastTimeUpdatePosition_
      ) {
        return;
      }

      this.lastTimeUpdatePosition_ = position;
      this.lastTimeUpdateSong_ = song;
      this.numErrors_ = 0;

      const duration = song ? song.length : this.audio_.duration;
      if (duration) {
        const cur = formatTime(position);
        const dur = formatTime(duration);
        this.timeDiv_.innerText = `[ ${cur} / ${dur} ]`;
      } else {
        this.timeDiv_.innerText = '';
      }

      const now = getCurrentTimeSec();
      if (this.lastUpdateTime_ > 0) {
        this.totalPlayedSec_ += now - this.lastUpdateTime_;
      }
      this.lastUpdateTime_ = now;

      if (!this.reportedCurrentTrack_) {
        if (
          this.totalPlayedSec_ >= 240 ||
          this.totalPlayedSec_ > duration / 2
        ) {
          this.reportedCurrentTrack_ = true;
          this.updater_.reportPlay(song.songId, this.startTime_);
        }
      }

      this.presentationLayer_.updatePosition(position);
    }

    onError_(e) {
      this.numErrors_++;

      const error = e.target.error;
      console.log('Got playback error: ' + error.code);
      switch (error.code) {
        case error.MEDIA_ERR_ABORTED: // 1
          break;
        case error.MEDIA_ERR_NETWORK: // 2
        case error.MEDIA_ERR_DECODE: // 3
        case error.MEDIA_ERR_SRC_NOT_SUPPORTED: // 4
          if (this.numErrors_ <= this.MAX_RETRIES) {
            console.log(
              'Retrying from position ' + this.lastTimeUpdatePosition_,
            );
            this.audio_.load();
            this.audio_.currentTime = this.lastTimeUpdatePosition_;
            this.audio_.play();
          } else {
            console.log('Giving up after ' + this.numErrors_ + ' error(s)');
            this.cycleTrack_(1);
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
      this.dumpSongLink_.href = getDumpSongUrl(song, false);
      this.dumpSongCacheLink_.href = getDumpSongUrl(song, true);
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
        if (
          !this.tags_.length ||
          this.tags_.indexOf(tag) != -1 ||
          song.tags.indexOf(tag) != -1
        ) {
          newTags.push(tag);
        } else if (tag[0] == '+' && tag.length > 1) {
          tag = tag.substring(1);
          newTags.push(tag);
          if (this.tags_.indexOf(tag) == -1) createdTags.push(tag);
        } else {
          console.log('Skipping unknown tag "' + tag + '"');
        }
      }
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
        tagsChanged ? newTags : null,
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
            false,
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
      if (!this.dialogManager_) throw new Error('No <dialog-manager>');

      this.optionsDialog_ = new OptionsDialog(
        this.config_,
        this.dialogManager_,
        () => {
          this.optionsDialog_ = null;
        },
      );
    }

    processAccelerator_(e) {
      if (this.dialogManager_ && this.dialogManager_.numDialogs) return false;

      if (e.altKey && e.key == 'd') {
        const song = this.currentSong_;
        if (song) window.open(getDumpSongUrl(song, false), '_blank');
        return true;
      } else if (e.altKey && e.key == 'n') {
        this.cycleTrack_(1);
        return true;
      } else if (e.altKey && e.key == 'o') {
        this.showOptions_();
        return true;
      } else if (e.altKey && e.key == 'p') {
        this.cycleTrack_(-1);
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
        this.seek_(-this.SEEK_SECONDS);
        return true;
      } else if (e.key == 'ArrowRight' && !this.updateSong_) {
        this.seek_(this.SEEK_SECONDS);
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

    emitPresentEvent_(visible) {
      this.dispatchEvent(new CustomEvent('present', {detail: {visible}}));
    }
  },
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

function getDumpSongUrl(song, cache) {
  return 'dump_song?id=' + song.songId + (cache ? '&cache=1' : '');
}

// Returns the first <link> element containing a 'rel' attribute of 'icon', or
// null if not found.
function getFavicon() {
  const links = document.getElementsByTagName('link');
  for (let i = 0; i < links.length; i++) {
    if (links[i].getAttribute('rel') == 'icon') return links[i];
  }
  return null;
}
