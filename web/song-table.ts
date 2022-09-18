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

  td.artist,
  td.album {
    cursor: pointer;
  }
  td.artist:hover,
  td.album:hover {
    color: var(--link-color);
    text-decoration: underline;
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
    pointer-events: none;
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
      <th class="checkbox">
        <input type="checkbox" class="small" title="Toggle all" />
      </th>
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
  <td class="artist"></td>
  <td class="title"></td>
  <td class="album"></td>
  <td class="time"></td>
</tr>
`);

const RESIZE_TIMEOUT_MS = 1000; // delay after resize to update titles

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
  #lastClickedCheckboxIndex = -1; // 0 is first song row
  #numCheckedSongs = 0;
  #shadow = createShadow(this, template);
  #table = this.#shadow.querySelector('table') as HTMLTableElement;
  #tbody = this.#table.querySelector('tbody') as HTMLTableSectionElement;
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

    // Listen for click events on checkboxes or table cells.
    this.#table.addEventListener('click', (e: MouseEvent) => {
      const el = e.target as HTMLElement;
      if (el.tagName === 'INPUT') {
        this.#onCheckboxClick(el as HTMLInputElement, e.shiftKey);
      } else if (el.tagName === 'TD') {
        const row = el.closest('tr') as HTMLTableRowElement;
        if (el.classList.contains('artist')) {
          this.#emitEvent('field', { artist: this.#getRowSong(row).artist });
        } else if (el.classList.contains('album')) {
          const song = this.#getRowSong(row);
          this.#emitEvent('field', {
            albumId: song.albumId,
            album: song.album,
          });
        }
      }
    });

    // Listen for right-clicks that should display the context menu.
    this.#tbody.addEventListener('contextmenu', (e: MouseEvent) => {
      const el = e.target as HTMLElement;
      const row = el.closest('tr') as HTMLTableRowElement;
      this.#emitEvent('menu', {
        songId: this.#getRowSong(row).songId,
        index: this.#songRowsArray.indexOf(row),
        orig: e,
      });
    });

    // Listen for drag starts originating from table rows.
    this.#tbody.addEventListener('dragstart', (e: DragEvent) => {
      const el = e.target as HTMLElement;
      const row = el.closest('tr') as HTMLTableRowElement;
      e.dataTransfer!.effectAllowed = 'move';
      e.dataTransfer!.setDragImage(this.#dragImage, 0, 0);
      row.classList.add('dragged');
      this.#dragFromIndex = this.#dragToIndex =
        this.#songRowsArray.indexOf(row);
      this.#dragListRect = this.#tbody.getBoundingClientRect();
      this.#moveDragTarget();
      this.#showDragTarget();
    });

    // When the table is resized, update all of the rows' title attributes
    // after a short delay.
    this.#resizeObserver = new ResizeObserver((entries) => {
      if (this.#resizeTimeoutId) window.clearTimeout(this.#resizeTimeoutId);
      this.#resizeTimeoutId = window.setTimeout(() => {
        this.#resizeTimeoutId = null;
        for (let i = 0; i < this.#songRows.length; i++) {
          this.#updateSongRowTitleAttributes(i);
        }
      }, RESIZE_TIMEOUT_MS);
    });
    this.#resizeObserver.observe(this.#table);

    this.addEventListener('scroll', (e) => {
      // Show/hide the header shadow when scrolling.
      if (this.scrollTop) this.#table.classList.add('scrolled');
      else this.#table.classList.remove('scrolled');

      // Update the rect used to determine the target position when dragging.
      if (this.#inDrag) {
        this.#dragListRect = this.#tbody.getBoundingClientRect();
      }
    });
  }

  connectedCallback() {
    // Listen for drag-and-drop events on document.body instead of #table so we
    // can still reorder songs if the user releases the button outside of the
    // table. Only the song table that initiated the drag will process the
    // events.
    document.body.addEventListener('dragenter', this.#onDragEnter);
    document.body.addEventListener('dragover', this.#onDragOver);

    // Listen for 'dragend' since 'drop' doesn't fire when the drag was
    // canceled. Chrome also seems to always report the drop effect as 'none' in
    // the 'drop' event, making it impossible to tell if the drag was canceled:
    // https://stackoverflow.com/a/43892407
    // https://crub.com/509752
    document.body.addEventListener('dragend', this.#onDragEnd);

    // TODO: Chrome 105.0.5195.134 flashes the can't-drop cursor and doesn't
    // allow dropping whenever the pointer moves into a new element. If the
    // element explicitly listens for and cancels the 'dragenter' event then
    // this doesn't happen, but document.body doesn't receive the event for some
    // (shadow-related?) reason. Listening for 'dragenter' in the table seems to
    // at least prevent this from happening as the pointer moves between rows:
    // https://github.com/derat/nup/issues/45
    this.#table.addEventListener('dragenter', this.#onDragEnter);
  }

  disconnectedCallback() {
    document.body.removeEventListener('dragenter', this.#onDragEnter);
    document.body.removeEventListener('dragover', this.#onDragOver);
    document.body.removeEventListener('dragend', this.#onDragEnd);
    this.#table.removeEventListener('dragenter', this.#onDragEnter);

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
      this.#tbody.insertBefore(
        rowTemplate.content.cloneNode(true),
        this.#tbody.rows?.item(startMatchLength) ?? null
      );
    }
    for (let i = numOldMiddleSongs; i > numNewMiddleSongs; i--) {
      this.#tbody.deleteRow(startMatchLength);
    }

    // Update all of the rows in the middle to contain the correct data.
    for (let i = 0; i < numNewMiddleSongs; i++) {
      const index = startMatchLength + i;
      this.#updateSongRow(index, newSongs[index]);
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

  // Updates the row at |index| to display data about |song| and attaches
  // |song| to the row.
  //
  // #updateSongRowTitleAttributes() should be called afterward to update the
  // row's title attributes.
  #updateSongRow(index: number, song: Song) {
    const row = this.#songRows[index] as HTMLTableRowElement;
    this.#rowSongs.set(row, song);
    row.classList.remove('active');

    (row.querySelector('.artist') as HTMLElement).innerText = song.artist;
    (row.querySelector('.title') as HTMLElement).innerText = song.title;
    (row.querySelector('.album') as HTMLElement).innerText = song.album;
    (row.querySelector('.time') as HTMLElement).innerText = formatDuration(
      song.length
    );
  }

  // Adds or removes 'title' attributes from each of the specified row's cells
  // depending on whether its text is elided or not. This can trigger a reflow.
  #updateSongRowTitleAttributes(index: number) {
    const row = this.#songRows[index] as HTMLTableRowElement;
    const update = (cell: HTMLElement) =>
      updateTitleAttributeForTruncation(cell, cell.innerText);
    update(row.querySelector('.artist') as HTMLElement);
    update(row.querySelector('.title') as HTMLElement);
    update(row.querySelector('.album') as HTMLElement);
    // The time shouldn't overflow.
  }

  // Handles one of the checkboxes being clicked.
  #onCheckboxClick(checkbox: HTMLInputElement, shiftHeld: boolean) {
    const checkboxes = this.#tbody.querySelectorAll("input[type='checkbox']");
    const checked = checkbox.checked;

    if (checkbox === this.#headingCheckbox) {
      checkboxes.forEach((e) => ((e as HTMLInputElement).checked = checked));
      this.#numCheckedSongs = checked ? this.numSongs : 0;
      this.#lastClickedCheckboxIndex = -1;
    } else {
      const index = [...checkboxes].findIndex((e) => e === checkbox);
      if (index < 0) throw new Error("Didn't find checkbox row");
      this.#numCheckedSongs += checked ? 1 : -1;

      if (
        shiftHeld &&
        this.#lastClickedCheckboxIndex >= 0 &&
        this.#lastClickedCheckboxIndex < checkboxes.length &&
        this.#lastClickedCheckboxIndex !== index
      ) {
        const start = Math.min(index, this.#lastClickedCheckboxIndex);
        const end = Math.max(index, this.#lastClickedCheckboxIndex);
        for (let i = start; i <= end; i++) {
          const cb = checkboxes.item(i) as HTMLInputElement;
          if (cb.checked !== checked) {
            cb.checked = checked;
            this.#numCheckedSongs += checked ? 1 : -1;
          }
        }
      }
      this.#lastClickedCheckboxIndex = index;
    }

    this.#updateHeadingCheckbox();
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
    if (to < from) {
      this.#tbody.insertBefore(row, this.#songRows[to]);
    } else if (to < this.numSongs - 1) {
      this.#tbody.insertBefore(row, this.#songRows[to + 1]);
    } else {
      this.#tbody.appendChild(row);
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
