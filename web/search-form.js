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
  #search-title {
    padding-top: 8px;
    font-weight: bold;
    padding-left: 10px;
    cursor: default;
    -webkit-user-select: none;
  }
  #search-table {
    border: 0;
    margin-top: 5px;
    border-collapse: collapse;
    -webkit-user-select: none;
    white-space: nowrap;
  }
  #search-left {
    width: 4em;
  }
  #search-table td {
    padding-left: 10px;
  }
  #search-table label {
    -webkit-user-select: none;
  }
  #search-table input[type='text'] {
    border: 1px solid #aaa;
    padding-left: 2px;
  }
  #keywords-input,
  #tags-input,
  #minRatingInput {
    padding-right: 17px;
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
  #max-plays-input {
    padding-right: 2px;
    width: 2em;
  }
  #search-buttons {
    padding-top: 5px;
  }
  #results-title {
    font-weight: bold;
    padding-right: 10px;
    cursor: default;
    -webkit-user-select: none;
  }
  #results-controls {
    border-top: 1px solid #888;
    margin-top: 10px;
    padding-top: 10px;
    padding-left: 10px;
    -webkit-user-select: none;
  }
  #results-table {
    margin-top: 10px;
  }
  #waiting {
    position: fixed;
    top: 8px;
    right: 8px;
    background-color: #a00;
    color: white;
    padding: 5px;
    font-size: 11px;
    border-radius: 8px;
    display: none;
  }
</style>

<div id="search-title">Search</div>

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
  <span id="results-title">Results</span>
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

      this.musicPlayer_ = null;
      this.dialogManager_ = null;
      this.currentIndex = -1;
      this.request = null;

      this.style.display = 'block';
      document.body.addEventListener('keydown', e =>
        this.handleBodyKeyDown_(e),
      );

      this.shadow_ = createShadow(this, template);
      const get = id => $(id, this.shadow_);

      this.keywordsInput = get('keywords-input');
      this.keywordsInput.addEventListener('keydown', e =>
        this.handleFormKeyDown(e),
      );
      get('keywords-clear').addEventListener(
        'click',
        () => (this.keywordsInput.value = null),
      );

      this.tagSuggester = get('tags-suggester');
      this.tagsInput = get('tags-input');
      this.tagsInput.addEventListener('keydown', e =>
        this.handleFormKeyDown(e),
      );
      get('tags-clear').addEventListener(
        'click',
        () => (this.tagsInput.value = null),
      );

      this.shuffleCheckbox = get('shuffle-checkbox');
      this.shuffleCheckbox.addEventListener('keydown', e =>
        this.handleFormKeyDown(e),
      );
      this.firstTrackCheckbox = get('first-track-checkbox');
      this.firstTrackCheckbox.addEventListener('keydown', e =>
        this.handleFormKeyDown(e),
      );
      this.unratedCheckbox = get('unrated-checkbox');
      this.unratedCheckbox.addEventListener('keydown', e =>
        this.handleFormKeyDown(e),
      );
      this.unratedCheckbox.addEventListener('change', e =>
        this.handleUnratedCheckboxChanged(e),
      );
      this.minRatingSelect = get('min-rating-select');
      this.maxPlaysInput = get('max-plays-input');
      this.maxPlaysInput.addEventListener('keydown', e =>
        this.handleFormKeyDown(e),
      );
      this.firstPlayedSelect = get('first-played-select');
      this.lastPlayedSelect = get('last-played-select');
      this.presetSelect = get('preset-select');
      this.presetSelect.addEventListener('change', e =>
        this.handlePresetSelectChanged(e),
      );

      this.searchButton = get('search-button');
      this.searchButton.addEventListener('click', () =>
        this.submitQuery(false),
      );
      this.resetButton = get('reset-button');
      this.resetButton.addEventListener('click', () =>
        this.resetSearchForm(null, null, true),
      );
      this.luckyButton = get('lucky-button');
      this.luckyButton.addEventListener('click', () => this.doLuckySearch());

      this.appendButton = get('append-button');
      this.appendButton.addEventListener('click', () =>
        this.enqueueSearchResults(
          false /* clearFirst */,
          false /* afterCurrent */,
        ),
      );
      this.insertButton = get('insert-button');
      this.insertButton.addEventListener('click', () =>
        this.enqueueSearchResults(false, true),
      );
      this.replaceButton = get('replace-button');
      this.replaceButton.addEventListener('click', () =>
        this.enqueueSearchResults(true, false),
      );

      this.searchResultsTable = get('results-table');
      // TODO: Find a better way to do this.
      window.setTimeout(() => {
        this.searchResultsTable.setArtistClickedCallback(artist =>
          this.resetSearchForm(artist, null),
        );
        this.searchResultsTable.setAlbumClickedCallback(album =>
          this.resetSearchForm(null, album),
        );
        this.searchResultsTable.setCheckedSongsChangedCallback(num =>
          this.handleCheckedSongsChanged_(num),
        );
      });

      this.waitingDiv = get('waiting');
    }

    set musicPlayer(player) {
      this.musicPlayer_ = player;
    }
    set dialogManager(manager) {
      this.dialogManager_ = manager;
    }

    resetForTesting() {
      this.resetSearchForm(null, null, true);
    }

    handleTagsUpdated(tags) {
      this.tagSuggester.words = tags;
    }

    parseQueryString(text) {
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

          if (value.length > 0)
            terms.push(key + '=' + encodeURIComponent(value));
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

    submitQuery(appendToQueue) {
      let terms = [];
      if (this.keywordsInput.value.trim()) {
        terms = terms.concat(this.parseQueryString(this.keywordsInput.value));
      }
      if (this.tagsInput.value.trim()) {
        terms.push('tags=' + encodeURIComponent(this.tagsInput.value.trim()));
      }
      if (this.minRatingSelect.value != 0 && !this.unratedCheckbox.checked) {
        terms.push('minRating=' + this.minRatingSelect.value);
      }
      if (this.shuffleCheckbox.checked) terms.push('shuffle=1');
      if (this.firstTrackCheckbox.checked) terms.push('firstTrack=1');
      if (this.unratedCheckbox.checked) terms.push('unrated=1');
      if (!isNaN(parseInt(this.maxPlaysInput.value))) {
        terms.push('maxPlays=' + parseInt(this.maxPlaysInput.value));
      }
      if (this.firstPlayedSelect.value != 0) {
        terms.push(
          'minFirstPlayed=' +
            (getCurrentTimeSec() - parseInt(this.firstPlayedSelect.value)),
        );
      }
      if (this.lastPlayedSelect.value != 0) {
        terms.push(
          'maxLastPlayed=' +
            (getCurrentTimeSec() - parseInt(this.lastPlayedSelect.value)),
        );
      }

      if (!terms.length) {
        this.dialogManager_.createMessageDialog(
          'Invalid Search',
          'You must supply search terms.',
        );
        return;
      }

      if (this.request) this.request.abort();

      this.request = new XMLHttpRequest();

      this.request.onload = () => {
        const req = this.request;
        if (req.status == 200) {
          if (req.responseText) {
            const songs = eval('(' + req.responseText + ')');
            console.log('Got response with ' + songs.length + ' song(s)');
            this.searchResultsTable.updateSongs(songs);
            this.searchResultsTable.setAllCheckboxes(true);
            if (appendToQueue) this.enqueueSearchResults(true, true);
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

        this.waitingDiv.style.display = 'none';
        this.request = null;
      };

      this.request.onerror = e => {
        this.dialogManager_.createMessageDialog(
          'Search Failed',
          'Request to server failed.',
        );
        console.log(e);
      };

      this.waitingDiv.style.display = 'block';
      const url = 'query?' + terms.join('&');
      console.log('Sending query: ' + url);
      this.request.open('GET', url, true);
      this.request.send();
    }

    enqueueSearchResults(clearFirst, afterCurrent) {
      if (!this.searchResultsTable.numSongs) return;

      const songs = this.searchResultsTable.checkedSongs;
      this.musicPlayer_.enqueueSongs(songs, clearFirst, afterCurrent);
      if (songs.length == this.searchResultsTable.numSongs) {
        this.searchResultsTable.updateSongs([]);
      }
    }

    // Reset all of the fields in the search form.  If |artist| or |album| are
    // non-null, the supplied values are used.  Also jumps to the top of the
    // page so the form is visible.
    resetSearchForm(artist, album, clearResults) {
      const keywords = [];
      const clean = s => {
        s = s.replace(/"/g, '\\"');
        if (s.indexOf(' ') != -1) s = '"' + s + '"';
        return s;
      };
      if (artist) keywords.push('artist:' + clean(artist));
      if (album) keywords.push('album:' + clean(album));

      this.keywordsInput.value = keywords.join(' ');
      this.tagsInput.value = null;
      this.shuffleCheckbox.checked = false;
      this.firstTrackCheckbox.checked = false;
      this.unratedCheckbox.checked = false;
      this.minRatingSelect.selectedIndex = 0;
      this.minRatingSelect.disabled = false;
      this.maxPlaysInput.value = null;
      this.firstPlayedSelect.selectedIndex = 0;
      this.lastPlayedSelect.selectedIndex = 0;
      this.presetSelect.selectedIndex = 0;
      if (clearResults) this.searchResultsTable.updateSongs([]);
      this.scrollIntoView();
    }

    // Handles the "I'm Feeling Lucky" button being clicked.
    doLuckySearch() {
      if (
        !this.keywordsInput.value &&
        !this.tagsInput.value &&
        !this.shuffleCheckbox.checked &&
        !this.firstTrackCheckbox.checked &&
        !this.unratedCheckbox.checked &&
        this.minRatingSelect.selectedIndex == 0 &&
        !this.maxPlaysInput.value &&
        this.firstPlayedSelect.selectedIndex == 0 &&
        this.lastPlayedSelect.selectedIndex == 0
      ) {
        this.resetSearchForm(null, null, false);
        this.shuffleCheckbox.checked = true;
        this.minRatingSelect.selectedIndex = 3;
      }
      this.submitQuery(true);
    }

    // Handle a key being pressed in the search form.
    handleFormKeyDown(e) {
      if (e.keyCode == KeyCodes.ENTER) {
        this.submitQuery(false);
      } else if (
        e.keyCode == KeyCodes.SPACE ||
        e.keyCode == KeyCodes.LEFT ||
        e.keyCode == KeyCodes.RIGHT ||
        e.keyCode == KeyCodes.SLASH
      ) {
        e.stopPropagation();
      }
    }

    handleUnratedCheckboxChanged(event) {
      this.minRatingSelect.disabled = this.unratedCheckbox.checked;
    }

    handlePresetSelectChanged(event) {
      if (this.presetSelect.value == '') return;

      const index = this.presetSelect.selectedIndex;
      this.resetSearchForm(null, null, false);
      this.presetSelect.selectedIndex = index;

      let play = false;

      const vals = this.presetSelect.value.split(';');
      for (let i = 0; i < vals.length; i++) {
        const parts = vals[i].split('=');
        if (parts[0] == 'fp') this.firstPlayedSelect.selectedIndex = parts[1];
        else if (parts[0] == 'ft') this.firstTrackCheckbox.checked = !!parts[1];
        else if (parts[0] == 'lp')
          this.lastPlayedSelect.selectedIndex = parts[1];
        else if (parts[0] == 'mr')
          this.minRatingSelect.selectedIndex = parts[1];
        else if (parts[0] == 'play') play = !!parts[1];
        else if (parts[0] == 's') this.shuffleCheckbox.checked = !!parts[1];
        else if (parts[0] == 't') this.tagsInput.value = parts[1];
        else if (parts[0] == 'u') this.unratedCheckbox.checked = !!parts[1];
        else console.log('Unknown preset setting ' + vals[i]);
      }

      // Unfocus the element so that arrow keys or Page Up/Down won't select new
      // presets.
      this.presetSelect.blur();

      this.submitQuery(play);
    }

    handleCheckedSongsChanged_(numChecked) {
      this.appendButton.disabled = this.insertButton.disabled = this.replaceButton.disabled = !numChecked;
    }

    handleBodyKeyDown_(e) {
      if (this.processAccelerator_(e)) {
        e.preventDefault();
        e.stopPropagation();
      }
    }

    processAccelerator_(e) {
      if (this.dialogManager_.numDialogs) return false;

      if (e.keyCode == KeyCodes.SLASH) {
        this.keywordsInput.focus();
        return true;
      }

      return false;
    }
  },
);
