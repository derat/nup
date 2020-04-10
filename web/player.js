// Copyright 2010 Daniel Erat.
// All rights reserved.

import {
  $,
  formatTime,
  getCurrentTimeSec,
  KeyCodes,
  updateTitleAttributeForTruncation,
} from './common.js';
import OptionsDialog from './options-dialog.js';
import Updater from './updater.js';

export default class Player {
  // Number of seconds that a seek operation should traverse.
  SEEK_SECONDS = 10;

  // Number of times to retry playback after consecutive errors.
  MAX_RETRIES = 2;

  // Number of seconds that a notification is shown when the song changes.
  NOTIFICATION_SECONDS = 3;

  constructor() {
    this.songs = [];

    // Available tags.  Loaded from the server.
    this.tags = [];

    // Index into |songs| of the track currently being played.
    this.currentIndex = -1;

    // Playback position when onTimeUpdate() was last called.
    this.lastPositionSec = 0;

    // Number of consecutive playback errors.
    this.numErrors = 0;

    // Time at which we started playing the current track as seconds since the epoch.
    this.startTime = -1;

    // Total number of seconds that we've spent playing the current
    // track (ignoring paused periods).
    this.totalPlayedSec = 0;

    // Time at which onTimeUpdate() was last invoked, as seconds since the epoch.
    this.lastUpdateTime = -1;

    // Have we already reported the current track as having been played?
    this.reportedCurrentTrack = false;

    // Did we hit the end of the last song in the playlist?
    this.reachedEndOfSongs = false;

    // Song that was playing when the update div was opened.
    this.updateSong = null;

    // Rating set in the update div.
    this.updatedRating = -1.0;

    // Song notification currently being shown or null.
    this.notification = null;

    // Timeout ID for calling closeNotification().
    this.closeNotificationTimeoutId = 0;

    this.updater = new Updater();

    this.dialogManager = $('dialogManager');
    this.presentationLayer = $('presentationLayer');
    this.presentationLayer.setPlayNextTrackFunction(() => this.cycleTrack(1));
    this.optionsDialog = null;

    this.audio = $('audio');
    this.favicon = $('favicon');
    this.coverImage = $('coverImage');
    this.ratingOverlayDiv = $('ratingOverlayDiv');
    this.artistDiv = $('artistDiv');
    this.titleDiv = $('titleDiv');
    this.albumDiv = $('albumDiv');
    this.timeDiv = $('timeDiv');
    this.prevButton = $('prevButton');
    this.nextButton = $('nextButton');
    this.playPauseButton = $('playPauseButton');
    this.updateDiv = $('updateDiv');
    this.updateCloseImage = $('updateCloseImage');
    this.ratingSpan = $('ratingSpan');
    this.ratingSpan = $('ratingSpan');
    this.dumpSongLink = $('dumpSongLink');
    this.dumpSongCacheLink = $('dumpSongCacheLink');
    this.tagsTextarea = $('editTagsTextarea');

    this.audio.addEventListener('ended', () => this.onEnded(), false);
    this.audio.addEventListener('pause', () => this.onPause(), false);
    this.audio.addEventListener('play', () => this.onPlay(), false);
    this.audio.addEventListener('timeupdate', () => this.onTimeUpdate(), false);
    this.audio.addEventListener('error', e => this.onError(e), false);

    if ('mediaSession' in navigator) {
      const ms = navigator.mediaSession;
      ms.setActionHandler('play', () => this.play());
      ms.setActionHandler('pause', () => this.pause());
      ms.setActionHandler('seekbackward', () => this.seek(-this.SEEK_SECONDS));
      ms.setActionHandler('seekforward', () => this.seek(this.SEEK_SECONDS));
      ms.setActionHandler('previoustrack', () => this.cycleTrack(-1));
      ms.setActionHandler('nexttrack', () => this.cycleTrack(1));
    }

    this.coverImage.addEventListener(
      'click',
      () => this.showUpdateDiv(),
      false,
    );
    this.coverImage.addEventListener(
      'load',
      () => this.updateMediaSessionMetadata(true /* imageLoaded */),
      false,
    );
    this.prevButton.addEventListener('click', () => this.cycleTrack(-1), false);
    this.nextButton.addEventListener('click', () => this.cycleTrack(1), false);
    this.playPauseButton.addEventListener(
      'click',
      () => this.togglePause(),
      false,
    );
    this.updateCloseImage.addEventListener(
      'click',
      () => this.hideUpdateDiv(true),
      false,
    );
    this.ratingSpan.addEventListener(
      'keydown',
      e => this.handleRatingSpanKeyDown(e),
      false,
    );

    this.tagSuggester = $('editTagsSuggester');

    document.body.addEventListener(
      'keydown',
      e => this.handleBodyKeyDown(e),
      false,
    );
    window.addEventListener(
      'beforeunload',
      e => this.handleBeforeUnload(e),
      false,
    );

    this.config = document.config;
    this.config.addListener(this);
    this.onVolumeChange(this.config.getVolume());

    this.updateTagsFromServer(true /* async */);
  }

  updateTagsFromServer(async) {
    const req = new XMLHttpRequest();
    req.open('GET', 'list_tags', async);
    req.setRequestHeader('Content-type', 'application/x-www-form-urlencoded');
    req.onreadystatechange = () => {
      if (req.readyState == 4) {
        if (req.status == 200) {
          this.updateTags(JSON.parse(req.responseText));
          console.log('Loaded ' + this.tags.length + ' tags');
        } else {
          console.log('Got ' + req.status + ' while loading tags');
        }
      }
    };
    req.send(null);
  }

  getCurrentSong() {
    return this.currentIndex >= 0 && this.currentIndex < this.songs.length
      ? this.songs[this.currentIndex]
      : null;
  }

  getDumpSongUrl(song, cache) {
    return 'dump_song?id=' + song.songId + (cache ? '&cache=1' : '');
  }

  setSongs(songs) {
    const oldSong = this.getCurrentSong();

    this.songs = songs;

    // If we're currently playing a track that's no longer in the playlist,
    // then jump to the first song. Otherwise, just keep playing it (some
    // tracks were probably just appended to the previous playlist).
    const song = this.getCurrentSong();
    if (!song || oldSong != song) {
      this.audio.pause();
      this.audio.src = '';
      this.currentIndex = -1;
      this.selectTrack(0);
    } else if (this.reachedEndOfSongs) {
      this.cycleTrack(1);
    } else {
      this.updateButtonState();
      this.notifyPlaylistAboutSongChange();
    }
  }

  cycleTrack(offset) {
    this.selectTrack(this.currentIndex + offset);
  }

  selectTrack(index) {
    if (!this.songs.length) {
      this.currentIndex = -1;
      this.updateSongDisplay();
      this.updateButtonState();
      return;
    }

    if (index < 0) index = 0;
    else if (index >= this.songs.length) index = this.songs.length - 1;

    if (index == this.currentIndex) return;

    this.currentIndex = index;
    this.notifyPlaylistAboutSongChange();
    this.updateSongDisplay();
    this.updatePresentationLayerSongs();
    this.startCurrentTrack();
    this.updateButtonState();
    if (!document.hasFocus()) this.showNotification();
  }

  updateTags(tags) {
    this.tags = tags.slice(0);
    this.tagSuggester.setWords(this.tags);
    document.playlist.handleTagsUpdated(this.tags);
  }

  updateButtonState() {
    this.prevButton.disabled = this.currentIndex <= 0;
    this.nextButton.disabled =
      this.currentIndex < 0 || this.currentIndex >= this.songs.length - 1;
    this.playPauseButton.disabled = this.currentIndex < 0;
  }

  updateSongDisplay() {
    const song = this.getCurrentSong();
    document.title = song ? song.artist + ' - ' + song.title : 'Player';

    this.artistDiv.innerText = song ? song.artist : '';
    this.titleDiv.innerText = song ? song.title : '';
    this.albumDiv.innerText = song ? song.album : '';
    this.timeDiv.innerText = '';

    updateTitleAttributeForTruncation(this.artistDiv, song ? song.artist : '');
    updateTitleAttributeForTruncation(this.titleDiv, song ? song.title : '');
    updateTitleAttributeForTruncation(this.albumDiv, song ? song.album : '');

    if (song && song.coverUrl) {
      this.coverImage.src = song.coverUrl;
      this.favicon.type = 'image/jpeg';
      this.favicon.href = song.coverUrl;
    } else {
      this.coverImage.src = 'images/missing_cover.png';
      this.favicon.type = 'image/png';
      this.favicon.href = 'images/missing_cover_icon.png';
    }

    this.updateCoverTitleAttribute();
    this.updateRatingOverlay();
    // Metadata will be updated again after |coverImage| is loaded.
    this.updateMediaSessionMetadata(false /* imageLoaded */);
  }

  updateCoverTitleAttribute() {
    const song = this.getCurrentSong();
    if (!song) {
      this.coverImage.title = '';
      return;
    }

    let text = this.getRatingString(song.rating, true, true);
    if (song.tags.length > 0) text += '\nTags: ' + song.tags.sort().join(' ');
    this.coverImage.title = text;
  }

  updateRatingOverlay() {
    const song = this.getCurrentSong();
    this.ratingOverlayDiv.innerText =
      song && song.rating >= 0.0
        ? this.getRatingString(song.rating, false, false)
        : '';
  }

  updateMediaSessionMetadata(imageLoaded) {
    if (!('mediaSession' in navigator)) return;

    const song = this.getCurrentSong();
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
      const img = this.coverImage;
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

  updatePresentationLayerSongs() {
    let nextSong = null;
    if (this.currentIndex >= 0 && this.currentIndex + 1 < this.songs.length) {
      nextSong = this.songs[this.currentIndex + 1];
    }

    this.presentationLayer.updateSongs(this.getCurrentSong(), nextSong);
  }

  getRatingString(rating, withLabel, includeEmpty) {
    if (rating < 0.0) return 'Unrated';

    let ratingString = withLabel ? 'Rating: ' : '';
    const numStars = this.ratingToNumStars(rating);
    for (let i = 1; i <= 5; ++i) {
      if (i <= numStars) ratingString += '\u2605';
      else if (includeEmpty) ratingString += '\u2606';
      else break;
    }
    return ratingString;
  }

  showNotification() {
    if (!('Notification' in window)) return;

    if (Notification.permission !== 'granted') {
      if (Notification.permission !== 'denied') {
        Notification.requestPermission();
      }
      return;
    }

    if (this.notification) {
      window.clearTimeout(this.closeNotificationTimeoutId);
      this.closeNotification();
    }

    const song = this.getCurrentSong();
    if (!song) return;

    this.notification = new Notification(song.artist + '\n' + song.title, {
      body: song.album + '\n' + formatTime(song.length),
      icon: song.coverUrl,
    });
    this.closeNotificationTimeoutId = window.setTimeout(
      () => this.closeNotification(),
      this.NOTIFICATION_SECONDS * 1000,
    );
  }

  closeNotification() {
    if (!this.notification) return;

    this.notification.close();
    this.notification = null;
    this.closeNotificationTimeoutId = 0;
  }

  startCurrentTrack() {
    this.lastPositionSec = 0;
    this.numErrors = 0;
    this.startTime = getCurrentTimeSec();
    this.totalPlayedSec = 0;
    this.lastUpdateTime = -1;
    this.reportedCurrentTrack = false;
    this.reachedEndOfSongs = false;

    const song = this.getCurrentSong();
    console.log('Starting ' + song.songId + ' (' + song.url + ')');
    this.audio.src = song.url;
    this.audio.currentTime = 0;
    this.play();
  }

  play() {
    console.log('Playing');
    this.audio.play();
  }

  pause() {
    console.log('Pausing');
    this.audio.pause();
  }

  togglePause() {
    if (this.reachedEndOfSongs) {
      this.startCurrentTrack();
    } else if (this.audio.paused) {
      this.play();
    } else {
      this.pause();
    }
  }

  seek(seconds) {
    if (!this.audio.seekable) return;

    const newTime = Math.max(this.audio.currentTime + seconds, 0);
    if (newTime < this.audio.duration) this.audio.currentTime = newTime;
  }

  onEnded() {
    if (this.currentIndex >= this.songs.length - 1) {
      this.reachedEndOfSongs = true;
    } else this.cycleTrack(1);
  }

  onPause() {
    this.playPauseButton.value = 'Play';
    this.lastUpdateTime = -1;
  }

  onPlay() {
    this.playPauseButton.value = 'Pause';
    this.lastUpdateTime = getCurrentTimeSec();
  }

  onTimeUpdate() {
    const song = this.getCurrentSong();
    const duration = song ? song.length : this.audio.duration;

    this.timeDiv.innerText =
      this.audio.duration > 0
        ? '[' +
          formatTime(this.audio.currentTime) +
          ' / ' +
          formatTime(duration) +
          ']'
        : '';
    this.lastPositionSec = this.audio.currentTime;
    this.numErrors = 0;

    const now = getCurrentTimeSec();
    if (this.lastUpdateTime > 0) {
      this.totalPlayedSec += now - this.lastUpdateTime;
    }
    this.lastUpdateTime = now;

    if (!this.reportedCurrentTrack) {
      if (this.totalPlayedSec >= 240 || this.totalPlayedSec > duration / 2) {
        this.reportedCurrentTrack = true;
        this.updater.reportPlay(song.songId, this.startTime);
      }
    }

    this.presentationLayer.updatePosition(this.audio.currentTime);
  }

  onError(e) {
    this.numErrors++;

    const error = e.target.error;
    console.log('Got playback error: ' + error.code);
    switch (error.code) {
      case error.MEDIA_ERR_ABORTED: // 1
        break;
      case error.MEDIA_ERR_NETWORK: // 2
      case error.MEDIA_ERR_DECODE: // 3
      case error.MEDIA_ERR_SRC_NOT_SUPPORTED: // 4
        if (this.numErrors <= this.MAX_RETRIES) {
          console.log('Retrying from position ' + this.lastPositionSec);
          this.audio.load();
          this.audio.currentTime = this.lastPositionSec;
          this.audio.play();
        } else {
          console.log('Giving up after ' + this.numErrors + ' error(s)');
          this.cycleTrack(1);
        }
        break;
    }
  }

  notifyPlaylistAboutSongChange() {
    document.playlist.handleSongChange(this.currentIndex);
  }

  showUpdateDiv() {
    const song = this.getCurrentSong();
    if (!song) return false;

    // Already shown.
    if (this.updateSong) return true;

    this.setRating(song.rating);
    this.dumpSongLink.href = this.getDumpSongUrl(song, false);
    this.dumpSongCacheLink.href = this.getDumpSongUrl(song, true);
    this.tagsTextarea.value = song.tags.sort().join(' ');
    this.updateDiv.style.display = 'block';
    this.updateSong = song;
    return true;
  }

  hideUpdateDiv(saveChanges) {
    this.updateDiv.style.display = 'none';
    this.ratingSpan.blur();
    this.tagsTextarea.blur();

    const song = this.updateSong;
    this.updateSong = null;

    if (!song || !saveChanges) return;

    const ratingChanged = this.updatedRating != song.rating;

    const newRawTags = this.tagsTextarea.value.trim().split(/\s+/);
    let newTags = [];
    const createdTags = [];
    for (let i = 0; i < newRawTags.length; ++i) {
      let tag = newRawTags[i].toLowerCase();
      if (
        !this.tags.length ||
        this.tags.indexOf(tag) != -1 ||
        song.tags.indexOf(tag) != -1
      ) {
        newTags.push(tag);
      } else if (tag[0] == '+' && tag.length > 1) {
        tag = tag.substring(1);
        newTags.push(tag);
        if (this.tags.indexOf(tag) == -1) createdTags.push(tag);
      } else {
        console.log('Skipping unknown tag "' + tag + '"');
      }
    }
    newTags = newTags
      .sort()
      .filter((item, pos, self) => self.indexOf(item) == pos);
    const tagsChanged = newTags.join(' ') != song.tags.sort().join(' ');

    if (createdTags.length > 0) this.updateTags(this.tags.concat(createdTags));

    if (!ratingChanged && !tagsChanged) return;

    this.updater.rateAndTag(
      song.songId,
      ratingChanged ? this.updatedRating : null,
      tagsChanged ? newTags : null,
    );

    song.rating = this.updatedRating;
    song.tags = newTags;

    this.updateCoverTitleAttribute();
    if (ratingChanged) this.updateRatingOverlay();
  }

  numStarsToRating(numStars) {
    return numStars <= 0 ? -1.0 : (Math.min(numStars, 5) - 1) / 4.0;
  }

  ratingToNumStars(rating) {
    return rating < 0.0 ? 0 : 1 + Math.round(Math.min(rating, 1.0) * 4.0);
  }

  setRating(rating) {
    this.updatedRating = rating;

    // Initialize the stars the first time we show them.
    if (!this.ratingSpan.hasChildNodes()) {
      for (let i = 1; i <= 5; ++i) {
        const anchor = document.createElement('a');
        const rating = this.numStarsToRating(i);
        anchor.addEventListener('click', () => this.setRating(rating), false);
        anchor.className = 'star';
        this.ratingSpan.appendChild(anchor);
      }
    }

    const numStars = this.ratingToNumStars(rating);
    for (let i = 1; i <= 5; ++i) {
      this.ratingSpan.childNodes[i - 1].innerText =
        i <= numStars ? '\u2605' : '\u2606';
    }
  }

  showOptions() {
    if (this.optionsDialog) return;

    this.optionsDialog = new OptionsDialog(
      this.config,
      this.dialogManager,
      () => {
        this.optionsDialog = null;
      },
    );
  }

  processAccelerator(e) {
    if (this.dialogManager.getNumDialogs()) return false;

    if (e.altKey && e.keyCode == KeyCodes.D) {
      const song = this.getCurrentSong();
      if (song) window.open(this.getDumpSongUrl(song, false), '_blank');
      return true;
    } else if (e.altKey && e.keyCode == KeyCodes.N) {
      this.cycleTrack(1);
      return true;
    } else if (e.altKey && e.keyCode == KeyCodes.O) {
      this.showOptions();
      return true;
    } else if (e.altKey && e.keyCode == KeyCodes.P) {
      this.cycleTrack(-1);
      return true;
    } else if (e.altKey && e.keyCode == KeyCodes.R) {
      if (this.showUpdateDiv()) this.ratingSpan.focus();
      return true;
    } else if (e.altKey && e.keyCode == KeyCodes.T) {
      if (this.showUpdateDiv()) this.tagsTextarea.focus();
      return true;
    } else if (e.altKey && e.keyCode == KeyCodes.V) {
      if (this.presentationLayer.isShown()) this.presentationLayer.hide();
      else this.presentationLayer.show();
      return true;
    } else if (e.keyCode == KeyCodes.SPACE && !this.updateSong) {
      this.togglePause();
      return true;
    } else if (e.keyCode == KeyCodes.ENTER && this.updateSong) {
      this.hideUpdateDiv(true);
      return true;
    } else if (
      e.keyCode == KeyCodes.ESCAPE &&
      this.presentationLayer.isShown()
    ) {
      this.presentationLayer.hide();
      return true;
    } else if (e.keyCode == KeyCodes.ESCAPE && this.updateSong) {
      this.hideUpdateDiv(false);
      return true;
    } else if (e.keyCode == KeyCodes.LEFT && !this.updateSong) {
      this.seek(-Player.SEEK_SECONDS);
      return true;
    } else if (e.keyCode == KeyCodes.RIGHT && !this.updateSong) {
      this.seek(Player.SEEK_SECONDS);
      return true;
    }

    return false;
  }

  handleBodyKeyDown(e) {
    if (this.processAccelerator(e)) {
      e.preventDefault();
      e.stopPropagation();
    }
  }

  handleBeforeUnload(e) {
    this.closeNotification();
    return null;
  }

  handleRatingSpanKeyDown(e) {
    if (e.keyCode >= KeyCodes.ZERO && e.keyCode <= KeyCodes.FIVE) {
      this.setRating(this.numStarsToRating(e.keyCode - KeyCodes.ZERO));
      e.preventDefault();
      e.stopPropagation();
    } else if (e.keyCode == KeyCodes.LEFT || e.keyCode == KeyCodes.RIGHT) {
      const oldStars = this.ratingToNumStars(this.updatedRating);
      const newStars = oldStars + (e.keyCode == KeyCodes.LEFT ? -1 : 1);
      this.setRating(this.numStarsToRating(newStars));
      e.preventDefault();
      e.stopPropagation();
    }
  }

  onVolumeChange(volume) {
    this.audio.volume = volume;
  }
}
