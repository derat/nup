// Copyright 2014 Daniel Erat.
// All rights reserved.

import {
  $,
  createShadow,
  createStyle,
  createTemplate,
  formatTime,
  updateTitleAttributeForTruncation,
} from './common.js';

const template = createTemplate(`
<style>
  table {
    border-collapse: collapse;
    padding-right: 10px;
    table-layout: fixed;
    width: 100%;
  }
  th {
    text-align: left;
    padding-right: 10px;
    border-top: solid 1px #ccc;
    border-bottom: solid 1px #ccc;
    padding-left: 8px;
    background-color: #f5f5f5;
    -webkit-user-select: none;
    cursor: default;
  }
  td {
    padding-left: 8px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  td a {
    color: black;
    text-decoration: none;
    cursor: pointer;
  }
  td a:hover {
    color: #666;
    text-decoration: underline;
  }
  td.checkbox,
  th.checkbox {
    width: 1px;
  }
  td.time,
  th.artist {
    padding-left: 6px;
  }
  td.time,
  th.time {
    width: 3em;
    text-align: right;
    padding-right: 6px;
    text-overflow: clip;
  }
  tr.highlight {
    background-color: #bde;
  }
  input[type='checkbox'] {
    height: 12px;
    margin: 0;
    vertical-align: middle;
    width: 12px;
  }
  input[type='checkbox'][class~='transparent'] {
    opacity: 0.3;
  }
</style>
<table>
  <thead>
    <tr>
      <th class="checkbox">
        <input type="checkbox" />
      </th>
      <th class="artist">Artist</th>
      <th class="title">Title</th>
      <th class="album">Album</th>
      <th class="time">Time</th>
    </tr>
  </thead>
  <tbody>
  </tbody>
</table>
`);

const rowTemplate = createTemplate(`
<tr>
  <td class="checkbox">
    <input type="checkbox" />
  </td>
  <td class="artist"><a></a></td>
  <td class="title"></td>
  <td class="album"><a></a></td>
  <td class="time"></td>
</tr>
`);

customElements.define(
  'song-table',
  class extends HTMLElement {
    constructor() {
      super();

      this.useCheckboxes_ = this.hasAttribute('use-checkboxes');
      this.lastClickedCheckboxIndex_ = -1; // 0 is header
      this.numCheckedSongs_ = 0;

      this.style.display = 'block';
      this.shadow_ = createShadow(this, template);
      this.table_ = this.shadow_.querySelector('table');

      this.headingCheckbox_ = this.shadow_.querySelector(
        'input[type="checkbox"]',
      );
      this.headingCheckbox_.addEventListener('click', e =>
        this.handleCheckboxClicked_(this.headingCheckbox_, e),
      );
      if (!this.useCheckboxes_) {
        this.shadow_.appendChild(createStyle('.checkbox { display: none }'));
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

    get songRows_() {
      return [].slice.call(this.table_.rows, 1); // exclude header
    }

    get numSongs() {
      return this.songRows_.length;
    }

    get checkedSongs() {
      return !this.useCheckboxes_
        ? []
        : this.songRows_
            .filter(r => r.cells[0].children[0].checked)
            .map(r => r.song);
    }

    highlightRow(index, highlight) {
      if (index < 0 || index >= this.numSongs) return;

      const row = this.songRows_[index];
      if (highlight) row.classList.add('highlight');
      else row.classList.remove('highlight');
    }

    setAllCheckboxes(checked) {
      if (!this.useCheckboxes_) return;

      this.headingCheckbox_.checked = checked ? 'checked' : null;
      this.handleCheckboxClicked_(this.headingCheckbox_);
    }

    // Updates the table to contain |newSongs| while trying to be smart about
    // not doing any more work than necessary.
    updateSongs(newSongs) {
      const oldSongs = this.songRows_.map(r => r.song);

      // Walk forward from the beginning and backward from the end to look for
      // common runs of songs.
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
        this.insertSongRow_(startMatchLength);
      }
      for (let i = numOldMiddleSongs; i > numNewMiddleSongs; i--) {
        this.deleteSongRow_(startMatchLength);
      }

      // Update all of the rows in the middle to contain the correct data.
      for (let i = 0; i < numNewMiddleSongs; i++) {
        const index = startMatchLength + i;
        this.updateSongRow_(index, newSongs[index]);
      }

      // Clear all of the checkboxes.
      if (this.useCheckboxes_) {
        this.songRows_.forEach(r => (r.cells[0].children[0].checked = null));
        this.lastClickedCheckboxIndex_ = -1;
        this.numCheckedSongs_ = 0;
        this.updateHeadingCheckbox_();
        if (this.checkedSongsChangedCallback_) {
          this.checkedSongsChangedCallback_(0);
        }
      }
    }

    // Insert and initialize a new song row at |index|.
    insertSongRow_(index) {
      const row = rowTemplate.content.cloneNode(true);
      const check = row.querySelector('input[type="checkbox"]');
      check.addEventListener('click', e =>
        this.handleCheckboxClicked_(check, e),
      );
      row
        .querySelector('.artist a')
        .addEventListener(
          'click',
          () =>
            this.artistClickedCallback_ &&
            this.artistClickedCallback_(row.song.artist),
        );
      row
        .querySelector('.album a')
        .addEventListener(
          'click',
          () =>
            this.albumClickedCallback_ &&
            this.albumClickedCallback_(row.song.album),
        );

      const tbody = this.table_.querySelector('tbody');
      tbody.insertBefore(row, tbody.children ? tbody.children[index] : null);
    }

    // Deletes the row at |index| (ignoring the header row).
    deleteSongRow_(index) {
      this.table_.deleteRow(index + 1); // skip header
    }

    // Updates |row| to display data about |song|.
    updateSongRow_(index, song) {
      const row = this.songRows_[index];
      row.song = song;
      row.classList.remove('highlight');

      const update = (cell, text, updateChild) => {
        (updateChild ? cell.children[0] : cell).innerText = text;
        updateTitleAttributeForTruncation(cell, text);
      };
      update(row.cells[1], song.artist, true);
      update(row.cells[2], song.title, false);
      update(row.cells[3], song.album, true);
      update(row.cells[4], formatTime(parseFloat(song.length)), false);
    }

    // Handles one of the checkboxes being clicked.
    handleCheckboxClicked_(checkbox, e) {
      const getCheckbox = i => this.table_.rows[i].cells[0].children[0];

      let index = -1;
      for (let i = 0; i < this.table_.rows.length; i++) {
        if (checkbox == getCheckbox(i)) {
          index = i;
          break;
        }
      }
      const checked = checkbox.checked;

      if (index == 0) {
        for (let i = 1; i < this.table_.rows.length; i++) {
          getCheckbox(i).checked = checked ? 'checked' : null;
        }
        this.numCheckedSongs_ = checked ? this.numSongs : 0;
      } else {
        this.numCheckedSongs_ += checked ? 1 : -1;

        if (e && e.shiftKey) {
          if (
            this.lastClickedCheckboxIndex_ > 0 &&
            this.lastClickedCheckboxIndex_ < this.table_.rows.length &&
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

    // Updates |headingCheckbox_|'s visual state for the current number of
    // checked songs.
    updateHeadingCheckbox_() {
      this.headingCheckbox_.checked = this.numCheckedSongs_ ? 'checked' : null;
      if (this.numCheckedSongs_ && this.numCheckedSongs_ != this.numSongs) {
        this.headingCheckbox_.classList.add('transparent');
      } else {
        this.headingCheckbox_.classList.remove('transparent');
      }
    }
  },
);
