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
  static #SUGGESTION_MARGIN = 4;

  #tabAdvancesFocus = this.hasAttribute('tab-advances-focus');
  #words: string[] = [];
  #shadow = createShadow(this, template);
  #suggestionsDiv = $('suggestions', this.#shadow);
  #target: HTMLInputElement | HTMLTextAreaElement | null = null;

  connectedCallback() {
    const slotElements = this.#shadow.querySelector('slot')?.assignedElements();
    if (slotElements?.length !== 1) {
      throw new Error('Editable element must be provided via slot');
    }
    this.#target = slotElements[0] as HTMLInputElement | HTMLTextAreaElement;
    this.#target.addEventListener(
      'keydown',
      this.#onKeyDown as EventListenerOrEventListenerObject
    );
    this.#target.addEventListener('focus', this.#onFocus);
    this.#target.spellcheck = false;

    document.addEventListener('click', this.#onDocumentClick);
  }

  disconnectedCallback() {
    this.#target?.removeEventListener(
      'keydown',
      this.#onKeyDown as EventListenerOrEventListenerObject
    );
    this.#target?.removeEventListener('focus', this.#onFocus);
    this.#target = null;

    document.removeEventListener('click', this.#onDocumentClick);
  }

  set words(words: string[]) {
    this.#words = words.slice(0);
  }

  // Breaks |#target|'s text up into the current word (based on the caret
  // position) and the parts before and after it.
  #getTextParts() {
    const text = this.#target!.value;
    const caret = this.#target!.selectionStart ?? 0;

    let start = caret;
    while (start > 0 && text[start - 1] !== ' ') start--;

    let end = caret;
    while (end < text.length && text[end] !== ' ') end++;

    // If we're in the middle of the word and the part to the right of the
    // caret is already a known word, then just use the part to the left of
    // the caret as the word that we're trying to complete.
    if (caret < end) {
      const rest = text.slice(caret, end);
      if (this.#words.includes(rest)) end = caret;
    }

    return {
      before: text.slice(0, start),
      word: text.slice(start, end),
      after: text.slice(end, text.length),
    };
  }

  #onDocumentClick = () => {
    this.#hideSuggestions();
  };

  #onFocus = () => {
    // Preserve the caret position but remove the selection. Otherwise,
    // tabbing to the field selects its contents, which makes it too easy to
    // accidentally clear it.
    this.#target!.selectionStart = this.#target!.selectionEnd;
  };

  #onKeyDown = (e: KeyboardEvent) => {
    this.#hideSuggestions();

    if (e.key !== 'Tab' || e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) {
      return;
    }

    const parts = this.#getTextParts();
    if (!parts.word.length && this.#tabAdvancesFocus) return;

    const matches = this.#findMatches(parts.word);
    if (matches.length === 1) {
      // If there's a single match, use it.
      const word = matches[0];
      const old = this.#target!.value;
      const text = parts.before + word + prependSpace(parts.after);
      this.#target!.value = text;

      // Bail out before stopping the event if we want tab to advance the
      // focus and we didn't do anything.
      if (text === old && this.#tabAdvancesFocus) return;

      // Move the caret to the beginning of the next word.
      let next = parts.before.length + word.length;
      while (next < text.length && text[next] === ' ') next++;
      this.#target!.selectionStart = this.#target!.selectionEnd = next;
    } else if (matches.length > 1) {
      // Complete as much of the word as we can and show suggestions.
      let prefix = parts.word;
      for (let len = parts.word.length + 1; len <= matches[0].length; ++len) {
        const newPrefix = matches[0].slice(0, len);
        if (this.#findMatches(newPrefix).length !== matches.length) break;
        prefix = newPrefix;
      }

      this.#target!.value =
        parts.before + prefix + prependSpace(parts.after, false);
      this.#target!.selectionStart = this.#target!.selectionEnd =
        parts.before.length + prefix.length;
      this.#showSuggestions(matches.sort());
    }

    e.preventDefault();
    e.stopPropagation();
  };

  #showSuggestions(words: string[]) {
    const cont = this.#suggestionsDiv;
    while (cont.childNodes.length > 0) cont.removeChild(cont.lastChild!);

    for (let i = 0; i < words.length; i++) {
      const word = words[i];
      if (cont.childNodes.length > 0) {
        cont.appendChild(document.createTextNode(' '));
      }
      const item = createElement('div', null, null, word);
      item.addEventListener('click', () => {
        this.#hideSuggestions();
        const parts = this.#getTextParts();
        this.#target!.value = parts.before + word + prependSpace(parts.after);
        this.#target!.focus();
      });
      cont.appendChild(item);
    }

    // Move the suggestions a bit below the target.
    const offset =
      this.#target!.offsetTop +
      this.#target!.offsetHeight +
      TagSuggester.#SUGGESTION_MARGIN;
    this.#suggestionsDiv.style.top = `${offset}px`;
    this.#suggestionsDiv.style.left = `${this.#target!.offsetLeft}px`;

    this.#suggestionsDiv.classList.add('shown');
  }

  #hideSuggestions() {
    this.#suggestionsDiv.classList.remove('shown');
  }

  #findMatches(prefix: string) {
    return this.#words.filter((w) => w.startsWith(prefix));
  }
}

customElements.define('tag-suggester', TagSuggester);

// Adds a space to the beginning of |s| if it doesn't already start with one.
// If |ifEmpty| is false, doesn't add spaces to empty strings.
const prependSpace = (s: string, ifEmpty = true) =>
  (s.startsWith(' ') || (s === '' && !ifEmpty) ? '' : ' ') + s;
