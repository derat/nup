// Copyright 2010 Daniel Erat.
// All rights reserved.

import {
  $,
  clamp,
  commonStyles,
  createShadow,
  createTemplate,
  getCoverUrl,
  getCurrentTimeSec,
  getDumpSongUrl,
  handleFetchError,
  smallCoverSize,
} from './common.js';
import { isDialogShown, showMessageDialog } from './dialog.js';
import { createMenu, isMenuShown } from './menu.js';
import { showSongInfo } from './song-info.js';
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
    width: 8em;
  }
  #search-form .row .label-col {
    width: 6em;
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
    bottom: 1px;
    cursor: pointer;
    padding: 6px 8px;
    position: relative;
    right: 26px;
  }
  #min-rating-select {
    /* The stars are too close together, but letter-spacing unfortunately
     * doesn't work on <select>. */
    font-family: var(--icon-font-family);
    font-size: 12px;
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
  #waiting {
    background-color: #a00;
    border-radius: 8px;
    color: #fff;
    display: none;
    font-size: 11px;
    padding: 5px;
    position: fixed;
    right: var(--margin);
    top: var(--margin);
  }
  #waiting.shown {
    display: block;
  }
</style>

<form id="search-form" autocomplete="off">
  <div class="heading">Search</div>
  <div class="row">
    <input id="keywords-input" type="text" placeholder="Keywords" />
    <span id="keywords-clear" class="x-icon" title="Clear text"></span>
  </div>

  <div id="tags-input-div" class="row">
    <tag-suggester id="tags-suggester" tab-advances-focus>
      <input id="tags-input" slot="text" type="text" placeholder="Tags" />
    </tag-suggester>
    <span id="tags-clear" class="x-icon" title="Clear text"></span>
  </div>

  <div class="row">
    <label for="shuffle-checkbox" class="checkbox-col">
      <input id="shuffle-checkbox" type="checkbox" value="shuffle" />
      Shuffle
    </label>
    <label for="first-track-checkbox">
      <input id="first-track-checkbox" type="checkbox" value="firstTrack" />
      First track
    </label>
  </div>

  <div class="row">
    <label for="unrated-checkbox" class="checkbox-col">
      <input id="unrated-checkbox" type="checkbox" value="unrated" />
      Unrated
    </label>
    <label for="min-rating-select">
      Min rating
      <span class="select-wrapper">
        <select id="min-rating-select">
          <option value=""></option>
          <option value="1">★</option>
          <option value="2">★★</option>
          <option value="3">★★★</option>
          <option value="4">★★★★</option>
          <option value="5">★★★★★</option>
        </select></span
      >
    </label>
  </div>

  <div class="row">
    <label for="order-by-last-played-checkbox" class="checkbox-col">
      <input
        id="order-by-last-played-checkbox"
        type="checkbox"
        value="orderByLastPlayed"
      />
      Order by last played
    </label>
  </div>

  <div class="row">
    <label for="max-plays-input">
      Played <input id="max-plays-input" type="text" /> or fewer times
    </label>
  </div>

  <div class="row">
    <label for="first-played-select">
      <span class="label-col">First played</span>
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
      >or less ago
    </label>
  </div>

  <div class="row">
    <label for="last-played-select">
      <span class="label-col">Last played</span>
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
      >or longer ago
    </label>
  </div>

  <div class="row">
    <label for="preset-select">
      <span class="label-col">Preset</span>
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
      <button id="search-button" type="button">Search</button>
      <button id="reset-button" type="button">Reset</button>
      <button id="lucky-button" type="button">I'm Feeling Lucky</button>
    </div>
  </div>
</form>

<div id="results-controls">
  <span class="heading">Results</span>
  <button id="append-button" disabled>Append</button>
  <button id="insert-button" disabled>Insert</button>
  <button id="replace-button" disabled>Replace</button>
</div>

<song-table id="results-table" use-checkboxes></song-table>

<div id="waiting">Waiting for server...</div>
`);

// <music-searcher> displays a form for sending queries to the server and
// displays the results in a <song-table>. It provides controls for enqueuing
// some or all of the results.
//
// When songs should be enqueued, an 'enqueue' CustomEvent is emitted with the
// following properties in its |detail| property:
// - |songs|: array of song objects
// - |clearFirst|: true if playlist should be cleared first
// - |afterCurrent|: true to insert songs after current song (rather than at end)
// - |shuffled|: true if search results were shuffled
export class MusicSearcher extends HTMLElement {
  fetchController_: AbortController | null = null;
  shadow_ = createShadow(this, template);

  // Convenience methods for initializing members.
  getInput_ = (id: string) => $(id, this.shadow_) as HTMLInputElement;
  getSelect_ = (id: string) => $(id, this.shadow_) as HTMLSelectElement;
  getButton_ = (id: string) => $(id, this.shadow_) as HTMLButtonElement;

  keywordsInput_ = this.getInput_('keywords-input');
  tagSuggester_ = $('tags-suggester', this.shadow_) as TagSuggester;
  tagsInput_ = this.getInput_('tags-input');
  shuffleCheckbox_ = this.getInput_('shuffle-checkbox');
  firstTrackCheckbox_ = this.getInput_('first-track-checkbox');
  unratedCheckbox_ = this.getInput_('unrated-checkbox');
  minRatingSelect_ = this.getSelect_('min-rating-select');
  orderByLastPlayedCheckbox_ = this.getInput_('order-by-last-played-checkbox');
  maxPlaysInput_ = this.getInput_('max-plays-input');
  firstPlayedSelect_ = this.getSelect_('first-played-select');
  lastPlayedSelect_ = this.getSelect_('last-played-select');
  presetSelect_ = this.getSelect_('preset-select');

  resultsTable_ = $('results-table', this.shadow_) as SongTable;
  waitingDiv_ = $('waiting', this.shadow_);
  presets_: SearchPreset[] = [];
  resultsShuffled_ = false;

  constructor() {
    super();

    this.shadow_.adoptedStyleSheets = [commonStyles];

    this.keywordsInput_.addEventListener('keydown', this.onFormKeyDown_);
    $('keywords-clear', this.shadow_).addEventListener(
      'click',
      () => (this.keywordsInput_.value = '')
    );

    this.tagsInput_.addEventListener('keydown', this.onFormKeyDown_);
    $('tags-clear', this.shadow_).addEventListener(
      'click',
      () => (this.tagsInput_.value = '')
    );

    this.shuffleCheckbox_.addEventListener('keydown', this.onFormKeyDown_);
    this.firstTrackCheckbox_.addEventListener('keydown', this.onFormKeyDown_);
    this.unratedCheckbox_.addEventListener('keydown', this.onFormKeyDown_);
    this.unratedCheckbox_.addEventListener('change', () =>
      this.updateFormDisabledState_()
    );
    this.orderByLastPlayedCheckbox_.addEventListener(
      'keydown',
      this.onFormKeyDown_
    );
    this.maxPlaysInput_.addEventListener('keydown', this.onFormKeyDown_);
    this.presetSelect_.addEventListener('change', this.onPresetSelectChange_);

    this.getButton_('search-button').addEventListener('click', () =>
      this.submitQuery_(false)
    );
    this.getButton_('reset-button').addEventListener('click', () =>
      this.reset_(null, null, null, true /* clearResults */)
    );
    this.getButton_('lucky-button').addEventListener('click', () =>
      this.doLuckySearch_()
    );

    this.getButton_('append-button').addEventListener('click', () =>
      this.enqueueSearchResults_(
        false /* clearFirst */,
        false /* afterCurrent */
      )
    );
    this.getButton_('insert-button').addEventListener('click', () =>
      this.enqueueSearchResults_(false, true)
    );
    this.getButton_('replace-button').addEventListener('click', () =>
      this.enqueueSearchResults_(true, false)
    );

    this.resultsTable_.addEventListener('field', (e: CustomEvent) => {
      this.reset_(
        e.detail.artist,
        e.detail.album,
        e.detail.albumId,
        false /* clearResults */
      );
    });
    this.resultsTable_.addEventListener('check', (e: CustomEvent) => {
      const checked = !!e.detail.count;
      this.getButton_('append-button').disabled =
        this.getButton_('insert-button').disabled =
        this.getButton_('replace-button').disabled =
          !checked;
    });
    this.resultsTable_.addEventListener('menu', (e: CustomEvent) => {
      const idx = e.detail.index;
      const orig = e.detail.orig;
      orig.preventDefault();
      const menu = createMenu(orig.pageX, orig.pageY, [
        {
          id: 'info',
          text: 'Info…',
          cb: () => showSongInfo(this.resultsTable_.getSong(idx)),
        },
        {
          id: 'debug',
          text: 'Debug…',
          cb: () => window.open(getDumpSongUrl(e.detail.songId), '_blank'),
        },
      ]);

      // Highlight the row while the menu is open.
      this.resultsTable_.setRowMenuShown(idx, true);
      menu.addEventListener('close', () => {
        this.resultsTable_.setRowMenuShown(idx, false);
      });
    });

    this.getPresetsFromServer_();
  }

  connectedCallback() {
    document.body.addEventListener('keydown', this.onBodyKeyDown_);
  }

  disconnectedCallback() {
    document.body.removeEventListener('keydown', this.onBodyKeyDown_);
  }

  // Uses |tags| as autocomplete suggestions in the tags search field.
  set tags(tags: string[]) {
    // Also suggest negative tags.
    this.tagSuggester_.words = tags.concat(tags.map((t) => '-' + t));
  }

  // Resets the search fields using the supplied (optional) values.
  resetFields(artist?: string, album?: string, albumId?: string) {
    this.reset_(artist, album, albumId, false);
  }

  resetForTesting() {
    this.reset_(null, null, null, true /* clearResults */);
  }

  getPresetsFromServer_() {
    fetch('presets', { method: 'GET' })
      .then((res) => handleFetchError(res))
      .then((res) => res.json())
      .then((presets) => {
        if (!presets) return;
        this.presets_ = presets;
        for (const p of presets) {
          const opt = document.createElement('option');
          opt.text = p.name;
          this.presetSelect_.add(opt);
        }
        console.log(`Loaded ${presets.length} preset(s)`);
      })
      .catch((err) => {
        console.error(`Failed loading presets: ${err}`);
      });
  }

  submitQuery_(appendToQueue: boolean) {
    let terms: String[] = [];
    if (this.keywordsInput_.value.trim()) {
      terms = terms.concat(parseQueryString(this.keywordsInput_.value));
    }
    if (this.tagsInput_.value.trim()) {
      terms.push('tags=' + encodeURIComponent(this.tagsInput_.value.trim()));
    }
    if (!this.minRatingSelect_.disabled && this.minRatingSelect_.value !== '') {
      terms.push('minRating=' + this.minRatingSelect_.value);
    }
    if (this.shuffleCheckbox_.checked) terms.push('shuffle=1');
    if (this.firstTrackCheckbox_.checked) terms.push('firstTrack=1');
    if (this.unratedCheckbox_.checked) terms.push('unrated=1');
    if (this.orderByLastPlayedCheckbox_.checked) {
      terms.push('orderByLastPlayed=1');
    }
    if (parseInt(this.maxPlaysInput_.value) >= 0) {
      terms.push('maxPlays=' + parseInt(this.maxPlaysInput_.value));
    }
    const firstPlayed = parseInt(this.firstPlayedSelect_.value);
    if (firstPlayed !== 0) {
      terms.push(
        `minFirstPlayed=${Math.round(getCurrentTimeSec()) - firstPlayed}`
      );
    }
    const lastPlayed = parseInt(this.lastPlayedSelect_.value);
    if (lastPlayed !== 0) {
      terms.push(
        `maxLastPlayed=${Math.round(getCurrentTimeSec()) - lastPlayed}`
      );
    }

    if (!terms.length) {
      showMessageDialog('Invalid Search', 'You must supply search terms.');
      return;
    }

    const url = 'query?' + terms.join('&');
    console.log(`Sending query: ${url}`);

    const shuffled = this.shuffleCheckbox_.checked;

    this.fetchController_?.abort();
    this.fetchController_ = new AbortController();
    const signal = this.fetchController_.signal;

    this.waitingDiv_.classList.add('shown');

    fetch(url, { method: 'GET', signal })
      .then((res) => handleFetchError(res))
      .then((res) => res.json())
      .then((songs: Song[]) => {
        console.log('Got response with ' + songs.length + ' song(s)');
        this.resultsTable_.setSongs(songs);
        this.resultsTable_.setAllCheckboxes(true);
        this.resultsShuffled_ = shuffled;
        if (appendToQueue) {
          this.enqueueSearchResults_(true, true);
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
        this.waitingDiv_.classList.remove('shown');
      });
  }

  enqueueSearchResults_(clearFirst: boolean, afterCurrent: boolean) {
    const songs = this.resultsTable_.checkedSongs;
    if (!songs.length) return;

    const detail = {
      songs,
      clearFirst,
      afterCurrent,
      shuffled: this.resultsShuffled_,
    };
    this.dispatchEvent(new CustomEvent('enqueue', { detail }));

    if (songs.length === this.resultsTable_.numSongs) {
      this.resultsTable_.setSongs([]);
    }
  }

  // Resets all of the fields in the search form. If |newArtist|, |newAlbum|,
  // or |newAlbumId| are non-null, the supplied values are used. Also jumps to
  // the top of the page so the form is visible.
  reset_(
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

    this.keywordsInput_.value = keywords.join(' ');
    this.tagsInput_.value = '';
    this.shuffleCheckbox_.checked = false;
    this.firstTrackCheckbox_.checked = false;
    this.unratedCheckbox_.checked = false;
    this.minRatingSelect_.selectedIndex = 0;
    this.orderByLastPlayedCheckbox_.checked = false;
    this.maxPlaysInput_.value = '';
    this.firstPlayedSelect_.selectedIndex = 0;
    this.lastPlayedSelect_.selectedIndex = 0;
    this.presetSelect_.selectedIndex = 0;

    this.updateFormDisabledState_();

    if (clearResults) this.resultsTable_.setSongs([]);
    this.scrollIntoView();
  }

  // Handles the "I'm Feeling Lucky" button being clicked.
  doLuckySearch_() {
    if (
      this.keywordsInput_.value.trim() === '' &&
      this.tagsInput_.value.trim() === '' &&
      !this.shuffleCheckbox_.checked &&
      !this.firstTrackCheckbox_.checked &&
      !this.unratedCheckbox_.checked &&
      this.minRatingSelect_.selectedIndex === 0 &&
      !this.orderByLastPlayedCheckbox_.checked &&
      !(parseInt(this.maxPlaysInput_.value) >= 0) &&
      this.firstPlayedSelect_.selectedIndex === 0 &&
      this.lastPlayedSelect_.selectedIndex === 0
    ) {
      this.reset_(null, null, null, false /* clearResults */);
      this.shuffleCheckbox_.checked = true;
      this.minRatingSelect_.selectedIndex = 4; // 4 stars
    }
    this.submitQuery_(true);
  }

  // Handle a key being pressed in the search form.
  onFormKeyDown_ = (e: KeyboardEvent) => {
    if (e.key === 'Enter') {
      this.submitQuery_(false);
    } else if ([' ', 'ArrowLeft', 'ArrowRight', '/'].includes(e.key)) {
      e.stopPropagation();
    }
  };

  updateFormDisabledState_() {
    this.minRatingSelect_.disabled = this.unratedCheckbox_.checked;
  }

  onPresetSelectChange_ = () => {
    const index = this.presetSelect_.selectedIndex;
    if (index === 0) return; // ignore '...' item

    const preset = this.presets_[index - 1]; // skip '...' item
    this.reset_(null, null, null, false /* clearResults */);
    this.presetSelect_.selectedIndex = index;

    this.tagsInput_.value = preset.tags;
    this.minRatingSelect_.selectedIndex = clamp(preset.minRating, 0, 5);
    this.unratedCheckbox_.checked = preset.unrated;
    this.orderByLastPlayedCheckbox_.checked = preset.orderByLastPlayed;
    this.firstPlayedSelect_.selectedIndex = preset.firstPlayed;
    this.lastPlayedSelect_.selectedIndex = preset.lastPlayed;
    this.maxPlaysInput_.value =
      preset.maxPlays >= 0 ? preset.maxPlays.toString() : '';
    this.firstTrackCheckbox_.checked = preset.firstTrack;
    this.shuffleCheckbox_.checked = preset.shuffle;

    this.updateFormDisabledState_();

    // Unfocus the element so that arrow keys or Page Up/Down won't select new
    // presets.
    this.presetSelect_.blur();

    this.submitQuery_(preset.play);
  };

  onBodyKeyDown_ = (e: KeyboardEvent) => {
    if (isDialogShown() || isMenuShown()) return;

    if (e.key === '/') {
      this.keywordsInput_.focus();
      e.preventDefault();
      e.stopPropagation();
    }
  };
}

customElements.define('music-searcher', MusicSearcher);

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
        match[1].split(/[-_+=~!?@#$%^&*()'".,:;]+/).filter((s) => s.length)
      );
      text = match[2];
    }
    text = text.trim();
  }

  if (keywords.length > 0) {
    terms.push('keywords=' + encodeURIComponent(keywords.join(' ')));
  }

  return terms;
}
