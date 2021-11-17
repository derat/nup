// Copyright 2010 Daniel Erat.
// All rights reserved.

import {
  $,
  createShadow,
  createTemplate,
  getCurrentTimeSec,
  getDumpSongUrl,
  handleFetchError,
} from './common.js';

const template = createTemplate(`
<style>
  @import 'common.css';
  :host {
    display: flex;
    flex-direction: column;
  }
  .heading {
    font-size: 15px;
    font-weight: bold;
    margin-left: 2px;
    margin-bottom: 8px;
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
    padding-right: 20px;
    width: 240px;
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
    position: relative;
    right: 24px;
  }
  #min-rating-select {
    /* The stars are too close together, but letter-spacing unfortunately
     * doesn't work on <select>. */
    font-family: var(--icon-font-family);
    font-size: 12px;
  }
  #max-plays-input {
    margin: 0 6px;
    padding-right: 2px;
    width: 2em;
  }
  #search-buttons {
    padding-top: 5px;
  }
  #search-buttons > *:not(:first-child) {
    margin-left: var(--button-spacing);
  }
  #results-controls {
    border-top: 1px solid var(--border-color);
    padding: var(--margin) 0 var(--margin) var(--margin);
    user-select: none;
  }
  #results-controls .heading {
    margin-right: 4px;
  }
  #results-controls > *:not(:first-child) {
    margin-left: var(--button-spacing);
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

<form id="search-form">
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
      <div class="select-wrapper">
        <select id="min-rating-select">
          <option value="0.00">★</option>
          <option value="0.25">★★</option>
          <option value="0.50">★★★</option>
          <option value="0.75">★★★★</option>
          <option value="1.00">★★★★★</option>
        </select>
      </div>
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
      <div class="select-wrapper">
        <select id="first-played-select">
          <option value="0">...</option>
          <option value="86400">one day</option>
          <option value="604800">one week</option>
          <option value="2592000">one month</option>
          <option value="7776000">three months</option>
          <option value="15552000">six months</option>
          <option value="31536000">one year</option>
          <option value="94608000">three years</option>
        </select>
      </div>
      or less ago
    </label>
  </div>

  <div class="row">
    <label for="last-played-select">
      <span class="label-col">Last played</span>
      <div class="select-wrapper">
        <select id="last-played-select">
          <option value="0">...</option>
          <option value="86400">one day</option>
          <option value="604800">one week</option>
          <option value="2592000">one month</option>
          <option value="7776000">three months</option>
          <option value="15552000">six months</option>
          <option value="31536000">one year</option>
          <option value="94608000">three years</option>
        </select>
      </div>
      or longer ago
    </label>
  </div>

  <div class="row">
    <label for="preset-select">
      <span class="label-col">Preset</span>
      <div class="select-wrapper">
        <select id="preset-select">
          <option value="">...</option>
          <option value="mr=3;t=instrumental;lp=6;s=1;play=1">
            instrumental old
          </option>
          <option value="mr=3;t=mellow;s=1;play=1">mellow</option>
          <option value="ft=1;fp=3">new albums</option>
          <option value="u=1;play=1">unrated</option>
          <option value="mr=3;lp=6;s=1;play=1">old</option>
        </select>
      </div>
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

// <music-searcher> sends queries to the server and supports enqueuing the
// resulting songs in a <music-player>.
customElements.define(
  'music-searcher',
  class extends HTMLElement {
    constructor() {
      super();

      this.fetchController_ = null;

      document.body.addEventListener('keydown', (e) =>
        this.handleBodyKeyDown_(e)
      );

      this.shadow_ = createShadow(this, template);
      const get = (id) => $(id, this.shadow_);

      this.keywordsInput_ = get('keywords-input');
      this.keywordsInput_.addEventListener('keydown', (e) =>
        this.handleFormKeyDown_(e)
      );
      get('keywords-clear').addEventListener(
        'click',
        () => (this.keywordsInput_.value = null)
      );

      this.tagSuggester_ = get('tags-suggester');
      this.tagsInput_ = get('tags-input');
      this.tagsInput_.addEventListener('keydown', (e) =>
        this.handleFormKeyDown_(e)
      );
      get('tags-clear').addEventListener(
        'click',
        () => (this.tagsInput_.value = null)
      );

      this.shuffleCheckbox_ = get('shuffle-checkbox');
      this.shuffleCheckbox_.addEventListener('keydown', (e) =>
        this.handleFormKeyDown_(e)
      );
      this.firstTrackCheckbox_ = get('first-track-checkbox');
      this.firstTrackCheckbox_.addEventListener('keydown', (e) =>
        this.handleFormKeyDown_(e)
      );
      this.unratedCheckbox_ = get('unrated-checkbox');
      this.unratedCheckbox_.addEventListener('keydown', (e) =>
        this.handleFormKeyDown_(e)
      );
      this.unratedCheckbox_.addEventListener('change', (e) =>
        this.handleUnratedCheckboxChanged_(e)
      );
      this.minRatingSelect_ = get('min-rating-select');
      this.maxPlaysInput_ = get('max-plays-input');
      this.maxPlaysInput_.addEventListener('keydown', (e) =>
        this.handleFormKeyDown_(e)
      );
      this.firstPlayedSelect_ = get('first-played-select');
      this.lastPlayedSelect_ = get('last-played-select');
      this.presetSelect_ = get('preset-select');
      this.presetSelect_.addEventListener('change', (e) =>
        this.handlePresetSelectChanged_(e)
      );

      this.searchButton_ = get('search-button');
      this.searchButton_.addEventListener('click', () =>
        this.submitQuery_(false)
      );
      this.resetButton_ = get('reset-button');
      this.resetButton_.addEventListener('click', () =>
        this.reset_(null, null, null, true /* clearResults */)
      );
      this.luckyButton_ = get('lucky-button');
      this.luckyButton_.addEventListener('click', () => this.doLuckySearch_());

      this.appendButton_ = get('append-button');
      this.appendButton_.addEventListener('click', () =>
        this.enqueueSearchResults_(
          false /* clearFirst */,
          false /* afterCurrent */
        )
      );
      this.insertButton_ = get('insert-button');
      this.insertButton_.addEventListener('click', () =>
        this.enqueueSearchResults_(false, true)
      );
      this.replaceButton_ = get('replace-button');
      this.replaceButton_.addEventListener('click', () =>
        this.enqueueSearchResults_(true, false)
      );

      this.resultsTable_ = get('results-table');
      this.resultsTable_.addEventListener('field', (e) => {
        this.reset_(
          e.detail.artist,
          e.detail.album,
          e.detail.albumId,
          false /* clearResults */
        );
      });
      this.resultsTable_.addEventListener('check', (e) => {
        const checked = !!e.detail.count;
        this.appendButton_.disabled =
          this.insertButton_.disabled =
          this.replaceButton_.disabled =
            !checked;
      });
      this.resultsTable_.addEventListener('menu', (e) => {
        if (!this.dialogManager_) throw new Error('No <dialog-manager>');

        const idx = e.detail.index;
        const orig = e.detail.orig;
        orig.preventDefault();
        const menu = this.dialogManager_.createMenu(orig.pageX, orig.pageY, [
          {
            id: 'debug',
            text: 'Debug',
            cb: () => window.open(getDumpSongUrl(e.detail.songId), '_blank'),
          },
        ]);

        // Highlight the row while the menu is open.
        this.resultsTable_.setRowMenuShown(idx, true);
        menu.addEventListener('close', () => {
          this.resultsTable_.setRowMenuShown(idx, false);
        });
      });

      this.waitingDiv_ = get('waiting');
    }

    set dialogManager(manager) {
      this.dialogManager_ = manager;
    }
    set musicPlayer(player) {
      this.musicPlayer_ = player;
      player.addEventListener('field', (e) => {
        this.reset_(
          e.detail.artist,
          e.detail.album,
          e.detail.albumId,
          false /* clearResults */
        );
      });
      player.addEventListener('tags', (e) => {
        this.tagSuggester_.words = e.detail.tags;
      });
    }

    resetForTesting() {
      this.reset_(null, null, null, true /* clearResults */);
    }

    submitQuery_(appendToQueue) {
      let terms = [];
      if (this.keywordsInput_.value.trim()) {
        terms = terms.concat(parseQueryString(this.keywordsInput_.value));
      }
      if (this.tagsInput_.value.trim()) {
        terms.push('tags=' + encodeURIComponent(this.tagsInput_.value.trim()));
      }
      if (this.minRatingSelect_.value != 0 && !this.unratedCheckbox_.checked) {
        terms.push('minRating=' + this.minRatingSelect_.value);
      }
      if (this.shuffleCheckbox_.checked) terms.push('shuffle=1');
      if (this.firstTrackCheckbox_.checked) terms.push('firstTrack=1');
      if (this.unratedCheckbox_.checked) terms.push('unrated=1');
      if (!isNaN(parseInt(this.maxPlaysInput_.value))) {
        terms.push('maxPlays=' + parseInt(this.maxPlaysInput_.value));
      }
      if (this.firstPlayedSelect_.value != 0) {
        terms.push(
          'minFirstPlayed=' +
            (getCurrentTimeSec() - parseInt(this.firstPlayedSelect_.value))
        );
      }
      if (this.lastPlayedSelect_.value != 0) {
        terms.push(
          'maxLastPlayed=' +
            (getCurrentTimeSec() - parseInt(this.lastPlayedSelect_.value))
        );
      }

      if (!terms.length) {
        this.showMessage_('Invalid Search', 'You must supply search terms.');
        return;
      }

      const url = 'query?' + terms.join('&');
      console.log(`Sending query: ${url}`);

      if (this.fetchController_) this.fetchController_.abort();
      this.fetchController_ = new AbortController();
      const signal = this.fetchController_.signal;

      this.waitingDiv_.classList.add('shown');

      fetch(url, { method: 'GET', signal })
        .then((res) => handleFetchError(res))
        .then((res) => res.json())
        .then((songs) => {
          console.log('Got response with ' + songs.length + ' song(s)');
          this.resultsTable_.setSongs(songs);
          this.resultsTable_.setAllCheckboxes(true);
          if (appendToQueue) this.enqueueSearchResults_(true, true);
        })
        .catch((err) => {
          this.showMessage_('Search Failed', err.toString());
        })
        .finally(() => {
          this.waitingDiv_.classList.remove('shown');
        });
    }

    enqueueSearchResults_(clearFirst, afterCurrent) {
      if (!this.musicPlayer_) throw new Error('No <music-player>');
      if (!this.resultsTable_.numSongs) return;

      const songs = this.resultsTable_.checkedSongs;
      this.musicPlayer_.enqueueSongs(songs, clearFirst, afterCurrent);
      if (songs.length == this.resultsTable_.numSongs) {
        this.resultsTable_.setSongs([]);
      }
    }

    // Resets all of the fields in the search form. If |newArtist|, |newAlbum|,
    // or |newAlbumId| are non-null, the supplied values are used. Also jumps to
    // the top of the page so the form is visible.
    reset_(newArtist, newAlbum, newAlbumId, clearResults) {
      const keywords = [];
      const clean = (s) => {
        s = s.replace(/"/g, '\\"');
        if (s.indexOf(' ') != -1) s = '"' + s + '"';
        return s;
      };
      if (newArtist) keywords.push('artist:' + clean(newArtist));
      if (newAlbum) keywords.push('album:' + clean(newAlbum));
      if (newAlbumId) keywords.push('albumId:' + clean(newAlbumId));

      this.keywordsInput_.value = keywords.join(' ');
      this.tagsInput_.value = null;
      this.shuffleCheckbox_.checked = false;
      this.firstTrackCheckbox_.checked = false;
      this.unratedCheckbox_.checked = false;
      this.minRatingSelect_.selectedIndex = 0;
      this.minRatingSelect_.disabled = false;
      this.maxPlaysInput_.value = null;
      this.firstPlayedSelect_.selectedIndex = 0;
      this.lastPlayedSelect_.selectedIndex = 0;
      this.presetSelect_.selectedIndex = 0;
      if (clearResults) this.resultsTable_.setSongs([]);
      this.scrollIntoView();
    }

    // Handles the "I'm Feeling Lucky" button being clicked.
    doLuckySearch_() {
      if (
        !this.keywordsInput_.value &&
        !this.tagsInput_.value &&
        !this.shuffleCheckbox_.checked &&
        !this.firstTrackCheckbox_.checked &&
        !this.unratedCheckbox_.checked &&
        this.minRatingSelect_.selectedIndex == 0 &&
        !this.maxPlaysInput_.value &&
        this.firstPlayedSelect_.selectedIndex == 0 &&
        this.lastPlayedSelect_.selectedIndex == 0
      ) {
        this.reset_(null, null, null, false /* clearResults */);
        this.shuffleCheckbox_.checked = true;
        this.minRatingSelect_.selectedIndex = 3;
      }
      this.submitQuery_(true);
    }

    // Handle a key being pressed in the search form.
    handleFormKeyDown_(e) {
      if (e.key == 'Enter') {
        this.submitQuery_(false);
      } else if ([' ', 'ArrowLeft', 'ArrowRight', '/'].indexOf(e.key) != -1) {
        e.stopPropagation();
      }
    }

    handleUnratedCheckboxChanged_(event) {
      this.minRatingSelect_.disabled = this.unratedCheckbox_.checked;
    }

    handlePresetSelectChanged_(event) {
      if (this.presetSelect_.value == '') return;

      const index = this.presetSelect_.selectedIndex;
      this.reset_(null, null, null, false /* clearResults */);
      this.presetSelect_.selectedIndex = index;

      let play = false;

      const vals = this.presetSelect_.value.split(';');
      for (let i = 0; i < vals.length; i++) {
        const parts = vals[i].split('=');
        if (parts[0] == 'fp') this.firstPlayedSelect_.selectedIndex = parts[1];
        else if (parts[0] == 'ft')
          this.firstTrackCheckbox_.checked = !!parts[1];
        else if (parts[0] == 'lp')
          this.lastPlayedSelect_.selectedIndex = parts[1];
        else if (parts[0] == 'mr')
          this.minRatingSelect_.selectedIndex = parts[1];
        else if (parts[0] == 'play') play = !!parts[1];
        else if (parts[0] == 's') this.shuffleCheckbox_.checked = !!parts[1];
        else if (parts[0] == 't') this.tagsInput_.value = parts[1];
        else if (parts[0] == 'u') this.unratedCheckbox_.checked = !!parts[1];
        else console.log('Unknown preset setting ' + vals[i]);
      }

      // Unfocus the element so that arrow keys or Page Up/Down won't select new
      // presets.
      this.presetSelect_.blur();

      this.submitQuery_(play);
    }

    handleBodyKeyDown_(e) {
      if (this.dialogManager_ && this.dialogManager_.numChildren) return;

      if (e.key == '/') {
        this.keywordsInput_.focus();
        e.preventDefault();
        e.stopPropagation();
      }
    }

    // Displays a dialog with the supplied title and message.
    showMessage_(title, message) {
      if (!this.dialogManager_) return;
      this.dialogManager_.createMessageDialog(title, message);
    }
  }
);

function parseQueryString(text) {
  let terms = [];
  let keywords = [];

  text = text.trim();
  while (text.length > 0) {
    if (
      text.indexOf('artist:') == 0 ||
      text.indexOf('title:') == 0 ||
      text.indexOf('albumId:') == 0 ||
      text.indexOf('album:') == 0
    ) {
      const key = text.substring(0, text.indexOf(':'));

      // Skip over key and leading whitespace.
      let index = key.length + 1;
      for (; index < text.length && text[index] == ' '; index++);

      let value = '';
      let inEscape = false;
      let inQuote = false;
      for (; index < text.length; index++) {
        const ch = text[index];
        if (ch == '\\' && !inEscape) {
          inEscape = true;
        } else if (ch == '"' && !inEscape) {
          inQuote = !inQuote;
        } else if (ch == ' ' && !inQuote) {
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
