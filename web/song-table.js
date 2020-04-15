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
  @import 'common.css';
  :host {
    display: block;
  }
  table {
    border-collapse: collapse;
    padding-right: 10px;
    table-layout: fixed;
    width: 100%;
  }
  th {
    background-color: #f5f5f5;
    border-bottom: solid 1px #ddd;
    border-top: solid 1px #ddd;
    cursor: default;
    padding-left: 8px;
    padding-right: 10px;
    padding-top: 2px;
    text-align: left;
    user-select: none;
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
  tr.highlight td a {
    color: white;
  }

  td.checkbox,
  th.checkbox {
    width: 4px;
  }
  td.time {
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
    background-color: var(--accent-color);
    color: white;
  }
  input[type='checkbox'] {
    margin: 2px 0 0 0;
  }
  input[type='checkbox'][class~='transparent'] {
    opacity: 0.3;
  }
</style>
<table>
  <thead>
    <tr>
      <th class="checkbox">
        <input type="checkbox" class="small" />
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

const noCheckboxesTemplate = createTemplate(`
<style>
  .checkbox {
    display: none;
  }
</style>
`);

const rowTemplate = createTemplate(`
<tr>
  <td class="checkbox">
    <input type="checkbox" class="small" />
  </td>
  <td class="artist"><a></a></td>
  <td class="title"></td>
  <td class="album"><a></a></td>
  <td class="time"></td>
</tr>
`);

// <song-table> displays a list of songs.
//
// If the 'use-checkboxes' attribute is set, checkboxes will be displayed at the
// left side of each row, and a 'check' event with a |detail.count| property
// will be emitted whenever the number of checked songs changes.
//
// When a song's artist or album field is clicked, a 'field' event will be
// emitted with either a |detail.artist| or |detail.album| property.
customElements.define(
  'song-table',
  class extends HTMLElement {
    constructor() {
      super();

      this.useCheckboxes_ = this.hasAttribute('use-checkboxes');
      this.lastClickedCheckboxIndex_ = -1; // 0 is header
      this.numCheckedSongs_ = 0;

      this.shadow_ = createShadow(this, template);
      this.table_ = this.shadow_.querySelector('table');

      this.headingCheckbox_ = this.shadow_.querySelector(
        'input[type="checkbox"]',
      );
      this.headingCheckbox_.addEventListener('click', e => {
        this.onCheckboxClick_(this.headingCheckbox_, e.shiftKey);
      });
      if (!this.useCheckboxes_) {
        this.shadow_.appendChild(noCheckboxesTemplate.content.cloneNode(true));
      }
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

    // Sets highlighting for the song at index |index| to |highlight|.
    highlightRow(index, highlight) {
      if (index < 0 || index >= this.numSongs) return;

      const row = this.songRows_[index];
      if (highlight) row.classList.add('highlight');
      else row.classList.remove('highlight');
    }

    // Sets all checkboxes to |checked|.
    setAllCheckboxes(checked) {
      if (!this.useCheckboxes_) return;

      this.headingCheckbox_.checked = checked;
      this.onCheckboxClick_(this.headingCheckbox_, false);
    }

    // Updates the table to contain |newSongs| while trying to be smart about
    // not doing any more work than necessary.
    setSongs(newSongs) {
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

      if (this.useCheckboxes_) this.setAllCheckboxes(false);
    }

    // Emits a |name| CustomEvent with its 'detail' property set to |detail|.
    emitEvent_(name, detail) {
      this.dispatchEvent(new CustomEvent(name, {detail}));
    }

    // Inserts and initializes a new song row at |index| (ignoring the header
    // row).
    insertSongRow_(index) {
      // Cloning the template produces a DocumentFragment, so attach it to the
      // DOM so we can get the actual <tr> to use in event listeners.
      const tbody = this.table_.querySelector('tbody');
      tbody.insertBefore(
        rowTemplate.content.cloneNode(true),
        tbody.children ? tbody.children[index] : null,
      );
      const row = tbody.children[index];

      row
        .querySelector('input[type="checkbox"]')
        .addEventListener('click', e =>
          this.onCheckboxClick_(e.target, e.shiftKey),
        );
      row.querySelector('.artist a').addEventListener('click', () => {
        this.emitEvent_('field', {artist: row.song.artist});
      });
      row.querySelector('.album a').addEventListener('click', () => {
        this.emitEvent_('field', {album: row.song.album});
      });
    }

    // Deletes the row at |index| (ignoring the header row).
    deleteSongRow_(index) {
      this.table_.deleteRow(index + 1); // skip header
    }

    // Updates |row| to display data about |song|. Also attaches |song|.
    updateSongRow_(index, song) {
      const row = this.songRows_[index];
      row.classList.remove('highlight');

      // HTML5 dataset properties can only hold strings, so we attach the song
      // directly to the element instead. (Serializing it to JSON would also be
      // an option but seems like it may have performance concerns.)
      row.song = song;

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
    onCheckboxClick_(checkbox, shiftHeld) {
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

        if (shiftHeld) {
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

      this.emitEvent_('check', {count: this.numCheckedSongs_});
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
