// Copyright 2015 Daniel Erat.
// All rights reserved.

import {
  $,
  createElement,
  createShadow,
  createTemplate,
  KeyCodes,
} from './common.js';

const template = createTemplate(`
<style>
#suggestions {
  z-index: 2;
  font-size: 10px;
  font-family: Arial, Helvetica, sans-serif;
  background-color: #eee;
  box-shadow: 0 1px 2px 1px rgba(0, 0, 0, 0.3);
  color: black;
  padding: 3px;
  border-radius: 6px;
  pointer-events: none;
  opacity: 0;
  -webkit-transition: opacity 200ms ease-out;
  -webkit-user-select: none;
  text-overflow: ellipsis;
  overflow: hidden;
}
#suggestions.shown {
  pointer-events: auto;
  opacity: 1;
  -webkit-transition: opacity 0s;
}
#suggestions span:hover {
  color: #666;
  cursor: pointer;
}
</style>
<div id="suggestions"></div>
`);

customElements.define(
  'tag-suggester',
  class extends HTMLElement {
    constructor() {
      super();

      this.tabAdvancesFocus_ = this.hasAttribute('tab-advances-focus');
      this.words_ = [];

      this.style.display = 'block';
      this.shadow_ = this.attachShadow({mode: 'open'});
      this.shadow_.appendChild(template.content.cloneNode(true));
      this.suggestionsDiv_ = $('suggestions', this.shadow_);

      const targetId = this.getAttribute('target-id');
      this.target_ = $(targetId);
      if (!this.target_) {
        throw new Error(`Unable to find target element "${targetId}"`);
      }
      this.target_.addEventListener(
        'keydown',
        e => this.handleKeyDown_(e),
        false,
      );
      this.target_.addEventListener(
        'focus',
        () => {
          const text = this.target_.value;
          if (text.length > 0 && text[text.length - 1] != ' ') {
            this.target_.value += ' ';
          }
          this.target_.selectionStart = this.target_.selectionEnd = this.target_.value.length;
        },
        false,
      );
      this.target_.spellcheck = false;

      document.addEventListener('click', e => this.hideSuggestions_(), false);
    }

    setWords(words) {
      this.words_ = words.slice(0);
    }

    getTextParts_() {
      const text = this.target_.value;
      const res = {};

      res.wordStart = this.target_.selectionStart;
      while (res.wordStart > 0 && text[res.wordStart - 1] != ' ')
        res.wordStart--;

      res.wordEnd = this.target_.selectionStart;
      while (res.wordEnd < text.length && text[res.wordEnd] != ' ')
        res.wordEnd++;

      res.word = text.substring(res.wordStart, res.wordEnd);
      res.before = text.substring(0, res.wordStart);
      res.after = text.substring(res.wordEnd, text.length);

      return res;
    }

    handleKeyDown_(e) {
      this.hideSuggestions_();

      if (e.keyCode != KeyCodes.TAB) return;

      const parts = this.getTextParts_();
      if (parts.word.length == 0 && this.tabAdvancesFocus_) return;

      const matchingWords = findWordsWithPrefix(this.words_, parts.word);

      if (matchingWords.length == 1) {
        const word = matchingWords[0];
        if (word == parts.word && this.tabAdvancesFocus_) return;

        const text =
          parts.before + word + (parts.after.length == 0 ? ' ' : parts.after);
        this.target_.value = text;

        let nextWordStart = parts.wordStart + word.length;
        while (nextWordStart < text.length && text[nextWordStart] == ' ') {
          nextWordStart++;
        }
        this.target_.selectionStart = this.target_.selectionEnd = nextWordStart;
      } else if (matchingWords.length > 1) {
        let longestSharedPrefix = parts.word;
        for (
          let length = parts.word.length + 1;
          length <= matchingWords[0].length;
          ++length
        ) {
          const newPrefix = matchingWords[0].substring(0, length);
          if (
            findWordsWithPrefix(matchingWords, newPrefix).length ==
            matchingWords.length
          ) {
            longestSharedPrefix = newPrefix;
          } else break;
        }

        this.target_.value = parts.before + longestSharedPrefix + parts.after;
        this.target_.selectionStart = this.target_.selectionEnd =
          parts.before.length + longestSharedPrefix.length;
        this.showSuggestions_(matchingWords.sort());
      }

      e.preventDefault();
      e.stopPropagation();
    }

    showSuggestions_(words) {
      const div = this.suggestionsDiv_;
      while (div.childNodes.length > 0) div.removeChild(div.lastChild);

      for (let i = 0; i < words.length; i++) {
        const word = words[i];
        if (div.childNodes.length > 0) {
          div.appendChild(document.createTextNode(' '));
        }
        const span = document.createElement('span');
        span.innerText = word;
        span.addEventListener(
          'click',
          () => {
            this.hideSuggestions_();
            const parts = this.getTextParts_();
            this.target_.value = parts.before + word + parts.after;
            this.target_.focus();
          },
          false,
        );
        div.appendChild(span);
      }

      this.suggestionsDiv_.classList.add('shown');
    }

    hideSuggestions_() {
      this.suggestionsDiv_.classList.remove('shown');
    }
  },
);

function findWordsWithPrefix(words, prefix) {
  const matchingWords = [];
  for (let i = 0; i < words.length; ++i) {
    if (words[i].indexOf(prefix) == 0) matchingWords.push(words[i]);
  }
  return matchingWords;
}