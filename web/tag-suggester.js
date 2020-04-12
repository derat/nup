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
  background-color: #eee;
  border-radius: 4px;
  box-shadow: 0 1px 2px 1px rgba(0, 0, 0, 0.3);
  display: inline-flex;
  flex-wrap: wrap;
  font-size: 10px;
  font-family: Arial, Helvetica, sans-serif;
  color: black;
  opacity: 0;
  overflow: hidden;
  padding: 4px 0 0 4px; // see margin on div below
  pointer-events: none;
  position: absolute;
  text-overflow: ellipsis;
  z-index: 1;
  -webkit-transition: opacity 200ms ease-out;
  -webkit-user-select: none;
}
#suggestions.shown {
  pointer-events: auto;
  opacity: 1;
  -webkit-transition: opacity 0s;
}
#suggestions div {
  margin: 0 4px 4px 0;
}
#suggestions div:hover {
  color: #666;
  cursor: pointer;
}
</style>
<slot name="text"></slot>
<div id="suggestions"></div>
`);

customElements.define(
  'tag-suggester',
  class extends HTMLElement {
    SUGGESTION_MARGIN = 4;

    constructor() {
      super();

      this.tabAdvancesFocus_ = this.hasAttribute('tab-advances-focus');
      this.words_ = [];

      this.style.display = 'contents';
      this.shadow_ = createShadow(this, template);
      this.suggestionsDiv_ = $('suggestions', this.shadow_);

      const slotElements = this.shadow_
        .querySelector('slot')
        .assignedElements();
      if (slotElements.length != 1) {
        throw new Error('Editable element must be provided via slot');
      }
      this.target_ = slotElements[0];
      this.target_.addEventListener('keydown', e => this.handleKeyDown_(e));
      this.target_.addEventListener('focus', () => {
        this.target_.selectionStart = this.target_.selectionEnd = this.target_.value.length;
      });
      this.target_.spellcheck = false;

      document.addEventListener('click', e => this.hideSuggestions_(), false);
    }

    set words(words) {
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
      const cont = this.suggestionsDiv_;
      while (cont.childNodes.length > 0) cont.removeChild(cont.lastChild);

      for (let i = 0; i < words.length; i++) {
        const word = words[i];
        if (cont.childNodes.length > 0) {
          cont.appendChild(document.createTextNode(' '));
        }
        const item = createElement('div', null, null, word);
        item.addEventListener('click', () => {
          this.hideSuggestions_();
          const parts = this.getTextParts_();
          this.target_.value = parts.before + word + parts.after;
          this.target_.focus();
        });
        cont.appendChild(item);
      }

      // Move the suggestions a bit below the target.
      const offset =
        this.target_.offsetTop +
        this.target_.offsetHeight +
        this.SUGGESTION_MARGIN;
      this.suggestionsDiv_.style.top = `${offset}px`;
      this.suggestionsDiv_.style.left = `${this.target_.offsetLeft}px`;

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
