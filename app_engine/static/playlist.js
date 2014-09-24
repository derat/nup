// Copyright 2010 Daniel Erat.
// All rights reserved.

function initPlaylist(singleWindow) {
  document.playlist = new Playlist(singleWindow ? document.player : window.opener.document.player);
};

function Playlist(player) {
  this.player = player;
  this.currentIndex = -1;
  this.request = null;

  this.playlistTable = new SongTable(
      $('playlistTable'),
      false /* useCheckboxes */,
      function(playlist, artist) { playlist.resetSearchForm(artist, null, false); }.bind(this, this),
      function(playlist, album) { playlist.resetSearchForm(null, album, false); }.bind(this, this));

  this.rightPane = $('rightPane');
  this.keywordsInput = $('keywordsInput');
  this.keywordsClearButton = $('keywordsClearButton');
  this.tagsInput = $('tagsInput');
  this.tagsClearButton = $('tagsClearButton');
  this.shuffleCheckbox = $('shuffleCheckbox');
  this.firstTrackCheckbox = $('firstTrackCheckbox');
  this.unratedCheckbox = $('unratedCheckbox');
  this.minRatingSelect = $('minRatingSelect');
  this.maxPlaysInput = $('maxPlaysInput');
  this.firstPlayedSelect = $('firstPlayedSelect');
  this.lastPlayedSelect = $('lastPlayedSelect');

  this.searchButton = $('searchButton');
  this.resetButton = $('resetButton');
  this.luckyButton = $('luckyButton');

  this.appendButton = $('appendButton');
  this.insertButton = $('insertButton');
  this.replaceButton = $('replaceButton');

  this.searchResultsTable = new SongTable(
      $('searchResultsTable'),
      true /* useCheckboxes */,
      function(playlist, artist) { playlist.resetSearchForm(artist, null, false); }.bind(this, this),
      function(playlist, album) { playlist.resetSearchForm(null, album, false); }.bind(this, this),
      this.handleCheckedSongsChanged_.bind(this));

  this.waitingDiv = $('waitingDiv');

  this.keywordsInput.addEventListener('keydown', this.handleFormKeyDown.bind(this), false);
  this.keywordsClearButton.addEventListener('click', function(e) { this.keywordsInput.value = null; }.bind(this), false);
  this.tagsInput.addEventListener('keydown', this.handleFormKeyDown.bind(this), false);
  this.tagsClearButton.addEventListener('click', function(e) { this.tagsInput.value = null; }.bind(this), false);
  this.shuffleCheckbox.addEventListener('keydown', this.handleFormKeyDown.bind(this), false);
  this.firstTrackCheckbox.addEventListener('keydown', this.handleFormKeyDown.bind(this), false);
  this.unratedCheckbox.addEventListener('keydown', this.handleFormKeyDown.bind(this), false);
  this.unratedCheckbox.addEventListener('change', this.handleUnratedCheckboxChanged.bind(this), false);
  this.maxPlaysInput.addEventListener('keydown', this.handleFormKeyDown.bind(this), false);
  this.searchButton.addEventListener('click', this.submitQuery.bind(this, false), false);
  this.resetButton.addEventListener('click', this.resetSearchForm.bind(this, null, null, true), false);
  this.luckyButton.addEventListener('click', this.doLuckySearch.bind(this), false);
  this.appendButton.addEventListener('click', this.enqueueSearchResults.bind(this, false /* clearFirst */, false /* afterCurrent */), false);
  this.insertButton.addEventListener('click', this.enqueueSearchResults.bind(this, false, true), false);
  this.replaceButton.addEventListener('click', this.enqueueSearchResults.bind(this, true, false), false);

  this.playlistTable.updateSongs(this.player.songs);
  this.handleSongChange(this.player.currentIndex);
};

Playlist.prototype.parseQueryString = function(text) {
  var terms = [];
  var keywords = [];

  text = text.trim();
  while (text.length > 0) {
    if (text.indexOf("artist:") == 0 || text.indexOf("title:") == 0 || text.indexOf("album:") == 0) {
      var match = text.match(/^([a-z]+):\s*("[^"]*"|\S+)(.*)/);
      if (match != null) {
        var value = match[2];
        if (value[0] == '"' && value[value.length-1] == '"')
          value = value.substring(1, value.length-1);
        terms.push(match[1] + '=' + encodeURIComponent(value));
        text = match[3];
      }
    } else {
      var match = text.match(/^(\S+)(.*)/);
      keywords.push(match[1]);
      text = match[2];
    }
    text = text.trim();
  }

  if (keywords.length > 0)
    terms.push('keywords=' + encodeURIComponent(keywords.join(' ')));

  return terms;
};

Playlist.prototype.submitQuery = function(appendToQueue) {
  var terms = [];
  if (this.keywordsInput.value.trim())
    terms = terms.concat(this.parseQueryString(this.keywordsInput.value));
  if (this.tagsInput.value.trim())
    terms.push('tags=' + encodeURIComponent(this.tagsInput.value.trim()));
  if (this.minRatingSelect.value != 0 && !this.unratedCheckbox.checked)
    terms.push('minRating=' + this.minRatingSelect.value);
  if (this.shuffleCheckbox.checked)
    terms.push('shuffle=1');
  if (this.firstTrackCheckbox.checked)
    terms.push('firstTrack=1');
  if (this.unratedCheckbox.checked)
    terms.push('unrated=1');
  if (!isNaN(parseInt(this.maxPlaysInput.value)))
    terms.push('maxPlays=' + parseInt(this.maxPlaysInput.value));
  if (this.firstPlayedSelect.value != 0)
    terms.push('minFirstPlayed=' + (getCurrentTimeSec() - parseInt(this.firstPlayedSelect.value)));
  if (this.lastPlayedSelect.value != 0)
    terms.push('maxLastPlayed=' + (getCurrentTimeSec() - parseInt(this.lastPlayedSelect.value)));

  if (!terms.length) {
    alert('You must supply search terms.');
    return;
  }

  if (this.request)
    this.request.abort();

  this.request = new XMLHttpRequest();
  this.request.onreadystatechange = function() {
    if (this.request.readyState == 4) {
      var songs = [];

      try {
        var req = this.request;
        if (req.status && req.status == 200) {
          if (req.responseText) {
            songs = eval('(' + req.responseText + ')');
            console.log('Got response with ' + songs.length + ' song(s)');
          } else {
            console.log('No response text');
          }
        }
      } catch (e) {
        console.log('Caught exception while waiting for reply: ' + e);
      }

      this.searchResultsTable.updateSongs(songs);
      this.searchResultsTable.setAllCheckboxes(true);
      if (appendToQueue)
        this.enqueueSearchResults(true, true);

      this.waitingDiv.style.display = 'none';
      this.request = null;
    }
  }.bind(this);

  this.waitingDiv.style.display = 'block';
  var url = 'query?' + terms.join('&');
  console.log(url);
  this.request.open('GET', url, true);
  this.request.send();
};

Playlist.prototype.enqueueSearchResults = function(clearFirst, afterCurrent) {
  if (this.searchResultsTable.getNumSongs() == 0)
    return;

  var newSongs = clearFirst ? [] : this.player.songs.slice(0);
  var index = afterCurrent ? Math.min(this.currentIndex + 1, newSongs.length) : newSongs.length;

  var checkedSongs = this.searchResultsTable.getCheckedSongs();
  for (var i = 0; i < checkedSongs.length; i++)
    newSongs.splice(index++, 0, checkedSongs[i]);

  this.playlistTable.updateSongs(newSongs);
  this.player.setSongs(newSongs);
};

// Reset all of the fields in the search form.  If |artist| or |album| are
// non-null, the supplied values are used.  Also jumps to the top of the
// page so the form is visible.
Playlist.prototype.resetSearchForm = function(artist, album, clearResults) {
  var keywords = [];
  if (artist)
    keywords.push('artist:"' + artist + '" ');
  if (album)
    keywords.push('album:"' + album + '" ');

  this.keywordsInput.value = keywords.join(' ');
  this.tagsInput.value = null;
  this.shuffleCheckbox.checked = false;
  this.firstTrackCheckbox.checked = false;
  this.unratedCheckbox.checked = false;
  this.minRatingSelect.selectedIndex = 0;
  this.minRatingSelect.disabled = false;
  this.maxPlaysInput.value = null;
  this.firstPlayedSelect.selectedIndex = 0;
  this.lastPlayedSelect.selectedIndex = 0;
  if (clearResults)
    this.searchResultsTable.updateSongs([]);
  this.rightPane.scrollIntoView();
};

// Handles the "I'm Feeling Lucky" button being clicked.
Playlist.prototype.doLuckySearch = function() {
  if (!this.keywordsInput.value &&
      !this.tagsInput.value &&
      !this.shuffleCheckbox.checked &&
      !this.firstTrackCheckbox.checked &&
      !this.unratedCheckbox.checked &&
      this.minRatingSelect.selectedIndex == 0 &&
      !this.maxPlaysInput.value &&
      this.firstPlayedSelect.selectedIndex == 0 &&
      this.lastPlayedSelect.selectedIndex == 0) {
    this.resetSearchForm(null, null, false);
    this.shuffleCheckbox.checked = true;
    this.minRatingSelect.selectedIndex = 3;
  }
  this.submitQuery(true);
};

// Handle a key being pressed in the search form.
Playlist.prototype.handleFormKeyDown = function(e) {
  if (e.keyCode == KeyCodes.ENTER) {
    this.submitQuery(false);
  } else if (e.keyCode == KeyCodes.SPACE ||
             e.keyCode == KeyCodes.LEFT ||
             e.keyCode == KeyCodes.RIGHT) {
    e.stopPropagation();
  }
};

Playlist.prototype.handleUnratedCheckboxChanged = function(event) {
  this.minRatingSelect.disabled = this.unratedCheckbox.checked;
};

// Handle notification from the player that the current song has changed.
Playlist.prototype.handleSongChange = function(index) {
  this.playlistTable.highlightRow(this.currentIndex, false);

  if (index >= 0 && index < this.playlistTable.getNumSongs()) {
    this.playlistTable.highlightRow(index, true);
    this.currentIndex = index;
  } else {
    this.currentIndex = -1;
  }
};

Playlist.prototype.handleCheckedSongsChanged_ = function(numChecked) {
  this.appendButton.disabled = this.insertButton.disabled = this.replaceButton.disabled = (numChecked == 0);
};
