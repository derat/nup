// Copyright 2015 Daniel Erat.
// All rights reserved.

import {$, createElement, createShadow, KeyCodes} from './common.js';

class TagSuggester extends HTMLElement {
  constructor() {
    super();

    this.style.display = 'block';

    this.tabAdvancesFocus = this.hasAttribute('tab-advances-focus');
    this.words = [];

    this.shadow_ = createShadow(this, 'tag-suggester.css');
    this.suggestionsDiv = createElement('div', 'suggestions', this.shadow_);

    const targetId = this.getAttribute('target-id');
    this.target = $(targetId);
    if (!this.target) {
      throw new Error(`Unable to find target element "${targetId}"`);
    }
    this.target.addEventListener('keydown', e => this.handleKeyDown(e), false);
    this.target.addEventListener('focus', e => this.handleFocus(e), false);
    this.target.spellcheck = false;

    document.addEventListener('click', e => this.handleDocumentClick(e), false);
  }

  setWords(words) {
    this.words = words.slice(0);
  }

  findWordsWithPrefix(words, prefix) {
    const matchingWords = [];
    for (let i = 0; i < words.length; ++i) {
      if (words[i].indexOf(prefix) == 0) matchingWords.push(words[i]);
    }
    return matchingWords;
  }

  getTextParts() {
    const text = this.target.value;
    const res = {};

    res.wordStart = this.target.selectionStart;
    while (res.wordStart > 0 && text[res.wordStart - 1] != ' ') res.wordStart--;

    res.wordEnd = this.target.selectionStart;
    while (res.wordEnd < text.length && text[res.wordEnd] != ' ') res.wordEnd++;

    res.word = text.substring(res.wordStart, res.wordEnd);
    res.before = text.substring(0, res.wordStart);
    res.after = text.substring(res.wordEnd, text.length);

    return res;
  }

  handleKeyDown(e) {
    this.hideSuggestions();

    if (e.keyCode != KeyCodes.TAB) return;

    const parts = this.getTextParts();
    if (parts.word.length == 0 && this.tabAdvancesFocus) return;

    const matchingWords = this.findWordsWithPrefix(this.words, parts.word);

    if (matchingWords.length == 1) {
      const word = matchingWords[0];
      if (word == parts.word && this.tabAdvancesFocus) return;

      const text =
        parts.before + word + (parts.after.length == 0 ? ' ' : parts.after);
      this.target.value = text;

      let nextWordStart = parts.wordStart + word.length;
      while (nextWordStart < text.length && text[nextWordStart] == ' ') {
        nextWordStart++;
      }
      this.target.selectionStart = this.target.selectionEnd = nextWordStart;
    } else if (matchingWords.length > 1) {
      let longestSharedPrefix = parts.word;
      for (
        let length = parts.word.length + 1;
        length <= matchingWords[0].length;
        ++length
      ) {
        const newPrefix = matchingWords[0].substring(0, length);
        if (
          this.findWordsWithPrefix(matchingWords, newPrefix).length ==
          matchingWords.length
        ) {
          longestSharedPrefix = newPrefix;
        } else break;
      }

      this.target.value = parts.before + longestSharedPrefix + parts.after;
      this.target.selectionStart = this.target.selectionEnd =
        parts.before.length + longestSharedPrefix.length;
      this.showSuggestions(matchingWords.sort());
    }

    e.preventDefault();
    e.stopPropagation();
  }

  handleFocus(e) {
    const text = this.target.value;
    if (text.length > 0 && text[text.length - 1] != ' ') {
      this.target.value += ' ';
    }
    this.target.selectionStart = this.target.selectionEnd = this.target.value.length;
  }

  handleDocumentClick(e) {
    this.hideSuggestions();
  }

  handleSuggestionClick(word, e) {
    this.hideSuggestions();
    const parts = this.getTextParts();
    this.target.value = parts.before + word + parts.after;
    this.target.focus();
  }

  showSuggestions(words) {
    const div = this.suggestionsDiv;
    while (div.childNodes.length > 0) div.removeChild(div.lastChild);

    for (let i = 0; i < words.length; i++) {
      if (div.childNodes.length > 0) {
        div.appendChild(document.createTextNode(' '));
      }
      const span = document.createElement('span');
      span.innerText = words[i];
      span.addEventListener(
        'click',
        e => this.handleSuggestionClick(words[i], e),
        false,
      );
      div.appendChild(span);
    }

    this.suggestionsDiv.classList.add('shown');
  }

  hideSuggestions() {
    this.suggestionsDiv.classList.remove('shown');
  }
}

customElements.define('tag-suggester', TagSuggester);
