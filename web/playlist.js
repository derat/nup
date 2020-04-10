// Copyright 2010 Daniel Erat.
// All rights reserved.

import {$, getCurrentTimeSec, KeyCodes} from './common.js';

export default class Playlist {
  constructor(player) {
    this.player = player;
    this.currentIndex = -1;
    this.request = null;

    this.playlistTable = $('playlistTable');
    this.playlistTable.setArtistClickedCallback(artist =>
      this.resetSearchForm(artist, null, false),
    );
    this.playlistTable.setAlbumClickedCallback(album =>
      this.resetSearchForm(null, album, false),
    );

    this.rightPane = $('rightPane');
    this.keywordsInput = $('keywordsInput');
    this.keywordsClearButton = $('keywordsClearButton');
    this.tagsInput = $('tagsInput');
    this.tagsClearButton = $('tagsClearButton');
    this.shuffleCheckbox = $('shuffleCheckbox');
    this.firstTrackCheckbox = $('firstTrackCheckbox');
    this.unratedCheckbox = $('unratedCheckbox');
    this.minRatingSelect = $('minRatingSelect');
    this.maxPlaysInput = $('maxPlaysInput');
    this.firstPlayedSelect = $('firstPlayedSelect');
    this.lastPlayedSelect = $('lastPlayedSelect');
    this.presetSelect = $('presetSelect');

    this.searchButton = $('searchButton');
    this.resetButton = $('resetButton');
    this.luckyButton = $('luckyButton');

    this.appendButton = $('appendButton');
    this.insertButton = $('insertButton');
    this.replaceButton = $('replaceButton');

    this.searchResultsTable = $('searchResultsTable');
    this.searchResultsTable.setArtistClickedCallback(artist =>
      this.resetSearchForm(artist, null, false),
    );
    this.searchResultsTable.setAlbumClickedCallback(album =>
      this.resetSearchForm(null, album, false),
    );
    this.searchResultsTable.setCheckedSongsChangedCallback(() =>
      this.handleCheckedSongsChanged_(),
    );

    this.waitingDiv = $('waitingDiv');

    this.keywordsInput.addEventListener(
      'keydown',
      e => this.handleFormKeyDown(e),
      false,
    );
    this.keywordsClearButton.addEventListener(
      'click',
      () => (this.keywordsInput.value = null),
      false,
    );
    this.tagsInput.addEventListener(
      'keydown',
      e => this.handleFormKeyDown(e),
      false,
    );
    this.tagsClearButton.addEventListener(
      'click',
      () => (this.tagsInput.value = null),
      false,
    );
    this.shuffleCheckbox.addEventListener(
      'keydown',
      e => this.handleFormKeyDown(e),
      false,
    );
    this.firstTrackCheckbox.addEventListener(
      'keydown',
      e => this.handleFormKeyDown(e),
      false,
    );
    this.unratedCheckbox.addEventListener(
      'keydown',
      e => this.handleFormKeyDown(e),
      false,
    );
    this.unratedCheckbox.addEventListener(
      'change',
      e => this.handleUnratedCheckboxChanged(e),
      false,
    );
    this.maxPlaysInput.addEventListener(
      'keydown',
      e => this.handleFormKeyDown(e),
      false,
    );
    this.presetSelect.addEventListener(
      'change',
      e => this.handlePresetSelectChanged(e),
      false,
    );
    this.searchButton.addEventListener(
      'click',
      () => this.submitQuery(false),
      false,
    );
    this.resetButton.addEventListener(
      'click',
      () => this.resetSearchForm(null, null, true),
      false,
    );
    this.luckyButton.addEventListener(
      'click',
      () => this.doLuckySearch(),
      false,
    );
    this.appendButton.addEventListener(
      'click',
      () =>
        this.enqueueSearchResults(
          false /* clearFirst */,
          false /* afterCurrent */,
        ),
      false,
    );
    this.insertButton.addEventListener(
      'click',
      () => this.enqueueSearchResults(false, true),
      false,
    );
    this.replaceButton.addEventListener(
      'click',
      () => this.enqueueSearchResults(true, false),
      false,
    );

    this.dialogManager = document.dialogManager;

    this.tagSuggester = $('tagsInputSuggester');

    document.body.addEventListener(
      'keydown',
      e => this.handleBodyKeyDown_(e),
      false,
    );

    this.playlistTable.updateSongs(this.player.songs);
    this.handleSongChange(this.player.currentIndex);
  }

  resetForTesting() {
    this.resetSearchForm(null, null, true);
    this.playlistTable.updateSongs([]);
    this.player.hideUpdateDiv(false /* saveChanges */);
    this.player.setSongs([]);
  }

  handleTagsUpdated(tags) {
    this.tagSuggester.setWords(tags);
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
      this.dialogManager.createMessageDialog(
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
          this.dialogManager.createMessageDialog(
            'Search Failed',
            'Response from server was empty.',
          );
        }
      } else {
        if (req.status && req.responseText) {
          this.dialogManager.createMessageDialog(
            'Search Failed',
            'Got ' + req.status + ': ' + req.responseText,
          );
        } else {
          this.dialogManager.createMessageDialog(
            'Search Failed',
            'Missing status in request.',
          );
        }
      }

      this.waitingDiv.style.display = 'none';
      this.request = null;
    };

    this.request.onerror = e => {
      this.dialogManager.createMessageDialog(
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
    if (this.searchResultsTable.getNumSongs() == 0) return;

    const newSongs = clearFirst ? [] : this.player.songs.slice(0);
    let index = afterCurrent
      ? Math.min(this.currentIndex + 1, newSongs.length)
      : newSongs.length;

    const checkedSongs = this.searchResultsTable.getCheckedSongs();
    for (let i = 0; i < checkedSongs.length; i++) {
      newSongs.splice(index++, 0, checkedSongs[i]);
    }

    // If we're replacing the current songs but e.g. the last song was highlighted
    // and is the same for both the old and new lists, make sure that the
    // highlighting on it gets cleared.
    if (clearFirst) this.playlistTable.highlightRow(this.currentIndex, false);

    this.playlistTable.updateSongs(newSongs);
    this.player.setSongs(newSongs);

    if (checkedSongs.length == this.searchResultsTable.getNumSongs()) {
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
    this.rightPane.scrollIntoView();
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
      else if (parts[0] == 'lp') this.lastPlayedSelect.selectedIndex = parts[1];
      else if (parts[0] == 'mr') this.minRatingSelect.selectedIndex = parts[1];
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

  // Handle notification from the player that the current song has changed.
  handleSongChange(index) {
    this.playlistTable.highlightRow(this.currentIndex, false);

    if (index >= 0 && index < this.playlistTable.getNumSongs()) {
      this.playlistTable.highlightRow(index, true);
      this.currentIndex = index;
    } else {
      this.currentIndex = -1;
    }
  }

  handleCheckedSongsChanged_(numChecked) {
    this.appendButton.disabled = this.insertButton.disabled = this.replaceButton.disabled =
      numChecked == 0;
  }

  handleBodyKeyDown_(e) {
    if (this.processAccelerator_(e)) {
      e.preventDefault();
      e.stopPropagation();
    }
  }

  processAccelerator_(e) {
    if (this.dialogManager.getNumDialogs()) return false;

    if (e.keyCode == KeyCodes.SLASH) {
      this.keywordsInput.focus();
      return true;
    }

    return false;
  }
}
