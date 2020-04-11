// Copyright 2010 Daniel Erat.
// All rights reserved.

import {
  $,
  createShadow,
  createTemplate,
  formatTime,
  getCurrentTimeSec,
  KeyCodes,
  updateTitleAttributeForTruncation,
} from './common.js';
import OptionsDialog from './options-dialog.js';
import Updater from './updater.js';

const template = createTemplate(`
<style>
  #song-info {
    padding: 5px;
  }
  #cover-div {
    float: left;
    width: 65px;
    height: 65px;
    padding-left: 2px;
    padding-right: 5px;
    display: flex;
    justify-content: center;
    align-items: center;
  }
  #cover-img {
    max-width: 65px;
    max-height: 65px;
    height: auto;
    cursor: pointer;
    -webkit-user-select: none;
  }
  #rating-overlay {
    position: absolute;
    left: 8px;
    top: 52px;
    color: white;
    text-shadow: 0 0 8px black;
    font-family: Arial, Helvetica, sans-serif;
    font-size: 13px;
    -webkit-user-select: none;
    pointer-events: none;
  }
  #artist {
    font-weight: bold;
    line-height: 1.3;
    white-space: nowrap;
    overflow: hidden;
  }
  #title {
    font-style: italic;
    line-height: 1.3;
    white-space: nowrap;
    overflow: hidden;
  }
  #album {
    white-space: nowrap;
    line-height: 1.3;
    overflow: hidden;
  }
  #controls {
    padding: 5px;
    clear: both;
    -webkit-user-select: none;
  }
  #update-container {
    position: absolute;
    left: 12px;
    top: 12px;
    width: 225px;
    height: 78px;
    background-color: white;
    -moz-box-shadow: 0 1px 4px 1px rgba(0, 0, 0, 0.3);
    -webkit-box-shadow: 0 1px 4px 1px rgba(0, 0, 0, 0.3);
    box-shadow: 0 1px 4px 1px rgba(0, 0, 0, 0.3);
    z-index: 1;
    display: none;
  }
  #update-close-img {
    position: absolute;
    right: 5px;
    top: 5px;
    cursor: pointer;
  }
  #rating-container {
    margin-left: 6px;
    margin-top: 8px;
  }
  #rating a.star {
    cursor: pointer;
    font-size: 16px;
    line-height: 12px;
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
    border: solid 1px #ccc;
    width: 210px;
    height: 35px;
    margin-left: 4px;
    margin-top: 10px;
    resize: none;
    font-family: Arial, Helvetica, sans-serif;
  }
  #edit-tags-suggester {
    position: absolute;
    left: 4px;
    bottom: 52px;
    max-width: 210px;
    max-height: 26px;
  }
</style>

<audio type="audio/mpeg">
  Your browser doesn't support the audio element.
</audio>

<div id="song-info">
  <div id="cover-div">
    <img id="cover-img" alt="" />
  </div>
  <div id="rating-overlay"></div>
  <div id="artist"></div>
  <div id="title"></div>
  <div id="album"></div>
  <div id="time"></div>
</div>

<div id="controls">
  <button id="prev" disabled>Prev</button>
  <button id="next" disabled>Next</button>
  <button id="play-pause" disabled>Pause</button>
</div>

<div id="update-container">
  <img id="update-close" src="images/update_close.png" />
  <div id="rating-container">
    Rating: <span id="rating" tabindex="0"></span>
    <a id="dump-song" class="debug-link" target="_blank">[d]</a>
    <a id="dump-song-cache" class="debug-link" target="_blank">[c]</a>
  </div>
  <textarea id="edit-tags"></textarea>
  <tag-suggester id="edit-tags-suggester" />
</div>
`);

customElements.define(
  'music-player',
  class extends HTMLElement {
    SEEK_SECONDS = 10; // seconds skipped by seeking forward or back
    MAX_RETRIES = 2; // number of consecutive playback errors to reply
    NOTIFICATION_SECONDS = 3; // duration for song-change notification

    constructor() {
      super();

      this.config_ = null;
      this.playlist_ = null;
      this.dialogManager_ = null;
      this.presentationLayer_ = null;
      this.favicon_ = null;
      this.optionsDialog_ = null;
      this.updater_ = new Updater();

      this.songs_ = []; // songs in the order in which they should be played
      this.tags_ = []; // available tags loaded from server
      this.currentIndex_ = -1; // index into |songs| of current track
      this.lastPositionSec_ = 0; // playback position at last onTimeUpdate_()
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

      this.style.display = 'block';
      this.shadow_ = createShadow(this, template);
      const get = id => $(id, this.shadow_);

      this.audio_ = this.shadow_.querySelector('audio');

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
      this.tagsTextarea = get('edit-tags');

      this.tagSuggester = get('edit-tags-suggester');
      this.tagSuggester.target = this.tagsTextarea;

      this.audio_.addEventListener('ended', () => this.onEnded_());
      this.audio_.addEventListener('pause', () => this.onPause_());
      this.audio_.addEventListener('play', () => this.onPlay_());
      this.audio_.addEventListener('timeupdate', () => this.onTimeUpdate_());
      this.audio_.addEventListener('error', e => this.onError_(e));

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

      this.updateTagsFromServer_(true /* async */);
    }

    set config(config) {
      this.config_ = config;
      config.addListener(this);
      this.onVolumeChange(config.getVolume());
    }

    set dialogManager(manager) {
      this.dialogManager_ = manager;
    }

    set presentationLayer(layer) {
      this.presentationLayer_ = layer;
      layer.setPlayNextTrackFunction(() => this.cycleTrack_(1));
    }

    set playlist(playlist) {
      this.playlist_ = playlist;
      if (playlist) {
        this.notifyPlaylistAboutSongChange_();
        this.playlist_.handleTagsUpdated(this.tags_); // TODO: get rid of this
      }
    }

    updateTagsFromServer_(async) {
      const req = new XMLHttpRequest();
      req.open('GET', 'list_tags', async);
      req.setRequestHeader('Content-type', 'application/x-www-form-urlencoded');
      req.onreadystatechange = () => {
        if (req.readyState == 4) {
          if (req.status == 200) {
            this.updateTags_(JSON.parse(req.responseText));
            console.log('Loaded ' + this.tags_.length + ' tags');
          } else {
            console.log('Got ' + req.status + ' while loading tags');
          }
        }
      };
      req.send(null);
    }

    get currentSong() {
      return this.currentIndex_ >= 0 && this.currentIndex_ < this.songs_.length
        ? this.songs_[this.currentIndex_]
        : null;
    }

    get songs() {
      return this.songs_;
    }

    // TODO: Make this not be called by the playlist?
    setSongs(songs) {
      const oldSong = this.currentSong;

      this.songs_ = songs;

      // If we're currently playing a track that's no longer in the playlist,
      // then jump to the first song. Otherwise, just keep playing it (some
      // tracks were probably just appended to the previous playlist).
      const song = this.currentSong;
      if (!song || oldSong != song) {
        this.audio_.pause();
        this.audio_.src = '';
        this.currentIndex_ = -1;
        this.selectTrack_(0);
      } else if (this.reachedEndOfSongs_) {
        this.cycleTrack_(1);
      } else {
        this.updateButtonState_();
        this.notifyPlaylistAboutSongChange_();
      }
    }

    cycleTrack_(offset) {
      this.selectTrack_(this.currentIndex_ + offset);
    }

    selectTrack_(index) {
      if (!this.songs_.length) {
        this.currentIndex_ = -1;
        this.updateSongDisplay_();
        this.updateButtonState_();
        return;
      }

      if (index < 0) index = 0;
      else if (index >= this.songs_.length) index = this.songs_.length - 1;

      if (index == this.currentIndex_) return;

      this.currentIndex_ = index;
      this.notifyPlaylistAboutSongChange_();
      this.updateSongDisplay_();
      this.updatePresentationLayerSongs_();
      this.startCurrentTrack_();
      this.updateButtonState_();
      if (!document.hasFocus()) this.showNotification_();
    }

    updateTags_(tags) {
      this.tags_ = tags.slice(0);
      this.tagSuggester.words = this.tags_;
      if (this.playlist_) this.playlist_.handleTagsUpdated(this.tags_);
    }

    updateButtonState_() {
      this.prevButton_.disabled = this.currentIndex_ <= 0;
      this.nextButton_.disabled =
        this.currentIndex_ < 0 || this.currentIndex_ >= this.songs_.length - 1;
      this.playPauseButton_.disabled = this.currentIndex_ < 0;
    }

    updateSongDisplay_() {
      const song = this.currentSong;
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

      const setCover = url => {
        this.coverImage_.src = url;
        if (this.favicon_) {
          this.favicon.href = url;
          this.favicon.type = /\.png$/.match(url) ? 'image/png' : 'image/jpeg';
        }
      };
      setCover(
        song && song.coverUrl ? song.coverUrl : 'images/missing_cover.png',
      );

      this.updateCoverTitleAttribute_();
      this.updateRatingOverlay_();
      // Metadata will be updated again after |coverImage| is loaded.
      this.updateMediaSessionMetadata_(false /* imageLoaded */);
    }

    updateCoverTitleAttribute_() {
      const song = this.currentSong;
      if (!song) {
        this.coverImage_.title = '';
        return;
      }

      let text = getRatingString(song.rating, true, true);
      if (song.tags.length > 0) text += '\nTags: ' + song.tags.sort().join(' ');
      this.coverImage_.title = text;
    }

    updateRatingOverlay_() {
      const song = this.currentSong;
      this.ratingOverlayDiv_.innerText =
        song && song.rating >= 0.0
          ? getRatingString(song.rating, false, false)
          : '';
    }

    updateMediaSessionMetadata_(imageLoaded) {
      if (!('mediaSession' in navigator)) return;

      const song = this.currentSong;
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

      this.presentationLayer_.updateSongs(this.currentSong, nextSong);
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

      const song = this.currentSong;
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

    startCurrentTrack_() {
      this.lastPositionSec_ = 0;
      this.numErrors_ = 0;
      this.startTime_ = getCurrentTimeSec();
      this.totalPlayedSec_ = 0;
      this.lastUpdateTime_ = -1;
      this.reportedCurrentTrack_ = false;
      this.reachedEndOfSongs_ = false;

      const song = this.currentSong;
      console.log('Starting ' + song.songId + ' (' + song.url + ')');
      this.audio_.src = song.url;
      this.audio_.currentTime = 0;
      this.play_();
    }

    play_() {
      console.log('Playing');
      this.audio_.play();
    }

    pause_() {
      console.log('Pausing');
      this.audio_.pause();
    }

    togglePause_() {
      if (this.reachedEndOfSongs_) {
        this.startCurrentTrack_();
      } else if (this.audio_.paused) {
        this.play_();
      } else {
        this.pause_();
      }
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
      this.playPauseButton_.innerText = 'Play';
      this.lastUpdateTime_ = -1;
    }

    onPlay_() {
      this.playPauseButton_.innerText = 'Pause';
      this.lastUpdateTime_ = getCurrentTimeSec();
    }

    onTimeUpdate_() {
      const song = this.currentSong;
      const duration = song ? song.length : this.audio_.duration;

      if (this.audio_.duration > 0) {
        const cur = formatTime(this.audio_.currentTime);
        const dur = formatTime(duration);
        this.timeDiv_.innerText = `[${cur} / ${dur}]`;
      } else {
        this.timeDiv_.innerText = '';
      }

      this.lastPositionSec_ = this.audio_.currentTime;
      this.numErrors_ = 0;

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

      this.presentationLayer_.updatePosition(this.audio_.currentTime);
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
            console.log('Retrying from position ' + this.lastPositionSec_);
            this.audio_.load();
            this.audio_.currentTime = this.lastPositionSec_;
            this.audio_.play();
          } else {
            console.log('Giving up after ' + this.numErrors_ + ' error(s)');
            this.cycleTrack_(1);
          }
          break;
      }
    }

    // TODO: Get rid of this.
    notifyPlaylistAboutSongChange_() {
      if (this.playlist_) this.playlist_.handleSongChange(this.currentIndex_);
    }

    showUpdateDiv_() {
      const song = this.currentSong;
      if (!song) return false;

      // Already shown.
      if (this.updateSong_) return true;

      this.setRating_(song.rating);
      this.dumpSongLink_.href = getDumpSongUrl(song, false);
      this.dumpSongCacheLink_.href = getDumpSongUrl(song, true);
      this.tagsTextarea.value = song.tags.sort().join(' ');
      this.updateDiv_.style.display = 'block';
      this.updateSong_ = song;
      return true;
    }

    hideUpdateDiv_(saveChanges) {
      this.updateDiv_.style.display = 'none';
      this.ratingSpan_.blur();
      this.tagsTextarea.blur();

      const song = this.updateSong_;
      this.updateSong_ = null;

      if (!song || !saveChanges) return;

      const ratingChanged = this.updatedRating_ != song.rating;

      const newRawTags = this.tagsTextarea.value.trim().split(/\s+/);
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

      this.optionsDialog_ = new OptionsDialog(
        this.config_,
        this.dialogManager_,
        () => {
          this.optionsDialog_ = null;
        },
      );
    }

    processAccelerator_(e) {
      if (this.dialogManager_.numDialogs) return false;

      if (e.altKey && e.keyCode == KeyCodes.D) {
        const song = this.currentSong;
        if (song) window.open(getDumpSongUrl(song, false), '_blank');
        return true;
      } else if (e.altKey && e.keyCode == KeyCodes.N) {
        this.cycleTrack_(1);
        return true;
      } else if (e.altKey && e.keyCode == KeyCodes.O) {
        this.showOptions_();
        return true;
      } else if (e.altKey && e.keyCode == KeyCodes.P) {
        this.cycleTrack_(-1);
        return true;
      } else if (e.altKey && e.keyCode == KeyCodes.R) {
        if (this.showUpdateDiv_()) this.ratingSpan_.focus();
        return true;
      } else if (e.altKey && e.keyCode == KeyCodes.T) {
        if (this.showUpdateDiv_()) this.tagsTextarea.focus();
        return true;
      } else if (e.altKey && e.keyCode == KeyCodes.V) {
        if (this.presentationLayer_.isShown()) this.presentationLayer_.hide();
        else this.presentationLayer_.show();
        return true;
      } else if (e.keyCode == KeyCodes.SPACE && !this.updateSong_) {
        this.togglePause_();
        return true;
      } else if (e.keyCode == KeyCodes.ENTER && this.updateSong_) {
        this.hideUpdateDiv_(true);
        return true;
      } else if (
        e.keyCode == KeyCodes.ESCAPE &&
        this.presentationLayer_.isShown()
      ) {
        this.presentationLayer_.hide();
        return true;
      } else if (e.keyCode == KeyCodes.ESCAPE && this.updateSong_) {
        this.hideUpdateDiv_(false);
        return true;
      } else if (e.keyCode == KeyCodes.LEFT && !this.updateSong_) {
        this.seek_(-Player.SEEK_SECONDS);
        return true;
      } else if (e.keyCode == KeyCodes.RIGHT && !this.updateSong_) {
        this.seek_(Player.SEEK_SECONDS);
        return true;
      }

      return false;
    }

    handleRatingSpanKeyDown_(e) {
      if (e.keyCode >= KeyCodes.ZERO && e.keyCode <= KeyCodes.FIVE) {
        this.setRating_(numStarsToRating(e.keyCode - KeyCodes.ZERO));
        e.preventDefault();
        e.stopPropagation();
      } else if (e.keyCode == KeyCodes.LEFT || e.keyCode == KeyCodes.RIGHT) {
        const oldStars = ratingToNumStars(this.updatedRating_);
        const newStars = oldStars + (e.keyCode == KeyCodes.LEFT ? -1 : 1);
        this.setRating_(numStarsToRating(newStars));
        e.preventDefault();
        e.stopPropagation();
      }
    }

    // TODO: Make this not be called by Config.
    onVolumeChange(volume) {
      this.audio_.volume = volume;
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
    if (i <= numStars) ratingString += '\u2605';
    else if (includeEmpty) ratingString += '\u2606';
    else break;
  }
  return ratingString;
}

function getDumpSongUrl(song, cache) {
  return 'dump_song?id=' + song.songId + (cache ? '&cache=1' : '');
}