// Copyright 2010 Daniel Erat.
// All rights reserved.

function initPlayer() {
  document.player = new Player();
}

function Player() {
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

  this.dialogManager = document.dialogManager;
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

  this.audio.addEventListener('ended', this.onEnded.bind(this), false);
  this.audio.addEventListener('pause', this.onPause.bind(this), false);
  this.audio.addEventListener('play', this.onPlay.bind(this), false);
  this.audio.addEventListener('timeupdate', this.onTimeUpdate.bind(this), false);
  this.audio.addEventListener('error', this.onError.bind(this), false);

  this.coverImage.addEventListener('click', this.showUpdateDiv.bind(this), false);
  this.prevButton.addEventListener('click', this.cycleTrack.bind(this, -1), false);
  this.nextButton.addEventListener('click', this.cycleTrack.bind(this, 1), false);
  this.playPauseButton.addEventListener('click', this.togglePause.bind(this), false);
  this.updateCloseImage.addEventListener('click', this.hideUpdateDiv.bind(this, true), false);
  this.ratingSpan.addEventListener('keydown', this.handleRatingSpanKeyDown.bind(this), false);

  this.tagSuggester = new Suggester(this.tagsTextarea, $('editTagsSuggestionsDiv'), [], false);

  document.body.addEventListener('keydown', this.handleBodyKeyDown.bind(this), false);
  window.addEventListener('beforeunload', this.handleBeforeUnload.bind(this), false);

  this.config = document.config;
  this.config.addListener(this);
  this.onVolumeChange(this.config.getVolume());

  this.updateTagsFromServer(true /* async */);
}

// Number of seconds that a seek operation should traverse.
Player.SEEK_SECONDS = 10;

// Number of times to retry playback after consecutive errors.
Player.MAX_RETRIES = 2;

// Number of seconds that a notification is shown when the song changes.
Player.NOTIFICATION_SECONDS = 3;

Player.prototype.updateTagsFromServer = function(async) {
  var req = new XMLHttpRequest();
  req.open('GET', 'list_tags', async);
  req.setRequestHeader('Content-type', 'application/x-www-form-urlencoded');
  req.onreadystatechange = function() {
    if (req.readyState == 4) {
      if (req.status == 200) {
        this.updateTags(JSON.parse(req.responseText));
        console.log('Loaded ' + this.tags.length + ' tags');
      } else {
        console.log('Got ' + req.status + ' while loading tags');
      }
    }
  }.bind(this);
  req.send(null);
};

Player.prototype.getCurrentSong = function() {
  return (this.currentIndex >= 0 && this.currentIndex < this.songs.length) ? this.songs[this.currentIndex] : null;
};

Player.prototype.getDumpSongUrl = function(song, cache) {
  return 'dump_song?id=' + song.songId + (cache ? '&cache=1' : '');
};

Player.prototype.setSongs = function(songs) {
  var oldCurrentSong = this.getCurrentSong();

  this.songs = songs;

  // If we're currently playing a track that's no longer in the playlist,
  // then jump to the first song. Otherwise, just keep playing it (some
  // tracks were probably just appended to the previous playlist).
  var song = this.getCurrentSong();
  if (!song || oldCurrentSong != song) {
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
};

Player.prototype.cycleTrack = function(offset) {
  this.selectTrack(this.currentIndex + offset);
};

Player.prototype.selectTrack = function(index) {
  if (!this.songs.length) {
    this.currentIndex = -1;
    this.updateSongDisplay();
    this.updateButtonState();
    return;
  }

  if (index < 0)
    index = 0;
  else if (index >= this.songs.length)
    index = this.songs.length - 1;

  if (index == this.currentIndex)
    return;

  this.currentIndex = index;
  this.lastPositionSec = 0;
  this.numErrors = 0;
  this.startTime = getCurrentTimeSec();
  this.totalPlayedSec = 0;
  this.lastUpdateTime = -1;
  this.reportedCurrentTrack = false;

  this.notifyPlaylistAboutSongChange();
  this.updateSongDisplay();
  this.updateButtonState();
  if (!document.hasFocus())
    this.showNotification();

  var song = this.getCurrentSong();
  console.log('Switching to ' + song.songId + ' (' + song.url + ')');
  this.audio.src = song.url;
  this.play();
  this.reachedEndOfSongs = false;
};

Player.prototype.updateTags = function(tags) {
  this.tags = tags.slice(0);
  this.tagSuggester.setWords(this.tags);
  document.playlist.handleTagsUpdated(this.tags);
};

Player.prototype.updateButtonState = function() {
  this.prevButton.disabled = this.currentIndex <= 0;
  this.nextButton.disabled = this.currentIndex < 0 || this.currentIndex >= this.songs.length - 1;
  this.playPauseButton.disabled = this.currentIndex < 0;
};

Player.prototype.updateSongDisplay = function() {
  var song = this.getCurrentSong();
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
};

Player.prototype.updateCoverTitleAttribute = function() {
  var song = this.getCurrentSong();
  if (!song) {
    this.coverImage.title = '';
    return;
  }

  var text = this.getRatingString(song.rating, true, true);
  if (song.tags.length > 0)
    text += "\nTags: " + song.tags.sort().join(' ');
  this.coverImage.title = text;
};

Player.prototype.updateRatingOverlay = function() {
  var song = this.getCurrentSong();
  this.ratingOverlayDiv.innerText = (song && song.rating >= 0.0) ? this.getRatingString(song.rating, false, false) : '';
};

Player.prototype.getRatingString = function(rating, withLabel, includeEmpty) {
  if (rating < 0.0)
    return "Unrated";

  ratingString = withLabel ? 'Rating: ' : '';
  var numStars = this.ratingToNumStars(rating);
  for (var i = 1; i <= 5; ++i) {
    if (i <= numStars)
      ratingString += "\u2605";
    else if (includeEmpty)
      ratingString += "\u2606";
    else
      break;
  }
  return ratingString;
};

Player.prototype.showNotification = function() {
  console.log('showNotification');
  if (!('Notification' in window))
    return;

  if (Notification.permission !== 'granted') {
    if (Notification.permission !== 'denied')
      Notification.requestPermission();
    return;
  }

  if (this.notification) {
    window.clearTimeout(this.closeNotificationTimeoutId);
    this.closeNotification();
  }

  var song = this.getCurrentSong();
  if (!song)
    return;

  this.notification = new Notification(
      song.artist + "\n" + song.title,
      {
        body: song.album + "\n" + formatTime(song.length),
        icon: song.coverUrl
      });
  this.closeNotificationTimeoutId = window.setTimeout(
      this.closeNotification.bind(this), Player.NOTIFICATION_SECONDS * 1000);
};

Player.prototype.closeNotification = function() {
  if (!this.notification)
    return;

  this.notification.close();
  this.notification = null;
  this.closeNotificationTimeoutId = 0;
};

Player.prototype.play = function() {
  console.log('Playing');
  this.audio.play();
};

Player.prototype.pause = function() {
  console.log('Pausing');
  this.audio.pause();
};

Player.prototype.togglePause = function() {
  if (this.audio.paused)
    this.play();
  else
    this.pause();
};

Player.prototype.seek = function(seconds) {
  if (!this.audio.seekable)
    return;

  var newTime = Math.max(this.audio.currentTime + seconds, 0);
  if (newTime < this.audio.duration)
    this.audio.currentTime = newTime;
};

Player.prototype.onEnded = function(e) {
  if (this.currentIndex >= this.songs.length - 1)
    this.reachedEndOfSongs = true;
  else
    this.cycleTrack(1);
};

Player.prototype.onPause = function(e) {
  this.playPauseButton.value = 'Play';
  this.lastUpdateTime = -1;
};

Player.prototype.onPlay = function(e) {
  this.playPauseButton.value = 'Pause';
  this.lastUpdateTime = getCurrentTimeSec();
};

Player.prototype.onTimeUpdate = function(e) {
  var song = this.getCurrentSong();
  var duration = song ? song.length : this.audio.duration;

  this.timeDiv.innerText =
      this.audio.duration > 0 ?
      '[' + formatTime(this.audio.currentTime) + ' / ' + formatTime(duration) + ']' :
      '';
  this.lastPositionSec = this.audio.currentTime;
  this.numErrors = 0;

  var now = getCurrentTimeSec();
  if (this.lastUpdateTime > 0)
    this.totalPlayedSec += (now - this.lastUpdateTime);
  this.lastUpdateTime = now;

  if (!this.reportedCurrentTrack) {
    if (this.totalPlayedSec >= 240 || this.totalPlayedSec > duration / 2)
      this.reportCurrentTrack();
  }
};

Player.prototype.onError = function(e) {
  this.numErrors++;

  var error = e.target.error;
  console.log('Got playback error: ' + error.code);
  switch (error.code) {
    case error.MEDIA_ERR_ABORTED:  // 1
      break;
    case error.MEDIA_ERR_NETWORK:            // 2
    case error.MEDIA_ERR_DECODE:             // 3
    case error.MEDIA_ERR_SRC_NOT_SUPPORTED:  // 4
      if (this.numErrors <= Player.MAX_RETRIES) {
        // TODO: Set a timeout?
        var url = this.getCurrentSong().url;
        console.log('Retrying ' + url + ' from position ' + this.lastPositionSec);
        this.audio.src = url;
        this.audio.currentTime = this.lastPositionSec;
      } else {
        console.log('Giving up after ' + this.numErrors + ' error(s)');
        this.cycleTrack(1);
      }
      break;
  }
};

Player.prototype.notifyPlaylistAboutSongChange = function() {
  document.playlist.handleSongChange(this.currentIndex);
};

Player.prototype.reportCurrentTrack = function() {
  if (this.reportedCurrentTrack)
    return;
  this.reportedCurrentTrack = true;

  var song = this.getCurrentSong();
  var url = 'report_played?songId=' + encodeURIComponent(song.songId) + '&startTime=' + encodeURIComponent(this.startTime);
  console.log("Reporting track: " + url);
  var req = new XMLHttpRequest();
  req.open('POST', url, true);
  req.send();
};

Player.prototype.showUpdateDiv = function() {
  var song = this.getCurrentSong();
  if (!song)
    return false;

  // Already shown.
  if (this.updateSong)
    return true;

  this.setRating(song.rating);
  this.dumpSongLink.href = this.getDumpSongUrl(song, false);
  this.dumpSongCacheLink.href = this.getDumpSongUrl(song, true);
  this.tagsTextarea.value = song.tags.sort().join(' ');
  this.updateDiv.style.display = 'block';
  this.updateSong = song;
  return true;
};

Player.prototype.hideUpdateDiv = function(saveChanges) {
  this.updateDiv.style.display = 'none';
  this.ratingSpan.blur();
  this.tagsTextarea.blur();

  var song = this.updateSong;
  this.updateSong = null;

  if (!song || !saveChanges)
    return;

  var ratingChanged = this.updatedRating != song.rating;

  var newRawTags = this.tagsTextarea.value.trim().split(/\s+/);
  var newTags = [];
  var createdTags = [];
  for (var i = 0; i < newRawTags.length; ++i) {
    var tag = newRawTags[i].toLowerCase();
    if (!this.tags.length || this.tags.indexOf(tag) != -1 || song.tags.indexOf(tag) != -1) {
      newTags.push(tag);
    } else if (tag[0] == '+' && tag.length > 1) {
      tag = tag.substring(1);
      newTags.push(tag);
      if (this.tags.indexOf(tag) == -1)
        createdTags.push(tag);
    } else {
      console.log('Skipping unknown tag "' + tag + '"');
    }
  }
  newTags = newTags.sort().filter(function(item, pos, self) {
    return self.indexOf(item) == pos;
  });
  var tagsChanged = newTags.join(' ') != song.tags.sort().join(' ');

  if (createdTags.length > 0)
    this.updateTags(this.tags.concat(createdTags));

  if (!ratingChanged && !tagsChanged)
    return;

  var url = 'rate_and_tag?songId=' + encodeURIComponent(song.songId);
  if (ratingChanged)
    url += '&rating=' + encodeURIComponent(this.updatedRating);
  if (tagsChanged)
    url += '&tags=' + encodeURIComponent(newTags.join(' '));
  console.log("Rating/tagging track: " + url);
  var req = new XMLHttpRequest();
  req.open('POST', url, true);
  req.send();

  song.rating = this.updatedRating;
  song.tags = newTags;

  this.updateCoverTitleAttribute();
  if (ratingChanged)
    this.updateRatingOverlay();
};

Player.prototype.numStarsToRating = function(numStars) {
  return (numStars <= 0) ? -1.0 : (Math.min(numStars, 5) - 1) / 4.0;
};

Player.prototype.ratingToNumStars = function(rating) {
  return (rating < 0.0) ? 0 : 1 + Math.round(Math.min(rating, 1.0) * 4.0);
};

Player.prototype.setRating = function(rating) {
  this.updatedRating = rating;

  // Initialize the stars the first time we show them.
  if (!this.ratingSpan.hasChildNodes()) {
    for (var i = 1; i <= 5; ++i) {
      var anchor = document.createElement('a');
      anchor.addEventListener('click', this.setRating.bind(this, this.numStarsToRating(i)), false);
      anchor.className = 'star';
      this.ratingSpan.appendChild(anchor);
    }
  }

  var numStars = this.ratingToNumStars(rating);
  for (var i = 1; i <= 5; ++i)
    this.ratingSpan.childNodes[i-1].innerText = (i <= numStars) ? "\u2605" : "\u2606";
};

Player.prototype.showOptions = function() {
  if (this.optionsDialog)
    return;

  this.optionsDialog = new OptionsDialog(this.config, this.dialogManager.createDialog());
  this.optionsDialog.setCloseCallback(this.closeOptions.bind(this));
};

Player.prototype.closeOptions = function() {
  this.dialogManager.closeDialog(this.optionsDialog.getContainer());
  this.optionsDialog = null;
};

Player.prototype.processAccelerator = function(e) {
  if (this.dialogManager.getNumDialogs())
    return false;

  if (e.altKey && e.keyCode == KeyCodes.D) {
    var song = this.getCurrentSong();
    if (song)
      window.open(this.getDumpSongUrl(song, false), '_blank');
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
    if (this.showUpdateDiv())
      this.ratingSpan.focus();
    return true;
  } else if (e.altKey && e.keyCode == KeyCodes.T) {
    if (this.showUpdateDiv())
      this.tagsTextarea.focus();
    return true;
  } else if (e.keyCode == KeyCodes.SPACE && !this.updateSong) {
    this.togglePause();
    return true;
  } else if (e.keyCode == KeyCodes.ENTER && this.updateSong) {
    this.hideUpdateDiv(true);
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
};

Player.prototype.handleBodyKeyDown = function(e) {
  if (this.processAccelerator(e)) {
    e.preventDefault();
    e.stopPropagation();
  }
};

Player.prototype.handleBeforeUnload = function(e) {
  this.closeNotification();
  return null;
};

Player.prototype.handleRatingSpanKeyDown = function(e) {
  if (e.keyCode >= KeyCodes.ZERO && e.keyCode <= KeyCodes.FIVE) {
    this.setRating(this.numStarsToRating(e.keyCode - KeyCodes.ZERO));
    e.preventDefault();
    e.stopPropagation();
  } else if (e.keyCode == KeyCodes.LEFT || e.keyCode == KeyCodes.RIGHT) {
    var oldStars = this.ratingToNumStars(this.updatedRating);
    var newStars = oldStars + (e.keyCode == KeyCodes.LEFT ? -1 : 1);
    this.setRating(this.numStarsToRating(newStars));
    e.preventDefault();
    e.stopPropagation();
  }
};

Player.prototype.onVolumeChange = function(volume) {
  this.audio.volume = volume;
};
