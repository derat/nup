// Copyright 2015 Daniel Erat.
// All rights reserved.

import { $, createElement, createShadow, createTemplate } from './common.js';

const template = createTemplate(`
<style>
  :host {
    display: contents;
  }
  #suggestions {
    background-color: var(--suggestions-color);
    border-radius: 4px;
    box-shadow: 0 1px 2px 1px rgba(0, 0, 0, 0.3);
    color: var(--text-color);
    display: none;
    flex-wrap: wrap;
    font-family: Arial, Helvetica, sans-serif;
    font-size: 10px;
    overflow: hidden;
    padding: 6px 0 0 8px; /* see margin on div below */
    position: absolute;
    text-overflow: ellipsis;
    z-index: 1;
  }
  #suggestions.shown {
    display: inline-flex;
  }
  #suggestions div {
    margin: 0 8px 4px 0;
  }
  #suggestions div:hover {
    color: var(--text-hover-color);
    cursor: pointer;
  }
</style>
<slot name="text"></slot>
<div id="suggestions"></div>
`);

// This class is exported so it can be used as a type.
export class TagSuggester extends HTMLElement {
  static SUGGESTION_MARGIN_ = 4;

  tabAdvancesFocus_: boolean;
  words_: string[];
  shadow_: ShadowRoot;
  suggestionsDiv_: HTMLElement;
  target_: HTMLInputElement | HTMLTextAreaElement | null;

  constructor() {
    super();

    this.tabAdvancesFocus_ = this.hasAttribute('tab-advances-focus');
    this.words_ = [];

    this.shadow_ = createShadow(this, template);
    this.suggestionsDiv_ = $('suggestions', this.shadow_);
    this.target_ = null;
  }

  connectedCallback() {
    const slotElements = this.shadow_.querySelector('slot').assignedElements();
    if (slotElements.length !== 1) {
      throw new Error('Editable element must be provided via slot');
    }
    this.target_ = slotElements[0] as HTMLInputElement | HTMLTextAreaElement;
    this.target_.addEventListener('keydown', this.onKeyDown_);
    this.target_.addEventListener('focus', this.onFocus_);
    this.target_.spellcheck = false;

    document.addEventListener('click', this.onDocumentClick_);
  }

  disconnectedCallback() {
    this.target_?.removeEventListener('keydown', this.onKeyDown_);
    this.target_?.removeEventListener('focus', this.onFocus_);
    this.target_ = null;

    document.removeEventListener('click', this.onDocumentClick_);
  }

  set words(words: string[]) {
    this.words_ = words.slice(0);
  }

  // Breaks |target_|'s text up into the current word (based on the caret
  // position) and the parts before and after it.
  getTextParts_() {
    const text = this.target_.value;
    const caret = this.target_.selectionStart;

    let start = caret;
    while (start > 0 && text[start - 1] !== ' ') start--;

    let end = caret;
    while (end < text.length && text[end] !== ' ') end++;

    // If we're in the middle of the word and the part to the right of the
    // caret is already a known word, then just use the part to the left of
    // the caret as the word that we're trying to complete.
    if (caret < end) {
      const rest = text.slice(caret, end);
      if (this.words_.includes(rest)) end = caret;
    }

    return {
      before: text.slice(0, start),
      word: text.slice(start, end),
      after: text.slice(end, text.length),
    };
  }

  onDocumentClick_ = () => {
    this.hideSuggestions_();
  };

  onFocus_ = () => {
    // Preserve the caret position but remove the selection. Otherwise,
    // tabbing to the field selects its contents, which makes it too easy to
    // accidentally clear it.
    this.target_.selectionStart = this.target_.selectionEnd;
  };

  onKeyDown_ = (e: KeyboardEvent) => {
    this.hideSuggestions_();

    if (e.key !== 'Tab' || e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) {
      return;
    }

    const parts = this.getTextParts_();
    if (!parts.word.length && this.tabAdvancesFocus_) return;

    const matches = this.findMatches_(parts.word);
    if (matches.length === 1) {
      // If there's a single match, use it.
      const word = matches[0];
      const old = this.target_.value;
      const text = parts.before + word + prependSpace(parts.after);
      this.target_.value = text;

      // Bail out before stopping the event if we want tab to advance the
      // focus and we didn't do anything.
      if (text === old && this.tabAdvancesFocus_) return;

      // Move the caret to the beginning of the next word.
      let next = parts.before.length + word.length;
      while (next < text.length && text[next] === ' ') next++;
      this.target_.selectionStart = this.target_.selectionEnd = next;
    } else if (matches.length > 1) {
      // Complete as much of the word as we can and show suggestions.
      let prefix = parts.word;
      for (let len = parts.word.length + 1; len <= matches[0].length; ++len) {
        const newPrefix = matches[0].slice(0, len);
        if (this.findMatches_(newPrefix).length !== matches.length) break;
        prefix = newPrefix;
      }

      this.target_.value =
        parts.before + prefix + prependSpace(parts.after, false);
      this.target_.selectionStart = this.target_.selectionEnd =
        parts.before.length + prefix.length;
      this.showSuggestions_(matches.sort());
    }

    e.preventDefault();
    e.stopPropagation();
  };

  showSuggestions_(words: string[]) {
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
        this.target_.value = parts.before + word + prependSpace(parts.after);
        this.target_.focus();
      });
      cont.appendChild(item);
    }

    // Move the suggestions a bit below the target.
    const offset =
      this.target_.offsetTop +
      this.target_.offsetHeight +
      TagSuggester.SUGGESTION_MARGIN_;
    this.suggestionsDiv_.style.top = `${offset}px`;
    this.suggestionsDiv_.style.left = `${this.target_.offsetLeft}px`;

    this.suggestionsDiv_.classList.add('shown');
  }

  hideSuggestions_() {
    this.suggestionsDiv_.classList.remove('shown');
  }

  findMatches_(prefix: string) {
    return this.words_.filter((w) => w.startsWith(prefix));
  }
}

customElements.define('tag-suggester', TagSuggester);

// Adds a space to the beginning of |s| if it doesn't already start with one.
// If |ifEmpty| is false, doesn't add spaces to empty strings.
const prependSpace = (s: string, ifEmpty = true) =>
  (s.startsWith(' ') || (s === '' && !ifEmpty) ? '' : ' ') + s;
