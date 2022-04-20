// Copyright 2014 Daniel Erat.
// All rights reserved.

import {
  $,
  createElement,
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
    overflow-y: auto;
  }

  table {
    border-collapse: collapse;
    line-height: 1.2;
    padding-right: 0;
    table-layout: fixed;
    width: 100%;
  }

  th {
    background-color: var(--header-color);
    cursor: default;
    padding: 2px 10px 0 10px;
    position: sticky;
    text-align: left;
    top: 0;
    user-select: none;
    z-index: 1;
  }

  /* Gross hack from https://stackoverflow.com/a/57170489/6882947 to keep
   * borders from scrolling along with table contents. */
  th:after, th:before {
    content: '';
    left: 0;
    position: absolute;
    width: 100%;
  }
  th:before {
    border-top: solid 1px var(--border-color);
    top: 0;
  }
  th:after {
    border-bottom: solid 1px var(--border-color);
    bottom: 0;
  }
  table.scrolled th:after {
    box-shadow: 0 0 3px black;
    clip-path: inset(0 -3px -3px -3px);
  }

  tr {
    background-color: var(--bg-color);
    scroll-margin-bottom: 22px;
    scroll-margin-top: 42px;
  }
  tr.active {
    background-color: var(--accent-color);
    color: var(--accent-text-color);
  }
  tr.menu, tr.dragged {
    background-color: var(--bg-active-color);
  }
  tr.active.menu, tr.active.dragged {
    background-color: var(--accent-active-color);
  }

  td {
    overflow: hidden;
    padding-left: 10px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  td a {
    color: var(--text-color);
    text-decoration: none;
    cursor: pointer;
  }
  td a:hover {
    color: var(--link-color);
    text-decoration: underline;
  }
  tr.active td a {
    color: var(--accent-text-color);
  }

  td.checkbox,
  th.checkbox {
    width: 4px;
  }
  .checkbox {
    display: none;
  }
  :host([use-checkboxes]) .checkbox {
    display: table-cell;
  }
  input[type='checkbox'] {
    margin: 2px 0 0 0;
  }
  input[type='checkbox'][class~='transparent'] {
    opacity: 0.3;
  }


  td.time {
    padding-left: 6px;
  }
  td.time,
  th.time {
    width: 3em;
    text-align: right;
    padding-right: 10px;
    text-overflow: clip;
  }

  #drag-target {
    background-color: var(--text-color);
    display: none;
    height: 2px;
    position: absolute;
    z-index: 1;
  }

  #drag-target.visible {
    display: block;
  }
</style>

<table>
  <thead>
    <tr>
      <th class="checkbox"><input type="checkbox" class="small" /></th>
      <th class="artist">Artist</th>
      <th class="title">Title</th>
      <th class="album">Album</th>
      <th class="time">Time</th>
    </tr>
  </thead>
  <tbody></tbody>
</table>

<div id="drag-target"></div>
`);

const rowTemplate = createTemplate(`
<tr draggable="true">
  <td class="checkbox"><input type="checkbox" class="small" /></td>
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
//
// When a song is right-clicked, a 'menu' event is emitted with |detail.songId|,
// |detail.index|, and |detail.orig| (containing the original PointerEvent)
// properties. The receiver should call detail.orig.preventDefault() if it
// displays its own menu.
//
// When a song is dragged to a new position, a 'reorder' event is emitted with
// |detail.fromIndex| and |detail.toIndex| properties. The song is automatically
// reordered within song-table.
customElements.define(
  'song-table',
  class extends HTMLElement {
    constructor() {
      super();

      this.lastClickedCheckboxIndex_ = -1; // 0 is header
      this.numCheckedSongs_ = 0;

      this.shadow_ = createShadow(this, template);
      this.table_ = this.shadow_.querySelector('table');

      this.dragImage_ = createElement('img');
      this.dragTarget_ = $('drag-target', this.shadow_);
      this.dragFromIndex_ = -1;
      this.dragToIndex_ = -1;
      this.dragListRect_ = null;

      this.headingCheckbox_ = this.shadow_.querySelector(
        'input[type="checkbox"]'
      );
      this.headingCheckbox_.addEventListener('click', (e) => {
        this.onCheckboxClick_(this.headingCheckbox_, e.shiftKey);
      });

      // Listen for drag-and-drop events on document.body instead of |table_| so
      // we can still reorder songs if the user releases the button outside of
      // the table. Only the song table that initiated the drag will process the
      // events.
      document.body.addEventListener('dragenter', (e) => {
        if (!this.inDrag_) return;
        e.preventDefault(); // needed to allow dropping
        e.stopPropagation();
        e.dataTransfer.dropEffect = 'move';
      });
      document.body.addEventListener('dragover', (e) => {
        if (!this.inDrag_) return;
        e.preventDefault(); // needed to allow dropping
        e.stopPropagation();
        const idx = this.getDragEventIndex_(e);
        if (idx != this.dragToIndex_) {
          this.dragToIndex_ = idx;
          this.moveDragTarget_();
        }
      });
      // Listen for 'dragend' since 'drop' doesn't fire when the drag was
      // canceled. Chrome 98 also seems to always misreport the drop effect as
      // 'none' in the 'drop' event, making it impossible to tell if the drag
      // was canceled: https://stackoverflow.com/a/43892407
      // Firefox 95 sets the drop effect properly.
      document.body.addEventListener('dragend', (e) => {
        if (!this.inDrag_) return;
        e.preventDefault();
        e.stopPropagation();

        const from = this.dragFromIndex_;
        const to = this.dragToIndex_;
        this.songRows_[from].classList.remove('dragged');
        this.hideDragTarget_();
        this.dragFromIndex_ = this.dragToIndex_ = -1;
        this.dragListRect_ = null;

        // The browser sets the drop effect to 'none' if the drag was aborted
        // e.g. with the Escape key or by dropping outside the window.
        if (e.dataTransfer.dropEffect === 'none' || to === from) return;

        const row = this.songRows_[from];
        const tbody = row.parentNode;
        if (to < from) {
          tbody.insertBefore(row, this.songRows_[to]);
        } else if (to < this.numSongs - 1) {
          tbody.insertBefore(row, this.songRows_[to + 1]);
        } else {
          tbody.appendChild(row);
        }
        this.emitEvent_('reorder', { fromIndex: from, toIndex: to });
      });

      // Show/hide the header shadow when scrolling.
      this.addEventListener('scroll', (e) => {
        if (this.scrollTop) this.table_.classList.add('scrolled');
        else this.table_.classList.remove('scrolled');
      });
    }

    get inDrag_() {
      return this.dragFromIndex_ !== -1;
    }

    get useCheckboxes_() {
      return this.hasAttribute('use-checkboxes');
    }

    get songRows_() {
      return [].slice.call(this.table_.rows, 1); // exclude header
    }

    get songs() {
      return this.songRows_.map((r) => r.song); // shallow copy
    }
    get numSongs() {
      return this.songRows_.length;
    }
    getSong(index) {
      return this.songRows_[index].song;
    }

    get checkedSongs() {
      return !this.useCheckboxes_
        ? []
        : this.songRows_
            .filter((r) => r.cells[0].children[0].checked)
            .map((r) => r.song);
    }

    // Marks the row at |index| as being active (or not).
    // The row receives a strong highlight.
    setRowActive(index, active) {
      this.setRowClass_(index, 'active', active);
    }

    // Marks the row at |index| as having its context menu shown (or not).
    // The row receives a faint highlight.
    setRowMenuShown(index, menuShown) {
      this.setRowClass_(index, 'menu', menuShown);
    }

    // Helper method that adds or removes |cls| from the row at |index|.
    setRowClass_(index, cls, add) {
      if (index < 0 || index >= this.numSongs) return;

      const row = this.songRows_[index];
      if (add) row.classList.add(cls);
      else row.classList.remove(cls);
    }

    // Scrolls the table so that the row at |index| is in view.
    scrollToRow(index) {
      if (index < 0 || index >= this.numSongs) return;
      this.songRows_[index].scrollIntoView({
        behavior: 'smooth',
        block: 'nearest',
      });
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
      const oldSongs = this.songs;

      // Walk forward from the beginning and backward from the end to look for
      // common runs of songs.
      const minLength = Math.min(oldSongs.length, newSongs.length);
      let startMatchLength = 0;
      let endMatchLength = 0;
      for (
        let i = 0;
        i < minLength && oldSongs[i].songId === newSongs[i].songId;
        i++
      ) {
        startMatchLength++;
      }
      for (
        let i = 0;
        i < minLength - startMatchLength &&
        oldSongs[oldSongs.length - i - 1].songId ===
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
        this.updateSongRow_(index, newSongs[index], true /* deferTitles */);
      }
      // Show or hide title attributes. Do this after updating all the rows so
      // we trigger a single reflow instead of |numNewMiddleSongs|.
      // TODO: Also update the attributes whenever the table is resized.
      for (let i = 0; i < numNewMiddleSongs; i++) {
        const index = startMatchLength + i;
        this.updateSongRowTitleAttributes_(index);
      }

      if (this.useCheckboxes_) this.setAllCheckboxes(false);
    }

    // Emits a |name| CustomEvent with its 'detail' property set to |detail|.
    emitEvent_(name, detail) {
      this.dispatchEvent(new CustomEvent(name, { detail }));
    }

    // Inserts and initializes a new song row at |index| (ignoring the header
    // row).
    insertSongRow_(index) {
      // Cloning the template produces a DocumentFragment, so attach it to the
      // DOM so we can get the actual <tr> to use in event listeners.
      const tbody = this.table_.querySelector('tbody');
      tbody.insertBefore(
        rowTemplate.content.cloneNode(true),
        tbody.children ? tbody.children[index] : null
      );
      const row = tbody.children[index];

      row
        .querySelector('input[type="checkbox"]')
        .addEventListener('click', (e) =>
          this.onCheckboxClick_(e.target, e.shiftKey)
        );
      row.querySelector('.artist a').addEventListener('click', () => {
        this.emitEvent_('field', { artist: row.song.artist });
      });
      row.querySelector('.album a').addEventListener('click', () => {
        this.emitEvent_('field', {
          albumId: row.song.albumId,
          album: row.song.album,
        });
      });
      row.addEventListener('contextmenu', (e) => {
        this.emitEvent_('menu', {
          songId: row.song.songId,
          index: this.songRows_.indexOf(row), // don't use orig (stale) index
          orig: e, // PointerEvent
        });
      });
      row.addEventListener('dragstart', (e) => {
        e.dataTransfer.effectAllowed = 'move';
        e.dataTransfer.setDragImage(this.dragImage_, 0, 0);
        row.classList.add('dragged');
        this.dragFromIndex_ = this.dragToIndex_ = this.songRows_.indexOf(row);
        this.dragListRect_ = this.table_
          .querySelector('tbody')
          .getBoundingClientRect();
        this.moveDragTarget_();
        this.showDragTarget_();
      });
    }

    // Deletes the row at |index| (ignoring the header row).
    deleteSongRow_(index) {
      this.table_.deleteRow(index + 1); // skip header
    }

    // Updates the row at |index| to display data about |song| and attaches
    // |song| to the row. Also updates the row's title attributes (which can
    // trigger a reflow) unless |deferTitles| is true.
    updateSongRow_(index, song, deferTitles) {
      const row = this.songRows_[index];
      row.classList.remove('active');

      // HTML5 dataset properties can only hold strings, so we attach the song
      // directly to the element instead. (Serializing it to JSON would also be
      // an option but seems like it may have performance concerns.)
      row.song = song;

      const update = (cell, text, textInChild) =>
        ((textInChild ? cell.firstChild : cell).innerText = text);
      update(row.cells[1], song.artist, true);
      update(row.cells[2], song.title, false);
      update(row.cells[3], song.album, true);
      update(row.cells[4], formatTime(parseFloat(song.length)), false);

      if (!deferTitles) this.updateSongRowTitleAttributes_(index);
    }

    // Adds or removes 'title' attributes from each of the specified row's cells
    // depending on whether its text is elided or not.
    updateSongRowTitleAttributes_(index) {
      const update = (cell, textInChild) =>
        updateTitleAttributeForTruncation(
          cell,
          (textInChild ? cell.firstChild : cell).innerText
        );
      const row = this.songRows_[index];
      update(row.cells[1], true);
      update(row.cells[2], false);
      update(row.cells[3], true);
      // The time shouldn't overflow.
    }

    // Handles one of the checkboxes being clicked.
    onCheckboxClick_(checkbox, shiftHeld) {
      const getCheckbox = (i) => this.table_.rows[i].cells[0].children[0];

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

      this.emitEvent_('check', { count: this.numCheckedSongs_ });
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

    getDragEventIndex_(e) {
      const ey = e.clientY;
      const list = this.dragListRect_;
      const nsongs = this.songRows_.length;

      if (ey <= list.top) return 0;
      if (ey >= list.bottom) return nsongs - 1;

      // Computing the destination row is a bit tricky:
      //
      //  ...   ---
      // ------- 2
      //  Row 2 ---
      // ------- 3
      //  Row 3 ---
      // -------
      //  Row 4  4
      // -------
      //  Row 5 ---
      // ------- 5
      //  Row 6 ---
      // ------- 6
      //  ...   ---
      //
      // Suppose that row 4 is being dragged. The destination row should be 4 as
      // long as the cursor is between 3.5 and 5.5. The dest is 3 between 2.5
      // and 3.5, and the dest is 5 between 5.5 and 6.5.

      const pos = Math.round(((ey - list.top) / list.height) * nsongs);
      return pos - (pos > this.dragFromIndex_ ? 1 : 0);
    }

    // Shows |dragTarget_| and updates its size and position.
    showDragTarget_() {
      this.dragTarget_.classList.add('visible');
      this.dragTarget_.style.width = this.dragListRect_.width + 'px';
      this.moveDragTarget_();
    }

    // Hides |dragTarget_|.
    hideDragTarget_() {
      this.dragTarget_.classList.remove('visible');
    }

    // Updates |dragTarget_|'s Y position for |dragToIndex_|.
    moveDragTarget_() {
      const idx =
        this.dragToIndex_ + (this.dragToIndex_ > this.dragFromIndex_ ? 1 : 0);
      const y =
        this.dragListRect_.top +
        idx * (this.dragListRect_.height / this.songRows_.length) -
        2;
      this.dragTarget_.style.top = `${y}px`;
    }
  }
);
