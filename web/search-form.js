// Copyright 2010 Daniel Erat.
// All rights reserved.

import {
  $,
  createShadow,
  createTemplate,
  getCurrentTimeSec,
  KeyCodes,
} from './common.js';

const template = createTemplate(`
<style>
  @import 'common.css';
  :host {
    display: block;
  }
  select {
    font-family: var(--font-family);
    font-size: var(--font-size);
  }
  .heading {
    font-size: 13px;
    font-weight: bold;
    user-select: none;
  }
  #search-heading {
    padding-left: 10px;
    padding-top: 8px;
  }
  #search-table {
    border: 0;
    border-collapse: collapse;
    margin-top: 5px;
    user-select: none;
    white-space: nowrap;
  }
  #search-left {
    width: 4em;
  }
  #search-table td {
    padding-left: 10px;
  }
  #search-table label {
    user-select: none;
  }
  #search-table input[type='text'] {
    border: 1px solid #ddd;
    padding-left: 2px;
  }
  #keywords-input,
  #tags-input {
    padding-right: 17px;
    width: 200px;
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
    position: relative;
    left: -22px;
    top: 2px;
    cursor: pointer;
  }
  #min-rating-select {
    /* The stars are too close together, but letter-spacing unfortunately
     * doesn't work on <select>. */
    color: #555;
    font-family: var(--star-font-family);
    font-size: 12px;
    margin-left: 2px;
    padding: 4px 0 2px 0;
  }
  #min-rating-select:disabled {
    opacity: 0.5;
  }
  #max-plays-input {
    padding-right: 2px;
    width: 2em;
  }
  #search-buttons {
    padding-top: 5px;
  }
  #results-heading {
    padding-right: 10px;
  }
  #results-controls {
    border-top: 1px solid #ddd;
    margin-top: 10px;
    padding-top: 10px;
    padding-left: 10px;
    user-select: none;
  }
  #results-table {
    margin-top: 10px;
  }
  #waiting {
    background-color: #a00;
    border-radius: 8px;
    color: white;
    display: none;
    font-size: 11px;
    padding: 5px;
    position: fixed;
    right: 8px;
    top: 8px;
  }
</style>

<div id="search-heading" class="heading">Search</div>

<form id="search-form">
  <table id="search-table">
    <colgroup>
      <col id="search-left" />
      <col id="search-right" />
    </colgroup>
    <tr>
      <td><label for="keywords-input">Keywords</label></td>
      <td>
        <input id="keywords-input" type="text" size="32" />
        <img
          id="keywords-clear"
          src="images/playlist_clear_text.png"
        />
      </td>
    </tr>
    <tr>
      <td><label for="tags-input">Tags</label></td>
      <td>
        <div id="tags-input-div">
          <tag-suggester id="tags-suggester" tab-advances-focus>
            <input id="tags-input" slot="text" type="text" size="32" />
          </tag-suggester>
          <img
            id="tags-clear"
            src="images/playlist_clear_text.png"
          />
        </div>
      </td>
    </tr>
    <tr>
      <td>
        <label for="shuffle-checkbox">Shuffle</label>
        <input id="shuffle-checkbox" type="checkbox" value="shuffle" />
      </td>
      <td>
        <label for="first-track-checkbox">First track</label>
        <input
          id="first-track-checkbox"
          type="checkbox"
          value="firstTrack"
        />
      </td>
    </tr>
    <tr>
      <td>
        <label for="unrated-checkbox">Unrated</label>
        <input id="unrated-checkbox" type="checkbox" value="unrated" />
      </td>
      <td>
        <label for="min-rating-select">Min rating</label>
        <select id="min-rating-select">
          <option value="0.00">★</option>
          <option value="0.25">★★</option>
          <option value="0.50">★★★</option>
          <option value="0.75">★★★★</option>
          <option value="1.00">★★★★★</option>
        </select>
      </td>
    </tr>
    <tr>
      <td colspan="2">
        <label for="max-plays-input">Played</label>
        <input id="max-plays-input" type="text" />
        <label for="max-plays-input">or fewer times</label>
      </td>
    </tr>
    <tr>
      <td>
        <label for="first-played-select">First played</label>
      </td>
      <td>
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
        <label for="first-played-select">or less ago</label>
      </td>
    </tr>
    <tr>
      <td>
        <label for="last-played-select">Last played</label>
      </td>
      <td>
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
        <label for="last-played-select">or longer ago</label>
      </td>
    </tr>
    <tr>
      <td>
        <label for="preset-select">Preset</label>
      </td>
      <td>
        <select id="preset-select">
          <option value="">...</option>
          <option value="mr=3;t=instrumental;lp=6;s=1;play=1"
            >instrumental old</option
          >
          <option value="mr=3;t=mellow;s=1;play=1">mellow</option>
          <option value="ft=1;fp=3">new albums</option>
          <option value="u=1;play=1">unrated</option>
          <option value="mr=3;lp=6;s=1;play=1">old</option>
        </select>
      </td>
    </tr>
    <tr>
      <td colspan="2">
        <div id="search-buttons">
          <!-- A "button" type is needed to prevent these from submitting the
               form by default, which seems really dumb.
               https://stackoverflow.com/q/932653 -->
          <button id="search-button" type="button">Search</button>
          <button id="reset-button" type="button">Reset</button>
          <button id="lucky-button" type="button">I'm Feeling Lucky</button>
        </div>
      </td>
    </tr>
  </table>
</form>

<div id="results-controls">
  <span id="results-heading" class="heading">Results</span>
  <button id="append-button" disabled>Append</button>
  <button id="insert-button" disabled>Insert</button>
  <button id="replace-button" disabled>Replace</button>
</div>

<song-table id="results-table" use-checkboxes></song-table>

<div id="waiting">Waiting for server...</div>
`);

customElements.define(
  'search-form',
  class extends HTMLElement {
    constructor() {
      super();

      this.dialogManager_ = document.querySelector('dialog-manager');
      if (!this.dialogManager_) throw new Error('No <dialog-manager>');

      const player = document.querySelector('music-player');
      if (!player) throw new Error('No <music-player>');
      player.addEventListener('field', e => {
        this.reset_(e.detail.artist, e.detail.album, false /* clearResults */);
      });
      player.addEventListener('tags', e => {
        this.tagSuggester_.words = e.detail.tags;
      });

      this.request_ = null;

      document.body.addEventListener('keydown', e =>
        this.handleBodyKeyDown_(e),
      );

      this.shadow_ = createShadow(this, template);
      const get = id => $(id, this.shadow_);

      this.keywordsInput_ = get('keywords-input');
      this.keywordsInput_.addEventListener('keydown', e =>
        this.handleFormKeyDown_(e),
      );
      get('keywords-clear').addEventListener(
        'click',
        () => (this.keywordsInput_.value = null),
      );

      this.tagSuggester_ = get('tags-suggester');
      this.tagsInput_ = get('tags-input');
      this.tagsInput_.addEventListener('keydown', e =>
        this.handleFormKeyDown_(e),
      );
      get('tags-clear').addEventListener(
        'click',
        () => (this.tagsInput_.value = null),
      );

      this.shuffleCheckbox_ = get('shuffle-checkbox');
      this.shuffleCheckbox_.addEventListener('keydown', e =>
        this.handleFormKeyDown_(e),
      );
      this.firstTrackCheckbox_ = get('first-track-checkbox');
      this.firstTrackCheckbox_.addEventListener('keydown', e =>
        this.handleFormKeyDown_(e),
      );
      this.unratedCheckbox_ = get('unrated-checkbox');
      this.unratedCheckbox_.addEventListener('keydown', e =>
        this.handleFormKeyDown_(e),
      );
      this.unratedCheckbox_.addEventListener('change', e =>
        this.handleUnratedCheckboxChanged_(e),
      );
      this.minRatingSelect_ = get('min-rating-select');
      this.maxPlaysInput_ = get('max-plays-input');
      this.maxPlaysInput_.addEventListener('keydown', e =>
        this.handleFormKeyDown_(e),
      );
      this.firstPlayedSelect_ = get('first-played-select');
      this.lastPlayedSelect_ = get('last-played-select');
      this.presetSelect_ = get('preset-select');
      this.presetSelect_.addEventListener('change', e =>
        this.handlePresetSelectChanged_(e),
      );

      this.searchButton_ = get('search-button');
      this.searchButton_.addEventListener('click', () =>
        this.submitQuery_(false),
      );
      this.resetButton_ = get('reset-button');
      this.resetButton_.addEventListener('click', () =>
        this.reset_(null, null, true /* clearResults */),
      );
      this.luckyButton_ = get('lucky-button');
      this.luckyButton_.addEventListener('click', () => this.doLuckySearch_());

      this.appendButton_ = get('append-button');
      this.appendButton_.addEventListener('click', () =>
        this.enqueueSearchResults_(
          false /* clearFirst */,
          false /* afterCurrent */,
        ),
      );
      this.insertButton_ = get('insert-button');
      this.insertButton_.addEventListener('click', () =>
        this.enqueueSearchResults_(false, true),
      );
      this.replaceButton_ = get('replace-button');
      this.replaceButton_.addEventListener('click', () =>
        this.enqueueSearchResults_(true, false),
      );

      this.searchResultsTable_ = get('results-table');
      this.searchResultsTable_.addEventListener('field', e => {
        this.reset_(e.detail.artist, e.detail.album, false /* clearResults */);
      });
      this.searchResultsTable_.addEventListener('check', e => {
        const checked = !!e.detail.count;
        this.appendButton_.disabled = this.insertButton_.disabled = this.replaceButton_.disabled = !checked;
      });

      this.waitingDiv_ = get('waiting');
    }

    resetForTesting() {
      this.reset_(null, null, true /* clearResults */);
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
            (getCurrentTimeSec() - parseInt(this.firstPlayedSelect_.value)),
        );
      }
      if (this.lastPlayedSelect_.value != 0) {
        terms.push(
          'maxLastPlayed=' +
            (getCurrentTimeSec() - parseInt(this.lastPlayedSelect_.value)),
        );
      }

      if (!terms.length) {
        this.dialogManager_.createMessageDialog(
          'Invalid Search',
          'You must supply search terms.',
        );
        return;
      }

      if (this.request_) this.request_.abort();

      this.request_ = new XMLHttpRequest();

      this.request_.onload = () => {
        const req = this.request_;
        if (req.status == 200) {
          if (req.responseText) {
            const songs = eval('(' + req.responseText + ')');
            console.log('Got response with ' + songs.length + ' song(s)');
            songs.forEach(s => {
              if (!s.coverUrl) s.coverUrl = 'images/missing_cover.png';
            });
            this.searchResultsTable_.setSongs(songs);
            this.searchResultsTable_.setAllCheckboxes(true);
            if (appendToQueue) this.enqueueSearchResults_(true, true);
          } else {
            this.dialogManager_.createMessageDialog(
              'Search Failed',
              'Response from server was empty.',
            );
          }
        } else {
          if (req.status && req.responseText) {
            this.dialogManager_.createMessageDialog(
              'Search Failed',
              'Got ' + req.status + ': ' + req.responseText,
            );
          } else {
            this.dialogManager_.createMessageDialog(
              'Search Failed',
              'Missing status in request.',
            );
          }
        }

        this.waitingDiv_.style.display = 'none';
        this.request_ = null;
      };

      this.request_.onerror = e => {
        this.dialogManager_.createMessageDialog(
          'Search Failed',
          'Request to server failed.',
        );
        console.log(e);
      };

      this.waitingDiv_.style.display = 'block';
      const url = 'query?' + terms.join('&');
      console.log('Sending query: ' + url);
      this.request_.open('GET', url, true);
      this.request_.send();
    }

    enqueueSearchResults_(clearFirst, afterCurrent) {
      if (!this.searchResultsTable_.numSongs) return;

      const songs = this.searchResultsTable_.checkedSongs;
      this.dispatchEvent(
        new CustomEvent('enqueue', {
          detail: {
            songs,
            clearFirst,
            afterCurrent,
          },
        }),
      );
      if (songs.length == this.searchResultsTable_.numSongs) {
        this.searchResultsTable_.setSongs([]);
      }
    }

    // Resets all of the fields in the search form. If |newArtist| or |newAlbum|
    // are non-null, the supplied values are used. Also jumps to the top of the
    // page so the form is visible.
    reset_(newArtist, newAlbum, clearResults) {
      const keywords = [];
      const clean = s => {
        s = s.replace(/"/g, '\\"');
        if (s.indexOf(' ') != -1) s = '"' + s + '"';
        return s;
      };
      if (newArtist) keywords.push('artist:' + clean(newArtist));
      if (newAlbum) keywords.push('album:' + clean(newAlbum));

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
      if (clearResults) this.searchResultsTable_.setSongs([]);
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
        this.reset_(null, null, false /* clearResults */);
        this.shuffleCheckbox_.checked = true;
        this.minRatingSelect_.selectedIndex = 3;
      }
      this.submitQuery_(true);
    }

    // Handle a key being pressed in the search form.
    handleFormKeyDown_(e) {
      if (e.keyCode == KeyCodes.ENTER) {
        this.submitQuery_(false);
      } else if (
        e.keyCode == KeyCodes.SPACE ||
        e.keyCode == KeyCodes.LEFT ||
        e.keyCode == KeyCodes.RIGHT ||
        e.keyCode == KeyCodes.SLASH
      ) {
        e.stopPropagation();
      }
    }

    handleUnratedCheckboxChanged_(event) {
      this.minRatingSelect_.disabled = this.unratedCheckbox_.checked;
    }

    handlePresetSelectChanged_(event) {
      if (this.presetSelect_.value == '') return;

      const index = this.presetSelect_.selectedIndex;
      this.reset_(null, null, false /* clearResults */);
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
      if (this.dialogManager_.numDialogs) return;

      if (e.keyCode == KeyCodes.SLASH) {
        this.keywordsInput_.focus();
        e.preventDefault();
        e.stopPropagation();
      }
    }
  },
);

function parseQueryString(text) {
  let terms = [];
  let keywords = [];

  text = text.trim();
  while (text.length > 0) {
    if (
      text.indexOf('artist:') == 0 ||
      text.indexOf('title:') == 0 ||
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
        match[1].split(/[-_+=~!?@#$%^&*()'".,:;]+/).filter(s => s.length),
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
