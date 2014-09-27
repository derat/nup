// Copyright 2014 Daniel Erat.
// All rights reserved.

function SongTable(table, useCheckboxes, artistClickedCallback, albumClickedCallback, checkedSongsChangedCallback) {
  this.table_ = table;
  this.useCheckboxes_ = useCheckboxes;
  this.artistClickedCallback_ = artistClickedCallback;
  this.albumClickedCallback_ = albumClickedCallback;

  if (useCheckboxes) {
    this.headingCheckbox_ = this.table_.rows[0].cells[0].childNodes[0];
    this.headingCheckbox_.addEventListener('click', this.handleCheckboxClicked_.bind(this, this.headingCheckbox_), false);
    this.checkedSongsChangedCallback_ = checkedSongsChangedCallback;
    this.numCheckedSongs_ = 0;
  }
}

SongTable.prototype.getNumSongs = function() {
  return this.table_.rows.length - 1;
};

SongTable.prototype.highlightRow = function(index, highlight) {
  if (index >= 0 && index < this.getNumSongs())
    this.table_.rows[index+1].className = highlight ? 'playing' : null;
};

SongTable.prototype.getCheckedSongs = function() {
  if (!this.useCheckboxes_)
    return [];

  var songs = [];
  for (var i = 1; i < this.table_.rows.length; i++) {
    var row = this.table_.rows[i];
    if (row.cells[0].children[0].checked)
      songs.push(row.song);
  }
  return songs;
};

SongTable.prototype.setAllCheckboxes = function(checked) {
  if (!this.useCheckboxes_)
    return;

  this.headingCheckbox_.checked = checked ? 'checked' : null;
  this.handleCheckboxClicked_(this.headingCheckbox_);
};

// Update to contain |newSongs|.
// Try to be smart about not doing any more work than necessary.
SongTable.prototype.updateSongs = function(newSongs) {
  var oldSongs = [];
  for (var i = 1; i < this.table_.rows.length; i++)  // Start at 1 to skip the header.
    oldSongs.push(this.table_.rows[i].song);

  // Walk forward from the beginning and backward from the end to look for common runs of songs.
  var minLength = Math.min(oldSongs.length, newSongs.length);
  var startMatchLength = 0, endMatchLength = 0;
  for (var i = 0; i < minLength && oldSongs[i].songId == newSongs[i].songId; i++)
    startMatchLength++;
  for (var i = 0; i < (minLength - startMatchLength) && oldSongs[oldSongs.length-i-1].songId == newSongs[newSongs.length-i-1].songId; i++)
    endMatchLength++;

  // Figure out how many songs in the middle differ.
  var numOldMiddleSongs = oldSongs.length - startMatchLength - endMatchLength;
  var numNewMiddleSongs = newSongs.length - startMatchLength - endMatchLength;

  // Get to the correct number of rows.
  for (var i = numOldMiddleSongs; i < numNewMiddleSongs; i++)
    this.initRow_(this.table_.insertRow(startMatchLength+1));
  for (var i = numOldMiddleSongs; i > numNewMiddleSongs; i--)
    this.table_.deleteRow(startMatchLength+1);

  // Update all of the rows in the middle to contain the correct data.
  for (var i = 0; i < numNewMiddleSongs; i++) {
    var song = newSongs[startMatchLength + i];
    var row = this.table_.rows[startMatchLength + i + 1];
    this.updateRow_(row, song);
  }

  // Clear all of the checkboxes.
  if (this.useCheckboxes_) {
    for (var i = 1; i < this.table_.rows.length; i++)
      this.table_.rows[i].cells[0].children[0].checked = null;
    this.numCheckedSongs_ = 0;
    this.updateHeadingCheckbox_();
    if (this.checkedSongsChangedCallback_)
      this.checkedSongsChangedCallback_(this.numCheckedSongs_);
  }
};

// Initialize newly-created |row| to contain song data.
SongTable.prototype.initRow_ = function(row) {
  // Checkbox.
  if (this.useCheckboxes_) {
    var cell = row.insertCell(-1);
    cell.className = 'checkbox';
    var checkbox = document.createElement('input');
    checkbox.type = 'checkbox';
    checkbox.checked = 'checked';
    checkbox.addEventListener('click', this.handleCheckboxClicked_.bind(this, checkbox), false);
    cell.appendChild(checkbox);
  }

  // Artist.
  var anchor = document.createElement('a');
  anchor.addEventListener('click', this.handleArtistClicked_.bind(this, row), false);
  row.insertCell(-1).appendChild(anchor);

  // Title.
  row.insertCell(-1);

  // Album.
  var anchor = document.createElement('a');
  anchor.addEventListener('click', this.handleAlbumClicked_.bind(this, row), false);
  row.insertCell(-1).appendChild(anchor);

  // Time.
  row.insertCell(-1).className = 'time';
};

// Update |row| to display data about |song|.
SongTable.prototype.updateRow_ = function(row, song) {
  row.song = song;

  var updateCell = function(cell, text, updateChild) {
    (updateChild ? cell.childNodes[0] : cell).innerText = text;
    updateTitleAttributeForTruncation(cell, text);
  };

  // Skip the checkbox if present.
  var artistCellIndex = this.useCheckboxes_ ? 1 : 0;
  updateCell(row.cells[artistCellIndex], song.artist, true);
  updateCell(row.cells[artistCellIndex+1], song.title, false);
  updateCell(row.cells[artistCellIndex+2], song.album, true);
  updateCell(row.cells[artistCellIndex+3], formatTime(parseFloat(song.length)), false);

  // Clear highlighting.
  row.className = null;
};

// Callback for the artist name being clicked in |row|.
SongTable.prototype.handleArtistClicked_ = function(row) {
  if (this.artistClickedCallback_)
    this.artistClickedCallback_(row.song.artist);
};

// Callback for the album name being clicked in |row|.
SongTable.prototype.handleAlbumClicked_ = function(row) {
  if (this.albumClickedCallback_)
    this.albumClickedCallback_(row.song.album);
};

// Handle one of the checkboxes being clicked.
SongTable.prototype.handleCheckboxClicked_ = function(checkbox) {
  var head = this.headingCheckbox_;
  var checked = checkbox.checked;

  if (checkbox == head) {
    for (var i = 1; i < this.table_.rows.length; i++)
      this.table_.rows[i].cells[0].children[0].checked = checked ? 'checked' : null;
    this.numCheckedSongs_ = checked ? this.getNumSongs() : 0;
  } else {
    this.numCheckedSongs_ += checked ? 1 : -1;
  }
  this.updateHeadingCheckbox_();

  if (this.checkedSongsChangedCallback_)
    this.checkedSongsChangedCallback_(this.numCheckedSongs_);
};

// Update the |headingCheckbox_|'s visual state for the current number of checked songs.
SongTable.prototype.updateHeadingCheckbox_ = function() {
  var head = this.headingCheckbox_;
  if (this.numCheckedSongs_ == 0) {
    head.checked = null;
    head.className = 'opaque';
  } else {
    head.checked = 'checked';
    head.className = (this.numCheckedSongs_ == this.getNumSongs()) ? 'opaque' : 'transparent';
  }
};
