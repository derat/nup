// Copyright 2010 Daniel Erat.
// All rights reserved.

import {
  $,
  clamp,
  commonStyles,
  createShadow,
  createTemplate,
  getCoverUrl,
  getDumpSongUrl,
  handleFetchError,
  setIcon,
  smallCoverSize,
  spinnerIcon,
  xIcon,
} from './common.js';
import { isDialogShown, showMessageDialog } from './dialog.js';
import { createMenu, isMenuShown } from './menu.js';
import { showSongInfoDialog } from './song-info-dialog.js';
import type { SongTable } from './song-table.js';
import type { TagSuggester } from './tag-suggester.js';

const template = createTemplate(`
<style>
  :host {
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  .heading {
    font-size: 15px;
    font-weight: bold;
    margin-bottom: var(--margin);
    user-select: none;
  }
  #search-form {
    display: block;
    margin: var(--margin);
    white-space: nowrap;
  }
  #search-form input[type='checkbox'] {
    margin: 4px 3px -3px 0;
  }
  #search-form .row {
    align-items: baseline;
    display: flex;
    margin-bottom: 6px;
  }
  #search-form .row:last-child {
    margin-bottom: 0;
  }
  #search-form .row .checkbox-col {
    width: 6.5em;
  }
  #search-form label {
    user-select: none;
  }
  #keywords-input,
  #tags-input {
    padding-right: 24px;
    width: 230px;
  }
  #tags-input-div {
    position: relative;
  }
  #tags-suggester {
    position: absolute;
    left: 4px;
    top: 26px;
    max-width: 210px;
    max-height: 26px;
  }
  #keywords-clear,
  #tags-clear {
    left: -28px;
    margin: auto 0;
    padding: 6px 8px;
    position: relative;
  }
  #min-date-input,
  #max-date-input {
    margin: 0 6px;
    width: 70px;
  }
  #first-track-checkbox {
    margin-left: var(--margin);
  }
  #max-plays-input {
    margin: 0 4px;
    padding-right: 2px;
    width: 2em;
  }
  #preset-select {
    /* Prevent a big jump in width when the presets are loaded. This is 90px
     * plus 30px of padding that |commonStyles| sets on select elements. */
    min-width: 110px;
  }
  #search-buttons {
    padding-top: 5px;
    user-select: none;
  }
  #search-buttons > *:not(:first-child) {
    margin-left: var(--button-spacing);
  }
  #search-buttons > button {
    min-width: 80px; /* avoid width jump when font is loaded */
  }
  #results-controls {
    border-top: 1px solid var(--border-color);
    padding: var(--margin) 0 var(--margin) var(--margin);
    user-select: none;
    white-space: nowrap;
  }
  #results-controls .heading {
    margin-right: 4px;
  }
  #results-controls > *:not(:first-child) {
    margin-left: var(--button-spacing);
  }
  #results-controls > button {
    min-width: 80px; /* avoid width jump when font is loaded */
  }
  #spinner {
    display: none;
    fill: var(--text-color);
    height: 16px;
    opacity: 0.9;
    position: fixed;
    right: var(--margin);
    top: var(--margin);
    width: 16px;
  }
  #spinner.shown {
    display: block;
  }
</style>

<form id="search-form" autocomplete="off">
  <div class="heading">Search</div>
  <div class="row">
    <input
      id="keywords-input"
      type="text"
      placeholder="Keywords"
      title="Space-separated words from artists, titles, and albums"
    />
    <svg id="keywords-clear" title="Clear text"></svg>
  </div>

  <div id="tags-input-div" class="row">
    <tag-suggester id="tags-suggester" tab-advances-focus>
      <input
        id="tags-input"
        slot="text"
        type="text"
        placeholder="Tags"
        title="Space-separated user-assigned tags ('-' to exclude)"
      />
    </tag-suggester>
    <svg id="tags-clear" title="Clear text"></svg>
  </div>

  <div class="row">
    Between
    <input
      id="min-date-input"
      type="text"
      placeholder="min date"
      title="Minimum song date (YYYY-MM-DD or YYYY)"
    />
    and
    <input
      id="max-date-input"
      type="text"
      placeholder="max date"
      title="Maximum song date (YYYY-MM-DD or YYYY)"
    />
  </div>

  <div class="row">
    <label for="shuffle-checkbox" class="checkbox-col" title="Shuffle results">
      <input id="shuffle-checkbox" type="checkbox" value="shuffle" />
      Shuffle
    </label>
    <label
      for="order-by-last-played-checkbox"
      class="checkbox-col"
      title="Order results by last-played time"
    >
      <input
        id="order-by-last-played-checkbox"
        type="checkbox"
        value="orderByLastPlayed"
      />
      Oldest
    </label>
    <label
      for="first-track-checkbox"
      class="checkbox-col"
      title="Only first tracks from albums"
    >
      <input id="first-track-checkbox" type="checkbox" value="firstTrack" />
      First track
    </label>
  </div>

  <div class="row">
    <label
      for="unrated-checkbox"
      class="checkbox-col"
      title="Only unrated songs"
    >
      <input id="unrated-checkbox" type="checkbox" value="unrated" />
      Unrated
    </label>
    <label for="min-rating-select" title="Only songs with at least this rating">
      Min rating
      <span class="select-wrapper">
        <select id="min-rating-select">
          <!-- Put U+2009 (THIN SPACE) between characters since the star icons
               in the Fontello font are crammed together otherwise. -->
          <option value=""></option>
          <option value="1">★</option>
          <option value="2">★ ★</option>
          <option value="3">★ ★ ★</option>
          <option value="4">★ ★ ★ ★</option>
          <option value="5">★ ★ ★ ★ ★</option>
        </select></span
      >
    </label>
  </div>

  <div class="row">
    <label
      for="max-plays-input"
      title="Maximum number of times songs have been played"
    >
      Played <input id="max-plays-input" type="text" /> or fewer times
    </label>
  </div>

  <div class="row">
    <label for="first-played-select">
      <span>First played </span>
      <span class="select-wrapper">
        <select id="first-played-select">
          <option value="0"></option>
          <option value="86400">one day</option>
          <option value="604800">one week</option>
          <option value="2592000">one month</option>
          <option value="7776000">three months</option>
          <option value="15552000">six months</option>
          <option value="31536000">one year</option>
          <option value="94608000">three years</option>
          <option value="157680000">five years</option>
        </select></span
      >
      or less ago
    </label>
  </div>

  <div class="row">
    <label for="last-played-select">
      <span>Last played </span>
      <span class="select-wrapper">
        <select id="last-played-select">
          <option value="0"></option>
          <option value="86400">one day</option>
          <option value="604800">one week</option>
          <option value="2592000">one month</option>
          <option value="7776000">three months</option>
          <option value="15552000">six months</option>
          <option value="31536000">one year</option>
          <option value="94608000">three years</option>
          <option value="157680000">five years</option>
        </select></span
      >
      or longer ago
    </label>
  </div>

  <div class="row">
    <label for="preset-select">
      <span>Preset</span>
      <span class="select-wrapper">
        <select id="preset-select">
          <option value=""></option></select
      ></span>
    </label>
  </div>

  <div class="row">
    <div id="search-buttons">
      <!-- A "button" type is needed to prevent these from submitting the
           form by default, which seems really dumb.
           https://stackoverflow.com/q/932653 -->
      <button id="search-button" type="button" title="Perform search">
        Search
      </button>
      <button id="reset-button" type="button" title="Clear search form">
        Reset
      </button>
      <button
        id="lucky-button"
        type="button"
        title="Perform search and replace playlist with results"
      >
        I'm Feeling Lucky
      </button>
    </div>
  </div>
</form>

<div id="results-controls">
  <span class="heading">Results</span>
  <button id="append-button" disabled title="Append results to playlist">
    Append
  </button>
  <button id="insert-button" disabled title="Insert results after current song">
    Insert
  </button>
  <button id="replace-button" disabled title="Replace playlist with results">
    Replace
  </button>
</div>

<song-table id="results-table" use-checkboxes></song-table>

<svg id="spinner"></svg>
`);

// <search-view> displays a form for sending queries to the server and
// displays the results in a <song-table>. It provides controls for enqueuing
// some or all of the results.
//
// When songs should be enqueued, an 'enqueue' CustomEvent is emitted with the
// following properties in its |detail| property:
// - |songs|: array of song objects
// - |clearFirst|: true if playlist should be cleared first
// - |afterCurrent|: true to insert songs after current song (rather than at end)
// - |shuffled|: true if search results were shuffled
export class SearchView extends HTMLElement {
  #fetchController: AbortController | null = null;
  #shadow = createShadow(this, template);

  // Convenience methods for initializing members.
  #getInput = (id: string) => $(id, this.#shadow) as HTMLInputElement;
  #getSelect = (id: string) => $(id, this.#shadow) as HTMLSelectElement;
  #getButton = (id: string) => $(id, this.#shadow) as HTMLButtonElement;

  #keywordsInput = this.#getInput('keywords-input');
  #tagSuggester = $('tags-suggester', this.#shadow) as TagSuggester;
  #tagsInput = this.#getInput('tags-input');
  #minDateInput = this.#getInput('min-date-input');
  #maxDateInput = this.#getInput('max-date-input');
  #shuffleCheckbox = this.#getInput('shuffle-checkbox');
  #firstTrackCheckbox = this.#getInput('first-track-checkbox');
  #unratedCheckbox = this.#getInput('unrated-checkbox');
  #minRatingSelect = this.#getSelect('min-rating-select');
  #orderByLastPlayedCheckbox = this.#getInput('order-by-last-played-checkbox');
  #maxPlaysInput = this.#getInput('max-plays-input');
  #firstPlayedSelect = this.#getSelect('first-played-select');
  #lastPlayedSelect = this.#getSelect('last-played-select');
  #presetSelect = this.#getSelect('preset-select');

  #resultsTable = $('results-table', this.#shadow) as SongTable;
  #spinner = $('spinner', this.#shadow);
  #presets: SearchPreset[] = [];
  #resultsShuffled = false;

  constructor() {
    super();

    this.#shadow.adoptedStyleSheets = [commonStyles];

    this.#keywordsInput.addEventListener('keydown', this.#onFormKeyDown);
    setIcon($('keywords-clear', this.#shadow), xIcon).addEventListener(
      'click',
      () => (this.#keywordsInput.value = '')
    );

    this.#tagsInput.addEventListener('keydown', this.#onFormKeyDown);
    setIcon($('tags-clear', this.#shadow), xIcon).addEventListener(
      'click',
      () => (this.#tagsInput.value = '')
    );

    this.#minDateInput.addEventListener('keydown', this.#onFormKeyDown);
    this.#maxDateInput.addEventListener('keydown', this.#onFormKeyDown);

    this.#spinner = setIcon(this.#spinner, spinnerIcon);

    this.#shuffleCheckbox.addEventListener('keydown', this.#onFormKeyDown);
    this.#firstTrackCheckbox.addEventListener('keydown', this.#onFormKeyDown);
    this.#unratedCheckbox.addEventListener('keydown', this.#onFormKeyDown);
    this.#unratedCheckbox.addEventListener('change', () =>
      this.#updateFormDisabledState()
    );
    this.#orderByLastPlayedCheckbox.addEventListener(
      'keydown',
      this.#onFormKeyDown
    );
    this.#maxPlaysInput.addEventListener('keydown', this.#onFormKeyDown);
    this.#presetSelect.addEventListener('change', this.#onPresetSelectChange);

    const handleButton = (id: string, cb: () => void) => {
      const button = this.#getButton(id);
      button.addEventListener('click', () => {
        // Unfocus the button so it isn't visibly focused when a <dialog> is
        // closed.
        button.blur();
        cb();
      });
    };
    handleButton('search-button', () => this.#submitQuery(false));
    handleButton('reset-button', () => this.#reset(null, null, null, true));
    handleButton('lucky-button', () => this.#doLuckySearch());
    handleButton('append-button', () =>
      this.#enqueueSearchResults(
        false /* clearFirst */,
        false /* afterCurrent */
      )
    );
    handleButton('insert-button', () =>
      this.#enqueueSearchResults(
        false /* clearFirst */,
        true /* afterCurrent */
      )
    );
    handleButton('replace-button', () =>
      this.#enqueueSearchResults(
        true /* clearFirst */,
        false /* afterCurrent */
      )
    );

    this.#resultsTable.addEventListener('field', ((e: CustomEvent) => {
      this.#reset(
        e.detail.artist,
        e.detail.album,
        e.detail.albumId,
        false /* clearResults */
      );
    }) as EventListenerOrEventListenerObject);
    this.#resultsTable.addEventListener('check', ((e: CustomEvent) => {
      const checked = !!e.detail.count;
      this.#getButton('append-button').disabled =
        this.#getButton('insert-button').disabled =
        this.#getButton('replace-button').disabled =
          !checked;
    }) as EventListenerOrEventListenerObject);
    this.#resultsTable.addEventListener('menu', ((e: CustomEvent) => {
      const idx = e.detail.index;
      const orig = e.detail.orig;
      orig.preventDefault();
      const menu = createMenu(orig.pageX, orig.pageY, [
        {
          id: 'info',
          text: 'Info…',
          cb: () => showSongInfoDialog(this.#resultsTable.getSong(idx)),
        },
        {
          id: 'debug',
          text: 'Debug…',
          cb: () => window.open(getDumpSongUrl(e.detail.songId), '_blank'),
        },
      ]);

      // Highlight the row while the menu is open.
      this.#resultsTable.setRowMenuShown(idx, true);
      menu.addEventListener('close', () => {
        this.#resultsTable.setRowMenuShown(idx, false);
      });
    }) as EventListenerOrEventListenerObject);

    this.#getPresetsFromServer();
  }

  connectedCallback() {
    document.body.addEventListener('keydown', this.#onBodyKeyDown);
  }

  disconnectedCallback() {
    document.body.removeEventListener('keydown', this.#onBodyKeyDown);
  }

  // Uses |tags| as autocomplete suggestions in the tags search field.
  set tags(tags: string[]) {
    // Also suggest negative tags.
    this.#tagSuggester.words = tags.concat(tags.map((t) => '-' + t));
  }

  // Resets the search fields using the supplied (optional) values.
  resetFields(
    artist: string | null = null,
    album: string | null = null,
    albumId: string | null = null
  ) {
    this.#reset(artist, album, albumId, false);
  }

  resetForTest() {
    this.#reset(null, null, null, true /* clearResults */);
  }

  #getPresetsFromServer() {
    fetch('presets', { method: 'GET' })
      .then((res) => handleFetchError(res))
      .then((res) => res.json())
      .then((presets) => {
        if (!presets) return;
        this.#presets = presets;
        for (const p of presets) {
          const opt = document.createElement('option');
          opt.text = p.name;
          this.#presetSelect.add(opt);
        }
        console.log(`Loaded ${presets.length} preset(s)`);
      })
      .catch((err) => {
        console.error(`Failed loading presets: ${err}`);
      });
  }

  #submitQuery(appendToQueue: boolean) {
    let terms: String[] = [];
    if (this.#keywordsInput.value.trim()) {
      terms = terms.concat(parseQueryString(this.#keywordsInput.value));
    }
    if (this.#tagsInput.value.trim()) {
      terms.push('tags=' + encodeURIComponent(this.#tagsInput.value.trim()));
    }
    if (this.#minDateInput.value.trim()) {
      let s = this.#minDateInput.value.trim();
      if (s.match(/^\d{4}$/)) s += '-01-01';
      if (s.match(/^\d{4}-\d{2}-\d{2}/)) {
        terms.push('minDate=' + encodeURIComponent(new Date(s).toISOString()));
      }
    }
    if (this.#maxDateInput.value.trim()) {
      let s = this.#maxDateInput.value.trim();
      if (s.match(/^\d{4}$/)) s += '-12-31T23:59:59.999Z';
      if (s.match(/^\d{4}-\d{2}-\d{2}/)) {
        terms.push('maxDate=' + encodeURIComponent(new Date(s).toISOString()));
      }
    }
    if (!this.#minRatingSelect.disabled && this.#minRatingSelect.value !== '') {
      terms.push('minRating=' + this.#minRatingSelect.value);
    }
    if (this.#shuffleCheckbox.checked) terms.push('shuffle=1');
    if (this.#firstTrackCheckbox.checked) terms.push('firstTrack=1');
    if (this.#unratedCheckbox.checked) terms.push('unrated=1');
    if (this.#orderByLastPlayedCheckbox.checked) {
      terms.push('orderByLastPlayed=1');
    }
    if (parseInt(this.#maxPlaysInput.value) >= 0) {
      terms.push('maxPlays=' + parseInt(this.#maxPlaysInput.value));
    }
    const firstPlayed = parseInt(this.#firstPlayedSelect.value);
    if (firstPlayed !== 0) {
      const date = new Date(Date.now() - firstPlayed * 1000);
      terms.push('minFirstPlayed=' + encodeURIComponent(date.toISOString()));
    }
    const lastPlayed = parseInt(this.#lastPlayedSelect.value);
    if (lastPlayed !== 0) {
      const date = new Date(Date.now() - lastPlayed * 1000);
      terms.push('maxLastPlayed=' + encodeURIComponent(date.toISOString()));
    }

    if (!terms.length) {
      showMessageDialog('Invalid Search', 'You must supply search terms.');
      return;
    }

    const url = 'query?' + terms.join('&');
    console.log(`Sending query: ${url}`);

    // 'Order by last played' essentially shuffles the results.
    const shuffled =
      this.#shuffleCheckbox.checked || this.#orderByLastPlayedCheckbox.checked;

    this.#fetchController?.abort();
    this.#fetchController = new AbortController();
    const signal = this.#fetchController.signal;

    this.#spinner?.classList.add('shown');

    fetch(url, { method: 'GET', signal })
      .then((res) => handleFetchError(res))
      .then((res) => res.json())
      .then((songs: Song[]) => {
        console.log('Got response with ' + songs.length + ' song(s)');
        this.#resultsTable.setSongs(songs);
        this.#resultsTable.setAllCheckboxes(true);
        this.#resultsShuffled = shuffled;
        if (appendToQueue) {
          this.#enqueueSearchResults(true, true);
        } else if (songs.length > 0 && songs[0].coverFilename) {
          // If we aren't automatically enqueuing the results, prefetch the
          // cover image for the first song so it'll be ready to go.
          new Image().src = getCoverUrl(songs[0].coverFilename, smallCoverSize);
        }
      })
      .catch((err) => {
        showMessageDialog('Search Failed', err.toString());
      })
      .finally(() => {
        this.#spinner?.classList.remove('shown');
      });
  }

  #enqueueSearchResults(clearFirst: boolean, afterCurrent: boolean) {
    const songs = this.#resultsTable.checkedSongs;
    if (!songs.length) return;

    const detail = {
      songs,
      clearFirst,
      afterCurrent,
      shuffled: this.#resultsShuffled,
    };
    this.dispatchEvent(new CustomEvent('enqueue', { detail }));

    if (songs.length === this.#resultsTable.numSongs) {
      this.#resultsTable.setSongs([]);
    }
  }

  // Resets all of the fields in the search form. If |newArtist|, |newAlbum|,
  // or |newAlbumId| are non-null, the supplied values are used. Also jumps to
  // the top of the page so the form is visible.
  #reset(
    newArtist: string | null,
    newAlbum: string | null,
    newAlbumId: string | null,
    clearResults: boolean
  ) {
    const keywords = [];
    const clean = (s: string) => {
      s = s.replace(/"/g, '\\"');
      if (s.includes(' ')) s = '"' + s + '"';
      return s;
    };
    if (newArtist) keywords.push('artist:' + clean(newArtist));
    if (newAlbum) keywords.push('album:' + clean(newAlbum));
    if (newAlbumId) keywords.push('albumId:' + clean(newAlbumId));

    this.#keywordsInput.value = keywords.join(' ');
    this.#tagsInput.value = '';
    this.#minDateInput.value = '';
    this.#maxDateInput.value = '';
    this.#shuffleCheckbox.checked = false;
    this.#firstTrackCheckbox.checked = false;
    this.#unratedCheckbox.checked = false;
    this.#minRatingSelect.selectedIndex = 0;
    this.#orderByLastPlayedCheckbox.checked = false;
    this.#maxPlaysInput.value = '';
    this.#firstPlayedSelect.selectedIndex = 0;
    this.#lastPlayedSelect.selectedIndex = 0;
    this.#presetSelect.selectedIndex = 0;

    this.#updateFormDisabledState();

    if (clearResults) this.#resultsTable.setSongs([]);
    this.scrollIntoView();
  }

  // Handles the "I'm Feeling Lucky" button being clicked.
  #doLuckySearch() {
    if (
      this.#keywordsInput.value.trim() === '' &&
      this.#tagsInput.value.trim() === '' &&
      !this.#shuffleCheckbox.checked &&
      !this.#firstTrackCheckbox.checked &&
      !this.#unratedCheckbox.checked &&
      this.#minRatingSelect.selectedIndex === 0 &&
      !this.#orderByLastPlayedCheckbox.checked &&
      !(parseInt(this.#maxPlaysInput.value) >= 0) &&
      this.#firstPlayedSelect.selectedIndex === 0 &&
      this.#lastPlayedSelect.selectedIndex === 0
    ) {
      this.#reset(null, null, null, false /* clearResults */);
      this.#shuffleCheckbox.checked = true;
      this.#minRatingSelect.selectedIndex = 4; // 4 stars
    }
    this.#submitQuery(true);
  }

  // Handles a key being pressed in the search form.
  #onFormKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Enter') {
      this.#submitQuery(false);
    } else if ([' ', 'ArrowLeft', 'ArrowRight', '/'].includes(e.key)) {
      e.stopPropagation();
    }
  };

  #updateFormDisabledState() {
    this.#minRatingSelect.disabled = this.#unratedCheckbox.checked;
  }

  #onPresetSelectChange = () => {
    const index = this.#presetSelect.selectedIndex;
    if (index === 0) return; // ignore '...' item

    const preset = this.#presets[index - 1]; // skip '...' item
    this.#reset(null, null, null, false /* clearResults */);
    this.#presetSelect.selectedIndex = index;

    this.#tagsInput.value = preset.tags;
    this.#minRatingSelect.selectedIndex = clamp(preset.minRating, 0, 5);
    this.#unratedCheckbox.checked = preset.unrated;
    this.#orderByLastPlayedCheckbox.checked = preset.orderByLastPlayed;
    this.#firstPlayedSelect.selectedIndex = preset.firstPlayed;
    this.#lastPlayedSelect.selectedIndex = preset.lastPlayed;
    this.#maxPlaysInput.value =
      preset.maxPlays >= 0 ? preset.maxPlays.toString() : '';
    this.#firstTrackCheckbox.checked = preset.firstTrack;
    this.#shuffleCheckbox.checked = preset.shuffle;

    this.#updateFormDisabledState();

    // Unfocus the element so that arrow keys or Page Up/Down won't select new
    // presets.
    this.#presetSelect.blur();

    this.#submitQuery(preset.play);
  };

  #onBodyKeyDown = (e: KeyboardEvent) => {
    if (isDialogShown() || isMenuShown()) return;

    if (e.key === '/') {
      this.#keywordsInput.focus();
      e.preventDefault();
      e.stopPropagation();
    }
  };
}

customElements.define('search-view', SearchView);

function parseQueryString(text: string) {
  const terms: string[] = [];
  let keywords: string[] = [];

  text = text.trim();
  while (text.length > 0) {
    if (
      text.startsWith('artist:') ||
      text.startsWith('title:') ||
      text.startsWith('albumId:') ||
      text.startsWith('album:')
    ) {
      const key = text.substring(0, text.indexOf(':'));

      // Skip over key and leading whitespace.
      let index = key.length + 1;
      for (; index < text.length && text[index] === ' '; index++);

      let value = '';
      let inEscape = false;
      let inQuote = false;
      for (; index < text.length; index++) {
        const ch = text[index];
        if (ch === '\\' && !inEscape) {
          inEscape = true;
        } else if (ch === '"' && !inEscape) {
          inQuote = !inQuote;
        } else if (ch === ' ' && !inQuote) {
          break;
        } else {
          value += ch;
          inEscape = false;
        }
      }

      if (value.length > 0) terms.push(key + '=' + encodeURIComponent(value));
      text = text.substring(index);
    } else {
      const match = text.match(/^(\S+)(.*)/);
      // The server splits on non-alphanumeric characters to make keywords.
      // Split on miscellaneous punctuation here to at least handle some of this.
      keywords = keywords.concat(
        match![1].split(/[-_+=~!?@#$%^&*()'".,:;]+/).filter((s) => s.length)
      );
      text = match![2];
    }
    text = text.trim();
  }

  if (keywords.length > 0) {
    terms.push('keywords=' + encodeURIComponent(keywords.join(' ')));
  }

  return terms;
}
