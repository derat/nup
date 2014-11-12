// Copyright 2010 Daniel Erat.
// All rights reserved.

function initPlayer() {
  document.player = new Player();
};

function Player() {
  this.songs = [];

  // Available tags.  Loaded from the server.
  this.tags = [];

  // Index into |songs| of the track currently being played.
  this.currentIndex = -1;

  // Time at which we started playing the current track as seconds since the epoch.
  this.startTime = -1;

  // Total number of seconds that we've spent playing the current
  // track (ignoring paused periods).
  this.totalPlayedSec = 0;

  // Time at which |onTimeUpdate()| was last invoked, as seconds since the epoch.
  this.lastUpdateTime = -1;

  // Have we already reported the current track as having been played?
  this.reportedCurrentTrack = false;

  // Did we hit the end of the last song in the playlist?
  this.reachedEndOfSongs = false;

  // Song that was playing when the update div was opened.
  this.updateSong = null;

  // Rating set in the update div.
  this.updatedRating = -1.0;

  this.audio = $('audio');
  this.favicon = $('favicon');
  this.coverImage = $('coverImage');
  this.ratingOverlayDiv = $('ratingOverlayDiv');
  this.artistDiv = $('artistDiv');
  this.titleDiv = $('titleDiv');
  this.albumDiv = $('albumDiv');
  this.timeDiv = $('timeDiv');
  this.playlistButton = $('playlistButton');
  this.prevButton = $('prevButton');
  this.nextButton = $('nextButton');
  this.playPauseButton = $('playPauseButton');
  this.updateDiv = $('updateDiv');
  this.updateCloseImage = $('updateCloseImage');
  this.ratingSpan = $('ratingSpan');
  this.ratingSpan = $('ratingSpan');
  this.dumpSongLink = $('dumpSongLink');
  this.dumpSongCacheLink = $('dumpSongCacheLink');
  this.tagTextarea = $('tagTextarea');
  this.tagSuggestionsDiv = $('tagSuggestionsDiv');

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

  this.tagTextarea.addEventListener('keydown', this.handleTagTextareaKeyDown.bind(this), false);
  this.tagTextarea.addEventListener('focus', this.handleTagTextareaFocus.bind(this), false);
  this.tagTextarea.addEventListener('blur', this.handleTagTextareaBlur.bind(this), false);
  this.tagTextarea.spellcheck = false;

  document.body.addEventListener('keydown', this.handleBodyKeyDown.bind(this), false);

  var req = new XMLHttpRequest();
  req.open('GET', 'list_tags', true);
  req.setRequestHeader('Content-type', 'application/x-www-form-urlencoded');
  req.onreadystatechange = function() {
    if (req.readyState == 4) {
      if (req.status == 200) {
        this.tags = JSON.parse(req.responseText);
        console.log('Loaded ' + this.tags.length + ' tags');
      } else {
        console.log('Got ' + req.status + ' while loading tags');
      }
    }
  }.bind(this);
  req.send(null);
};

// Number of seconds that a seek operation should traverse.
Player.SEEK_SECONDS = 10;

Player.prototype.getCurrentSong = function() {
  return (this.currentIndex >= 0 && this.currentIndex < this.songs.length) ? this.songs[this.currentIndex] : null;
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
  this.startTime = getCurrentTimeSec();
  this.totalPlayedSec = 0;
  this.lastUpdateTime = -1;
  this.reportedCurrentTrack = false;

  this.notifyPlaylistAboutSongChange();
  this.updateSongDisplay();
  this.updateButtonState();

  var song = this.getCurrentSong();
  console.log('Switching to ' + song.songId + ' (' + song.url + ')');
  this.audio.src = song.url;
  this.play();
  this.reachedEndOfSongs = false;
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

  updateTitleAttributeForTruncation(this.artistDiv, song.artist);
  updateTitleAttributeForTruncation(this.titleDiv, song.title);
  updateTitleAttributeForTruncation(this.albumDiv, song.album);

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
  var error = this.audio.error;
  console.log('got playback error: ' + error.code);
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
  this.dumpSongLink.href = 'dump_song?id=' + song.songId;
  this.dumpSongCacheLink.href = 'dump_song?id=' + song.songId + '&cache=1';
  this.tagTextarea.value = song.tags.sort().join(' ');
  this.updateDiv.style.display = 'block';
  this.updateSong = song;
  return true;
};

Player.prototype.hideUpdateDiv = function(saveChanges) {
  this.updateDiv.style.display = 'none';
  this.ratingSpan.blur();
  this.tagTextarea.blur();

  var song = this.updateSong;
  this.updateSong = null;

  if (!song || !saveChanges)
    return;

  var ratingChanged = this.updatedRating != song.rating;

  var newRawTags = this.tagTextarea.value.trim().split(/\s+/);
  var newTags = [];
  for (var i = 0; i < newRawTags.length; ++i) {
    var tag = newRawTags[i].toLowerCase();
    if (!this.tags.length || this.tags.indexOf(tag) != -1 || song.tags.indexOf(tag) != -1) {
      newTags.push(tag);
    } else if (tag[0] == '+' && tag.length > 1) {
      tag = tag.substring(1);
      newTags.push(tag);
      if (this.tags.indexOf(tag) == -1)
        this.tags.push(tag);
    } else {
      console.log('Skipping unknown tag "' + tag + '"');
    }
  }
  newTags = newTags.sort().filter(function(item, pos, self) {
    return self.indexOf(item) == pos;
  });
  var tagsChanged = newTags.join(' ') != song.tags.sort().join(' ');

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

Player.prototype.processAccelerator = function(e) {
  if (e.altKey && e.keyCode == KeyCodes.R) {
    if (this.showUpdateDiv())
      this.ratingSpan.focus();
    return true;
  }

  if (e.altKey && e.keyCode == KeyCodes.T) {
    if (this.showUpdateDiv())
      this.tagTextarea.focus();
    return true;
  }

  if (e.altKey && e.keyCode == KeyCodes.N) {
    this.cycleTrack(1);
    return true;
  }

  if (e.altKey && e.keyCode == KeyCodes.P) {
    this.cycleTrack(-1);
    return true;
  }

  if (e.keyCode == KeyCodes.SPACE && !this.updateSong) {
    this.togglePause();
    return true;
  }

  if (e.keyCode == KeyCodes.ENTER && this.updateSong) {
    this.hideUpdateDiv(true);
    return true;
  }

  if (e.keyCode == KeyCodes.ESCAPE && this.updateSong) {
    this.hideUpdateDiv(false);
    return true;
  }

  if (e.keyCode == KeyCodes.LEFT && !this.updateSong) {
    this.seek(-Player.SEEK_SECONDS);
    return true;
  }

  if (e.keyCode == KeyCodes.RIGHT && !this.updateSong) {
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

Player.prototype.findTagsWithPrefix = function(tags, prefix) {
  var matchingTags = [];
  for (var i = 0; i < tags.length; ++i) {
    if (tags[i].indexOf(prefix) == 0)
      matchingTags.push(tags[i]);
  }
  return matchingTags;
};

Player.prototype.handleTagTextareaKeyDown = function(e) {
  this.hideTagSuggestions();

  if (e.keyCode == KeyCodes.TAB) {
    var text = this.tagTextarea.value;

    var tagStart = this.tagTextarea.selectionStart;
    while (tagStart > 0 && text[tagStart - 1] != ' ')
      tagStart--;

    var tagEnd = this.tagTextarea.selectionStart;
    while (tagEnd < text.length && text[tagEnd] != ' ')
      tagEnd++;

    var before = text.substring(0, tagStart);
    var after = text.substring(tagEnd, text.length);
    var tagPrefix = text.substring(tagStart, tagEnd);
    var matchingTags = this.findTagsWithPrefix(this.tags, tagPrefix);

    if (matchingTags.length == 1) {
      var tag = matchingTags[0];
      text = before + tag + (after.length == 0 ? ' ' : after);
      this.tagTextarea.value = text;

      var nextTagStart = tagStart + tag.length;
      while (nextTagStart < text.length && text[nextTagStart] == ' ')
        nextTagStart++;
      this.tagTextarea.selectionStart = this.tagTextarea.selectionEnd = nextTagStart;
    } else if (matchingTags.length > 1) {
      var longestSharedPrefix = tagPrefix;
      for (var length = tagPrefix.length + 1; length <= matchingTags[0].length; ++length) {
        var newPrefix = matchingTags[0].substring(0, length);
        if (this.findTagsWithPrefix(matchingTags, newPrefix).length == matchingTags.length)
          longestSharedPrefix = newPrefix;
        else
          break;
      }

      this.tagTextarea.value = before + longestSharedPrefix + after;
      this.tagTextarea.selectionStart = this.tagTextarea.selectionEnd = before.length + longestSharedPrefix.length;
      this.showTagSuggestions(matchingTags);
    }

    e.preventDefault();
    e.stopPropagation();
  }
};

Player.prototype.handleTagTextareaFocus = function(e) {
  var text = this.tagTextarea.value;
  if (text.length > 0 && text[text.length - 1] != ' ')
    this.tagTextarea.value += ' ';
  this.tagTextarea.selectionStart = this.tagTextarea.selectionEnd = this.tagTextarea.value.length;
};


Player.prototype.handleTagTextareaBlur = function(e) {
  this.hideTagSuggestions();
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

Player.prototype.showTagSuggestions = function(tags) {
  this.tagSuggestionsDiv.innerText = tags.sort().join(' ');
  this.tagSuggestionsDiv.className = 'shown';
};

Player.prototype.hideTagSuggestions = function() {
  this.tagSuggestionsDiv.className = '';
};
