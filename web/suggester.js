// Copyright 2015 Daniel Erat.
// All rights reserved.

class Suggester {
  constructor(textarea, suggestionsDiv, words, tabAdvancesFocus) {
    this.textarea = textarea;
    this.suggestionsDiv = suggestionsDiv;
    this.words = words;
    this.tabAdvancesFocus = tabAdvancesFocus;

    addClassName(this.suggestionsDiv, 'suggestions');

    textarea.addEventListener(
      'keydown',
      this.handleTextareaKeyDown.bind(this),
      false,
    );
    textarea.addEventListener(
      'focus',
      this.handleTextareaFocus.bind(this),
      false,
    );
    textarea.spellcheck = false;

    document.addEventListener(
      'click',
      this.handleDocumentClick.bind(this),
      false,
    );
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
    const text = this.textarea.value;
    const res = {};

    res.wordStart = this.textarea.selectionStart;
    while (res.wordStart > 0 && text[res.wordStart - 1] != ' ') res.wordStart--;

    res.wordEnd = this.textarea.selectionStart;
    while (res.wordEnd < text.length && text[res.wordEnd] != ' ') res.wordEnd++;

    res.word = text.substring(res.wordStart, res.wordEnd);
    res.before = text.substring(0, res.wordStart);
    res.after = text.substring(res.wordEnd, text.length);

    return res;
  }

  handleTextareaKeyDown(e) {
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
      this.textarea.value = text;

      let nextWordStart = parts.wordStart + word.length;
      while (nextWordStart < text.length && text[nextWordStart] == ' ') {
        nextWordStart++;
      }
      this.textarea.selectionStart = this.textarea.selectionEnd = nextWordStart;
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

      this.textarea.value = parts.before + longestSharedPrefix + parts.after;
      this.textarea.selectionStart = this.textarea.selectionEnd =
        parts.before.length + longestSharedPrefix.length;
      this.showSuggestions(matchingWords.sort());
    }

    e.preventDefault();
    e.stopPropagation();
  }

  handleTextareaFocus(e) {
    const text = this.textarea.value;
    if (text.length > 0 && text[text.length - 1] != ' ') {
      this.textarea.value += ' ';
    }
    this.textarea.selectionStart = this.textarea.selectionEnd = this.textarea.value.length;
  }

  handleDocumentClick(e) {
    this.hideSuggestions();
  }

  handleSuggestionClick(word, e) {
    this.hideSuggestions();
    const parts = this.getTextParts();
    this.textarea.value = parts.before + word + parts.after;
    this.textarea.focus();
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
        this.handleSuggestionClick.bind(this, words[i]),
        false,
      );
      div.appendChild(span);
    }

    addClassName(this.suggestionsDiv, 'shown');
  }

  hideSuggestions() {
    removeClassName(this.suggestionsDiv, 'shown');
  }
}
