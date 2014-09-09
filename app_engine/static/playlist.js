// Copyright 2010 Daniel Erat.
// All rights reserved.

function initPlaylist(singleWindow) {
  document.playlist = new Playlist(singleWindow ? document.player : window.opener.document.player);
};

function Playlist(player) {
  this.player = player;
  this.currentIndex = -1;
  this.searchResults = [];
  this.numSelectedSearchResults = 0;
  this.request = null;

  this.playlistTable = $('playlistTable');

  this.rightPane = $('rightPane');
  this.artistInput = $('artistInput');
  this.artistClearButton = $('artistClearButton');
  this.titleInput = $('titleInput');
  this.titleClearButton = $('titleClearButton');
  this.albumInput = $('albumInput');
  this.albumClearButton = $('albumClearButton');
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

  this.searchResultsTable = $('searchResultsTable');
  this.searchResultsCheckbox = $('searchResultsCheckbox');
  this.waitingDiv = $('waitingDiv');

  this.artistInput.addEventListener('keydown', this.handleFormKeyDown.bind(this), false);
  this.artistClearButton.addEventListener('click', function(e) { this.artistInput.value = null; }.bind(this), false);
  this.titleClearButton.addEventListener('click', function(e) { this.titleInput.value = null; }.bind(this), false);
  this.albumClearButton.addEventListener('click', function(e) { this.albumInput.value = null; }.bind(this), false);
  this.keywordsClearButton.addEventListener('click', function(e) { this.keywordsInput.value = null; }.bind(this), false);
  this.tagsClearButton.addEventListener('click', function(e) { this.tagsInput.value = null; }.bind(this), false);
  this.titleInput.addEventListener('keydown', this.handleFormKeyDown.bind(this), false);
  this.albumInput.addEventListener('keydown', this.handleFormKeyDown.bind(this), false);
  this.keywordsInput.addEventListener('keydown', this.handleFormKeyDown.bind(this), false);
  this.tagsInput.addEventListener('keydown', this.handleFormKeyDown.bind(this), false);
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
  this.searchResultsCheckbox.addEventListener('click', this.handleSearchResultsCheckboxClicked.bind(this, -1), false);

  this.refreshSongTable(this.playlistTable, this.player.songs);
  this.handleSongChange(this.player.currentIndex);
};

// Update |table| to contain |songs|.
Playlist.prototype.refreshSongTable = function(table, songs) {
  while (table.rows.length > 1)
    table.deleteRow(-1);

  var hasCheckboxes = table.rows[0].cells.length == 5;
  for (var i = 0; i < songs.length; i++) {
    var song = songs[i];
    var row = table.insertRow(-1);
    row.song = song;

    if (hasCheckboxes) {
      var cell = row.insertCell(-1);
      cell.className = 'checkbox';
      var checkbox = document.createElement('input');
      checkbox.type = 'checkbox';
      checkbox.checked = 'checked';
      checkbox.addEventListener('click', this.handleSearchResultsCheckboxClicked.bind(this, i), false);
      cell.appendChild(checkbox);
    }

    var cell = row.insertCell(-1);
    cell.className = 'artist';
    var anchor = document.createElement('a');
    anchor.innerText = song.artist;
    anchor.addEventListener('click', this.resetSearchForm.bind(this, song.artist, null, false), false);
    cell.appendChild(anchor);
    updateTitleAttributeForTruncation(cell, song.artist);

    var cell = row.insertCell(-1);
    cell.innerText = song.title;
    cell.className = 'title';
    updateTitleAttributeForTruncation(cell, song.title);

    var cell = row.insertCell(-1);
    var anchor = document.createElement('a');
    anchor.innerText = song.album;
    anchor.addEventListener('click', this.resetSearchForm.bind(this, null, song.album, false), false);
    cell.appendChild(anchor);
    updateTitleAttributeForTruncation(cell, song.album);

    var cell = row.insertCell(-1);
    cell.innerText = formatTime(parseFloat(song.length));
    cell.className = 'time';
  }
};

Playlist.prototype.submitQuery = function(appendToQueue) {
  var terms = [];
  if (this.artistInput.value.trim())
    terms.push('artist=' + encodeURIComponent(this.artistInput.value.trim()));
  if (this.titleInput.value.trim())
    terms.push('title=' + encodeURIComponent(this.titleInput.value.trim()));
  if (this.albumInput.value.trim())
    terms.push('album=' + encodeURIComponent(this.albumInput.value.trim()));
  if (this.keywordsInput.value.trim())
    terms.push('keywords=' + encodeURIComponent(this.keywordsInput.value.trim()));
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
    terms.push('firstPlayed=' + this.firstPlayedSelect.value);
  if (this.lastPlayedSelect.value != 0)
    terms.push('lastPlayed=' + this.lastPlayedSelect.value);

  if (!terms.length) {
    alert('You must supply search terms.');
    return;
  }

  if (this.request)
    this.request.abort();

  this.request = new XMLHttpRequest();
  this.request.onreadystatechange = function() {
    if (this.request.readyState == 4) {
      try {
        var req = this.request;
        if (req.status && req.status == 200) {
          if (req.responseText) {
            var songs = eval('(' + req.responseText + ')');
            console.log('Got response with ' + songs.length + ' song(s)');

            this.refreshSongTable(this.searchResultsTable, songs);
            var checkbox = this.searchResultsCheckbox;
            checkbox.checked = 'checked';
            checkbox.className = 'opaque';
            this.numSelectedSearchResults = songs.length;
            this.searchResults = songs;
            if (appendToQueue)
              this.enqueueSearchResults(true, true);
          }
        }
      } catch (e) {
        console.log('Caught exception while waiting for reply: ' + e);
      }
      this.waitingDiv.style.display = 'none';
      this.request = null;
    }
  }.bind(this);

  this.waitingDiv.style.display = 'block';
  var url = 'query?' + terms.join('&');
  this.request.open('GET', url, true);
  this.request.send();
};

Playlist.prototype.enqueueSearchResults = function(clearFirst, afterCurrent) {
  if (!this.searchResults)
    return;

  var newSongs = clearFirst ? [] : this.player.songs;
  var index = afterCurrent ? Math.min(this.currentIndex + 1, newSongs.length) : newSongs.length;

  var table = this.searchResultsTable;
  for (var i = 0; i < this.searchResults.length; ++i) {
    if (table.rows[i+1].cells[0].children[0].checked) {
      newSongs.splice(index, 0, this.searchResults[i]);
      index++;
    }
  }

  this.refreshSongTable(this.playlistTable, newSongs);
  this.player.setSongs(newSongs);
};

// Reset all of the fields in the search form.  If |artist| or |album| are
// non-null, the supplied values are used.  Also jumps to the top of the
// page so the form is visible.
Playlist.prototype.resetSearchForm = function(artist, album, clearResults) {
  this.artistInput.value = artist ? artist : null;
  this.titleInput.value = null;
  this.albumInput.value = album ? album : null;
  this.keywordsInput.value = null;
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
    this.refreshSongTable(this.searchResultsTable, []);
  this.rightPane.scrollIntoView();
};

// Handles the "I'm Feeling Lucky" button being clicked.
Playlist.prototype.doLuckySearch = function() {
  if (!this.artistInput.value &&
      !this.titleInput.value &&
      !this.albumInput.value &&
      !this.keywordsInput.value &&
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

// Handle one of the checkboxes in the search results being clicked.
// |index| is -1 for the checkbox in the heading and the 0-indexed position
// of the search result otherwise.
Playlist.prototype.handleSearchResultsCheckboxClicked = function(index) {
  var table = this.searchResultsTable;
  var headingCheckbox = this.searchResultsCheckbox;

  if (index == -1) {
    var selected = headingCheckbox.checked;
    for (var i = 0; i < this.searchResults.length; ++i)
      table.rows[i+1].cells[0].children[0].checked = selected ? 'checked' : null;
    this.numSelectedSearchResults = selected ? this.searchResults.length : 0;
    headingCheckbox.className = 'opaque';

  } else {
    var selected = table.rows[index+1].cells[0].children[0].checked;
    if (selected)
      this.numSelectedSearchResults++;
    else
      this.numSelectedSearchResults--;

    if (this.numSelectedSearchResults == 0) {
      headingCheckbox.checked = null;
      headingCheckbox.className = 'opaque';
    } else {
      headingCheckbox.checked = 'checked';
      if (this.numSelectedSearchResults == this.searchResults.length)
        headingCheckbox.className = 'opaque';
      else
        headingCheckbox.className = 'transparent';
    }
  }
};

Playlist.prototype.handleUnratedCheckboxChanged = function(event) {
  this.minRatingSelect.disabled = this.unratedCheckbox.checked;
};

// Handle notification from the player that the current song has changed.
Playlist.prototype.handleSongChange = function(index) {
  var table = this.playlistTable;

  if (this.currentIndex >= 0 && this.currentIndex < table.rows.length - 1)
    table.rows[this.currentIndex+1].className = null;

  if (index >= 0 && index < table.rows.length - 1) {
    table.rows[index+1].className = 'playing';
    this.currentIndex = index;
  } else {
    this.currentIndex = -1;
  }
};
