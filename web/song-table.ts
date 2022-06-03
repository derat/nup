// Copyright 2014 Daniel Erat.
// All rights reserved.

import {
  $,
  commonStyles,
  createElement,
  createShadow,
  createTemplate,
  formatDuration,
  updateTitleAttributeForTruncation,
} from './common.js';

const template = createTemplate(`
<style>
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
  th:after,
  th:before {
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
  tr.menu,
  tr.dragged {
    background-color: var(--bg-active-color);
  }
  tr.active.menu,
  tr.active.dragged {
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
export class SongTable extends HTMLElement {
  static #RESIZE_TIMEOUT_MS = 1000; // delay after resize to update titles

  #lastClickedCheckboxIndex = -1; // 0 is header
  #numCheckedSongs = 0;
  #shadow = createShadow(this, template);
  #table = this.#shadow.querySelector('table') as HTMLTableElement;
  #rowSongs: WeakMap<HTMLTableRowElement, Song> = new WeakMap();
  #resizeTimeoutId: number | null = null;
  #resizeObserver: ResizeObserver;
  #dragImage = createElement('img') as HTMLImageElement;
  #dragTarget = $('drag-target', this.#shadow);
  #dragFromIndex = -1;
  #dragToIndex = -1;
  #dragListRect: DOMRect | null = null;
  #headingCheckbox = this.#shadow.querySelector(
    'input[type="checkbox"]'
  ) as HTMLInputElement;

  constructor() {
    super();

    this.#shadow.adoptedStyleSheets = [commonStyles];

    // When the table is resized, update all of the rows' title attributes
    // after a short delay.
    this.#resizeObserver = new ResizeObserver((entries) => {
      if (this.#resizeTimeoutId) window.clearTimeout(this.#resizeTimeoutId);
      this.#resizeTimeoutId = window.setTimeout(() => {
        this.#resizeTimeoutId = null;
        for (let i = 0; i < this.#songRows.length; i++) {
          this.#updateSongRowTitleAttributes(i);
        }
      }, SongTable.#RESIZE_TIMEOUT_MS);
    });
    this.#resizeObserver.observe(this.#table);

    this.#headingCheckbox.addEventListener('click', (e) => {
      this.#onCheckboxClick(this.#headingCheckbox, e.shiftKey);
    });

    // Show/hide the header shadow when scrolling.
    this.addEventListener('scroll', (e) => {
      if (this.scrollTop) this.#table.classList.add('scrolled');
      else this.#table.classList.remove('scrolled');
    });
  }

  connectedCallback() {
    // Listen for drag-and-drop events on document.body instead of |#table| so
    // we can still reorder songs if the user releases the button outside of
    // the table. Only the song table that initiated the drag will process the
    // events.
    document.body.addEventListener('dragenter', this.#onDragEnter);
    document.body.addEventListener('dragover', this.#onDragOver);

    // Listen for 'dragend' since 'drop' doesn't fire when the drag was
    // canceled. Chrome 98 also seems to always misreport the drop effect as
    // 'none' in the 'drop' event, making it impossible to tell if the drag
    // was canceled: https://stackoverflow.com/a/43892407
    // Firefox 95 sets the drop effect properly.
    document.body.addEventListener('dragend', this.#onDragEnd);
  }

  disconnectedCallback() {
    document.body.removeEventListener('dragenter', this.#onDragEnter);
    document.body.removeEventListener('dragover', this.#onDragOver);
    document.body.removeEventListener('dragend', this.#onDragEnd);

    if (this.#resizeTimeoutId !== null) {
      window.clearTimeout(this.#resizeTimeoutId);
      this.#resizeTimeoutId = null;
    }
  }

  get #inDrag() {
    return this.#dragFromIndex !== -1;
  }

  get #useCheckboxes() {
    return this.hasAttribute('use-checkboxes');
  }

  // |#songRows| is efficient, but it's regrettably an HTMLCollection.
  get #songRows(): HTMLCollection {
    return this.#table.tBodies[0].rows;
  }
  // |#songRowsArray| is convenient (map, indexOf, etc.) but slow.
  get #songRowsArray(): HTMLTableRowElement[] {
    return [...this.#songRows] as HTMLTableRowElement[];
  }

  get songs(): Song[] {
    return this.#songRowsArray.map((r) => this.#rowSongs.get(r)!); // shallow copy
  }
  get numSongs() {
    return this.#songRows.length;
  }
  getSong(index: number): Song {
    return this.#getRowSong(this.#songRows[index] as HTMLTableRowElement);
  }

  #getRowSong(row: HTMLTableRowElement): Song {
    const song = this.#rowSongs.get(row);
    if (!song) throw new Error('No song for row');
    return song;
  }

  get checkedSongs() {
    return !this.#useCheckboxes
      ? []
      : this.#songRowsArray
          .filter((r) => (r.cells[0].children[0] as HTMLInputElement).checked)
          .map((r) => this.#getRowSong(r));
  }

  // Marks the row at |index| as being active (or not).
  // The row receives a strong highlight.
  setRowActive(index: number, active: boolean) {
    this.#setRowClass(index, 'active', active);
  }

  // Marks the row at |index| as having its context menu shown (or not).
  // The row receives a faint highlight.
  setRowMenuShown(index: number, menuShown: boolean) {
    this.#setRowClass(index, 'menu', menuShown);
  }

  // Helper method that adds or removes |cls| from the row at |index|.
  #setRowClass(index: number, cls: string, add: boolean) {
    if (index < 0 || index >= this.numSongs) return;

    const row = this.#songRows[index];
    if (add) row.classList.add(cls);
    else row.classList.remove(cls);
  }

  // Scrolls the table so that the row at |index| is in view.
  scrollToRow(index: number) {
    if (index < 0 || index >= this.numSongs) return;
    this.#songRows[index].scrollIntoView({
      behavior: 'smooth',
      block: 'nearest',
    });
  }

  // Sets all checkboxes to |checked|.
  setAllCheckboxes(checked: boolean) {
    if (!this.#useCheckboxes) return;

    this.#headingCheckbox.checked = checked;
    this.#onCheckboxClick(this.#headingCheckbox, false);
  }

  // Updates the table to contain |newSongs| while trying to be smart about
  // not doing any more work than necessary.
  setSongs(newSongs: Song[]) {
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
      this.#insertSongRow(startMatchLength);
    }
    for (let i = numOldMiddleSongs; i > numNewMiddleSongs; i--) {
      this.#deleteSongRow(startMatchLength);
    }

    // Update all of the rows in the middle to contain the correct data.
    for (let i = 0; i < numNewMiddleSongs; i++) {
      const index = startMatchLength + i;
      this.#updateSongRow(index, newSongs[index], true /* deferTitles */);
    }
    // Show or hide title attributes (which function as tooltips for elided
    // strings). Do this after updating all the rows so we trigger a single
    // reflow instead of |numNewMiddleSongs| reflows.
    for (let i = 0; i < numNewMiddleSongs; i++) {
      const index = startMatchLength + i;
      this.#updateSongRowTitleAttributes(index);
    }

    if (this.#useCheckboxes) this.setAllCheckboxes(false);
  }

  // Emits a |name| CustomEvent with its 'detail' property set to |detail|.
  #emitEvent(name: string, detail: any) {
    this.dispatchEvent(new CustomEvent(name, { detail }));
  }

  // Inserts and initializes a new song row at |index| (ignoring the header
  // row).
  #insertSongRow(index: number) {
    // Cloning the template produces a DocumentFragment, so attach it to the
    // DOM so we can get the actual <tr> to use in event listeners.
    const tbody = this.#table.querySelector('tbody')!;
    tbody.insertBefore(
      rowTemplate.content.cloneNode(true),
      tbody.children ? tbody.children[index] : null
    );
    const row = tbody.children[index] as HTMLTableRowElement;

    row
      .querySelector('input[type="checkbox"]')!
      .addEventListener('click', ((e: MouseEvent) =>
        this.#onCheckboxClick(
          e.target as HTMLInputElement,
          e.shiftKey
        )) as EventListenerOrEventListenerObject);
    row.querySelector('.artist a')!.addEventListener('click', () => {
      this.#emitEvent('field', { artist: this.#getRowSong(row).artist });
    });
    row.querySelector('.album a')!.addEventListener('click', () => {
      const song = this.#getRowSong(row);
      this.#emitEvent('field', { albumId: song.albumId, album: song.album });
    });
    row.addEventListener('contextmenu', (e: MouseEvent) => {
      this.#emitEvent('menu', {
        songId: this.#getRowSong(row).songId,
        index: this.#songRowsArray.indexOf(row), // don't use orig (stale) index
        orig: e,
      });
    });
    row.addEventListener('dragstart', (e: DragEvent) => {
      e.dataTransfer!.effectAllowed = 'move';
      e.dataTransfer!.setDragImage(this.#dragImage, 0, 0);
      row.classList.add('dragged');
      this.#dragFromIndex = this.#dragToIndex =
        this.#songRowsArray.indexOf(row);
      this.#dragListRect = this.#table
        .querySelector('tbody')!
        .getBoundingClientRect();
      this.#moveDragTarget();
      this.#showDragTarget();
    });
  }

  // Deletes the row at |index| (ignoring the header row).
  #deleteSongRow(index: number) {
    this.#table.deleteRow(index + 1); // skip header
  }

  // Updates the row at |index| to display data about |song| and attaches
  // |song| to the row. Also updates the row's title attributes (which can
  // trigger a reflow) unless |deferTitles| is true.
  #updateSongRow(index: number, song: Song, deferTitles: boolean) {
    const row = this.#songRows[index] as HTMLTableRowElement;
    row.classList.remove('active');

    this.#rowSongs.set(row, song);

    const update = (
      cell: HTMLTableCellElement,
      text: string,
      textInChild: boolean
    ) =>
      ((textInChild
        ? (cell.firstElementChild as HTMLElement)
        : cell
      ).innerText = text);
    update(row.cells[1], song.artist, true);
    update(row.cells[2], song.title, false);
    update(row.cells[3], song.album, true);
    update(row.cells[4], formatDuration(song.length), false);

    if (!deferTitles) this.#updateSongRowTitleAttributes(index);
  }

  // Adds or removes 'title' attributes from each of the specified row's cells
  // depending on whether its text is elided or not.
  #updateSongRowTitleAttributes(index: number) {
    const update = (cell: HTMLTableCellElement, textInChild: boolean) =>
      updateTitleAttributeForTruncation(
        cell,
        (textInChild ? (cell.firstElementChild as HTMLElement) : cell).innerText
      );
    const row = this.#songRows[index] as HTMLTableRowElement;
    update(row.cells[1], true);
    update(row.cells[2], false);
    update(row.cells[3], true);
    // The time shouldn't overflow.
  }

  // Handles one of the checkboxes being clicked.
  #onCheckboxClick(checkbox: HTMLInputElement, shiftHeld: boolean) {
    const getCheckbox = (i: number) =>
      this.#table.rows[i].cells[0].children[0] as HTMLInputElement;

    let index = -1;
    for (let i = 0; i < this.#table.rows.length; i++) {
      if (checkbox === getCheckbox(i)) {
        index = i;
        break;
      }
    }
    const checked = checkbox.checked;

    if (index === 0) {
      for (let i = 1; i < this.#table.rows.length; i++) {
        getCheckbox(i).checked = checked;
      }
      this.#numCheckedSongs = checked ? this.numSongs : 0;
    } else {
      this.#numCheckedSongs += checked ? 1 : -1;

      if (shiftHeld) {
        if (
          this.#lastClickedCheckboxIndex > 0 &&
          this.#lastClickedCheckboxIndex < this.#table.rows.length &&
          this.#lastClickedCheckboxIndex !== index
        ) {
          const start = Math.min(index, this.#lastClickedCheckboxIndex);
          const end = Math.max(index, this.#lastClickedCheckboxIndex);
          for (let i = start; i <= end; i++) {
            const c = getCheckbox(i);
            if (checked && !c.checked) {
              c.checked = true;
              this.#numCheckedSongs++;
            } else if (!checked && c.checked) {
              c.checked = false;
              this.#numCheckedSongs--;
            }
          }
        }
      }
    }

    this.#updateHeadingCheckbox();
    this.#lastClickedCheckboxIndex = index;

    this.#emitEvent('check', { count: this.#numCheckedSongs });
  }

  // Updates |#headingCheckbox|'s visual state for the current number of
  // checked songs.
  #updateHeadingCheckbox() {
    this.#headingCheckbox.checked = this.#numCheckedSongs > 0;
    if (this.#numCheckedSongs && this.#numCheckedSongs !== this.numSongs) {
      this.#headingCheckbox.classList.add('transparent');
    } else {
      this.#headingCheckbox.classList.remove('transparent');
    }
  }

  #onDragEnter = (e: DragEvent) => {
    if (!this.#inDrag) return;
    e.preventDefault(); // needed to allow dropping
    e.stopPropagation();
    e.dataTransfer!.dropEffect = 'move';
  };

  #onDragOver = (e: DragEvent) => {
    if (!this.#inDrag) return;
    e.preventDefault(); // needed to allow dropping
    e.stopPropagation();
    const idx = this.#getDragEventIndex(e);
    if (idx !== this.#dragToIndex) {
      this.#dragToIndex = idx;
      this.#moveDragTarget();
    }
  };

  #onDragEnd = (e: DragEvent) => {
    if (!this.#inDrag) return;
    e.preventDefault();
    e.stopPropagation();

    const from = this.#dragFromIndex;
    const to = this.#dragToIndex;
    this.#songRows[from].classList.remove('dragged');
    this.#hideDragTarget();
    this.#dragFromIndex = this.#dragToIndex = -1;
    this.#dragListRect = null;

    // The browser sets the drop effect to 'none' if the drag was aborted
    // e.g. with the Escape key or by dropping outside the window.
    if (e.dataTransfer!.dropEffect === 'none' || to === from) return;

    if (from < 0 || from >= this.#songRows.length) {
      throw new Error(
        `From index ${from} not in [0, ${this.#songRows.length})`
      );
    }
    const row = this.#songRows[from];
    const tbody = row.parentNode!;
    if (to < from) {
      tbody.insertBefore(row, this.#songRows[to]);
    } else if (to < this.numSongs - 1) {
      tbody.insertBefore(row, this.#songRows[to + 1]);
    } else {
      tbody.appendChild(row);
    }
    this.#emitEvent('reorder', { fromIndex: from, toIndex: to });
  };

  #getDragEventIndex(e: DragEvent) {
    if (!this.#dragListRect) throw new Error('Missing drag rect');
    const ey = e.clientY;
    const list = this.#dragListRect;
    const nsongs = this.#songRows.length;

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
    return pos - (pos > this.#dragFromIndex ? 1 : 0);
  }

  // Shows |#dragTarget| and updates its size and position.
  #showDragTarget() {
    if (!this.#dragListRect) throw new Error('Missing drag rect');
    this.#dragTarget.classList.add('visible');
    this.#dragTarget.style.width = this.#dragListRect.width + 'px';
    this.#moveDragTarget();
  }

  // Hides |#dragTarget|.
  #hideDragTarget() {
    this.#dragTarget.classList.remove('visible');
  }

  // Updates |#dragTarget|'s Y position for |#dragToIndex|.
  #moveDragTarget() {
    if (!this.#dragListRect) throw new Error('Missing drag rect');
    const idx =
      this.#dragToIndex + (this.#dragToIndex > this.#dragFromIndex ? 1 : 0);
    const y =
      this.#dragListRect.top +
      idx * (this.#dragListRect.height / this.#songRows.length) -
      2;
    this.#dragTarget.style.top = `${y}px`;
  }
}

customElements.define('song-table', SongTable);
