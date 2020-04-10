// Copyright 2014 Daniel Erat.
// All rights reserved.

import {
  $,
  createElement,
  createShadow,
  createTemplate,
  formatTime,
  updateTitleAttributeForTruncation,
} from './common.js';

const tableTemplate = createTemplate(`
<table id="table">
  <thead>
    <tr id="head-row">
      <th class="artist">Artist</th>
      <th class="title">Title</th>
      <th class="album">Album</th>
      <th class="time">Time</th>
    </tr>
  </thead>
</table>
`);

const checkboxTemplate = createTemplate(`
<th class="checkbox">
  <input type="checkbox" id="head-checkbox" />
</th>
`);

class SongTable extends HTMLElement {
  constructor() {
    super();

    this.useCheckboxes_ = this.hasAttribute('use-checkboxes');
    this.lastClickedCheckboxIndex_ = -1; // 0 is header

    this.style.display = 'block';
    this.shadow_ = createShadow(this, 'song-table.css');
    this.shadow_.appendChild(tableTemplate.content.cloneNode(true));
    this.table_ = $('table', this.shadow_);

    if (this.useCheckboxes_) {
      const tr = $('head-row', this.shadow_);
      tr.insertBefore(checkboxTemplate.content.cloneNode(true), tr.firstChild);
      this.headingCheckbox_ = $('head-checkbox', this.shadow_);
      this.headingCheckbox_.addEventListener(
        'click',
        e => this.handleCheckboxClicked_(this.headingCheckbox_, e),
        false,
      );
      this.numCheckedSongs_ = 0;
    }
  }

  setArtistClickedCallback(cb) {
    this.artistClickedCallback_ = cb;
  }
  setAlbumClickedCallback(cb) {
    this.albumClickedCallback_ = cb;
  }
  setCheckedSongsChangedCallback(cb) {
    this.checkedSongsChangedCallback_ = cb;
  }

  getNumSongs() {
    return this.table_.rows.length - 1;
  }

  highlightRow(index, highlight) {
    if (index >= 0 && index < this.getNumSongs()) {
      this.table_.rows[index + 1].className = highlight ? 'highlight' : '';
    }
  }

  getCheckedSongs() {
    if (!this.useCheckboxes_) return [];

    const songs = [];
    for (let i = 1; i < this.table_.rows.length; i++) {
      const row = this.table_.rows[i];
      if (row.cells[0].children[0].checked) songs.push(row.song);
    }
    return songs;
  }

  setAllCheckboxes(checked) {
    if (!this.useCheckboxes_) return;

    this.headingCheckbox_.checked = checked ? 'checked' : null;
    this.handleCheckboxClicked_(this.headingCheckbox_);
  }

  // Update to contain |newSongs|.
  // Try to be smart about not doing any more work than necessary.
  updateSongs(newSongs) {
    const oldSongs = [];
    for (
      let i = 1; // Start at 1 to skip the header.
      i < this.table_.rows.length;
      i++
    ) {
      oldSongs.push(this.table_.rows[i].song);
    }

    // Walk forward from the beginning and backward from the end to look for common runs of songs.
    const minLength = Math.min(oldSongs.length, newSongs.length);
    let startMatchLength = 0;
    let endMatchLength = 0;
    for (
      let i = 0;
      i < minLength && oldSongs[i].songId == newSongs[i].songId;
      i++
    ) {
      startMatchLength++;
    }
    for (
      let i = 0;
      i < minLength - startMatchLength &&
      oldSongs[oldSongs.length - i - 1].songId ==
        newSongs[newSongs.length - i - 1].songId;
      i++
    ) {
      endMatchLength++;
    }

    // Figure out how many songs in the middle differ.
    const numOldMiddleSongs =
      oldSongs.length - startMatchLength - endMatchLength;
    const numNewMiddleSongs =
      newSongs.length - startMatchLength - endMatchLength;

    // Get to the correct number of rows.
    for (let i = numOldMiddleSongs; i < numNewMiddleSongs; i++) {
      this.initRow_(this.table_.insertRow(startMatchLength + 1));
    }
    for (let i = numOldMiddleSongs; i > numNewMiddleSongs; i--) {
      this.table_.deleteRow(startMatchLength + 1);
    }

    // Update all of the rows in the middle to contain the correct data.
    for (let i = 0; i < numNewMiddleSongs; i++) {
      const song = newSongs[startMatchLength + i];
      const row = this.table_.rows[startMatchLength + i + 1];
      this.updateRow_(row, song);
    }

    // Clear all of the checkboxes.
    if (this.useCheckboxes_) {
      for (let i = 1; i < this.table_.rows.length; i++) {
        this.table_.rows[i].cells[0].children[0].checked = null;
      }
      this.numCheckedSongs_ = 0;
      this.updateHeadingCheckbox_();
      if (this.checkedSongsChangedCallback_) {
        this.checkedSongsChangedCallback_(this.numCheckedSongs_);
      }
    }

    this.lastClickedCheckboxIndex_ = -1;
  }

  // Initialize newly-created |row| to contain song data.
  initRow_(row) {
    // Checkbox.
    if (this.useCheckboxes_) {
      const cell = row.insertCell();
      cell.className = 'checkbox';
      const checkbox = createElement('input', undefined, cell);
      checkbox.type = 'checkbox';
      checkbox.checked = 'checked';
      checkbox.addEventListener(
        'click',
        e => this.handleCheckboxClicked_(checkbox, e),
        false,
      );
    }

    createElement('a', undefined, row.insertCell()).addEventListener(
      'click',
      () => this.handleArtistClicked_(row),
      false,
    ); // artist
    row.insertCell(); // title
    createElement('a', undefined, row.insertCell()).addEventListener(
      'click',
      () => this.handleAlbumClicked_(row),
      false,
    ); // album
    row.insertCell().className = 'time'; // time
  }

  // Update |row| to display data about |song|.
  updateRow_(row, song) {
    row.song = song;

    const updateCell = (cell, text, updateChild) => {
      (updateChild ? cell.children[0] : cell).innerText = text;
      updateTitleAttributeForTruncation(cell, text);
    };

    // Skip the checkbox if present.
    const artistCellIndex = this.useCheckboxes_ ? 1 : 0;
    updateCell(row.cells[artistCellIndex], song.artist, true);
    updateCell(row.cells[artistCellIndex + 1], song.title, false);
    updateCell(row.cells[artistCellIndex + 2], song.album, true);
    updateCell(
      row.cells[artistCellIndex + 3],
      formatTime(parseFloat(song.length)),
      false,
    );

    // Clear highlighting.
    row.className = null;
  }

  // Callback for the artist name being clicked in |row|.
  handleArtistClicked_(row) {
    if (this.artistClickedCallback_) {
      this.artistClickedCallback_(row.song.artist);
    }
  }

  // Callback for the album name being clicked in |row|.
  handleAlbumClicked_(row) {
    if (this.albumClickedCallback_) this.albumClickedCallback_(row.song.album);
  }

  // Handle one of the checkboxes being clicked.
  handleCheckboxClicked_(checkbox, e) {
    const table = this.table_;
    const getCheckbox = index => table.rows[index].cells[0].children[0];
    let index = -1;
    for (let i = 0; i < table.rows.length; i++) {
      if (checkbox == getCheckbox(i)) {
        index = i;
        break;
      }
    }
    const checked = checkbox.checked;

    if (index == 0) {
      for (let i = 1; i < table.rows.length; i++) {
        getCheckbox(i).checked = checked ? 'checked' : null;
      }
      this.numCheckedSongs_ = checked ? this.getNumSongs() : 0;
    } else {
      this.numCheckedSongs_ += checked ? 1 : -1;

      if (e && e.shiftKey) {
        if (
          this.lastClickedCheckboxIndex_ > 0 &&
          this.lastClickedCheckboxIndex_ < table.rows.length &&
          this.lastClickedCheckboxIndex_ != index
        ) {
          const start = Math.min(index, this.lastClickedCheckboxIndex_);
          const end = Math.max(index, this.lastClickedCheckboxIndex_);
          for (let i = start; i <= end; i++) {
            const c = getCheckbox(i);
            if (checked && !c.checked) {
              c.checked = 'checked';
              this.numCheckedSongs_++;
            } else if (!checked && c.checked) {
              c.checked = null;
              this.numCheckedSongs_--;
            }
          }
        }
      }
    }

    this.updateHeadingCheckbox_();
    this.lastClickedCheckboxIndex_ = index;

    if (this.checkedSongsChangedCallback_) {
      this.checkedSongsChangedCallback_(this.numCheckedSongs_);
    }
  }

  // Update the |headingCheckbox_|'s visual state for the current number of checked songs.
  updateHeadingCheckbox_() {
    const head = this.headingCheckbox_;
    head.checked = !this.numCheckedSongs_ ? null : 'checked';
    head.className =
      !this.numCheckedSongs_ || this.numCheckedSongs_ == this.getNumSongs()
        ? ''
        : 'transparent';
  }
}

customElements.define('song-table', SongTable);
