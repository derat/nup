// Copyright 2015 Daniel Erat.
// All rights reserved.

function Suggester(textarea, suggestionsDiv, words) {
  this.textarea = textarea;
  this.suggestionsDiv = suggestionsDiv;
  this.words = words;

  addClassName(this.suggestionsDiv, 'suggestions');

  textarea.addEventListener('keydown', this.handleTextareaKeyDown.bind(this), false);
  textarea.addEventListener('focus', this.handleTextareaFocus.bind(this), false);
  textarea.addEventListener('blur', this.handleTextareaBlur.bind(this), false);
  textarea.spellcheck = false;
}

Suggester.prototype.setWords = function(words) {
  this.words = words.slice(0);
};

Suggester.prototype.findWordsWithPrefix = function(words, prefix) {
  var matchingWords = [];
  for (var i = 0; i < words.length; ++i) {
    if (words[i].indexOf(prefix) == 0)
      matchingWords.push(words[i]);
  }
  return matchingWords;
};

Suggester.prototype.getTextParts = function() {
  var text = this.textarea.value;
  var res = {};

  res.wordStart = this.textarea.selectionStart;
  while (res.wordStart > 0 && text[res.wordStart - 1] != ' ')
    res.wordStart--;

  res.wordEnd = this.textarea.selectionStart;
  while (res.wordEnd < text.length && text[res.wordEnd] != ' ')
    res.wordEnd++;

  res.word = text.substring(res.wordStart, res.wordEnd);
  res.before = text.substring(0, res.wordStart);
  res.after = text.substring(res.wordEnd, text.length);

  return res;
}

Suggester.prototype.handleTextareaKeyDown = function(e) {
  this.hideSuggestions();

  if (e.keyCode != KeyCodes.TAB)
    return;

  var parts = this.getTextParts();
  var matchingWords = this.findWordsWithPrefix(this.words, parts.word);

  if (matchingWords.length == 1) {
    var word = matchingWords[0];
    var text = parts.before + word + (parts.after.length == 0 ? ' ' : parts.after);
    this.textarea.value = text;
    var nextWordStart = parts.wordStart + word.length;
    while (nextWordStart < text.length && text[nextWordStart] == ' ')
      nextWordStart++;
    this.textarea.selectionStart = this.textarea.selectionEnd = nextWordStart;
  } else if (matchingWords.length > 1) {
    var longestSharedPrefix = parts.word;
    for (var length = parts.word.length + 1; length <= matchingWords[0].length; ++length) {
      var newPrefix = matchingWords[0].substring(0, length);
      if (this.findWordsWithPrefix(matchingWords, newPrefix).length == matchingWords.length)
        longestSharedPrefix = newPrefix;
      else
        break;
    }

    this.textarea.value = parts.before + longestSharedPrefix + parts.after;
    this.textarea.selectionStart = this.textarea.selectionEnd = parts.before.length + longestSharedPrefix.length;
    this.showSuggestions(matchingWords.sort());
  }

  e.preventDefault();
  e.stopPropagation();
};

Suggester.prototype.handleTextareaFocus = function(e) {
  var text = this.textarea.value;
  if (text.length > 0 && text[text.length - 1] != ' ')
    this.textarea.value += ' ';
  this.textarea.selectionStart = this.textarea.selectionEnd = this.textarea.value.length;
};


Suggester.prototype.handleTextareaBlur = function(e) {
  this.hideSuggestions();
};

Suggester.prototype.handleSuggestionClicked = function(word, e) {
  var parts = this.getTextParts();
  this.textarea.value = parts.before + word + parts.after;
  this.textarea.focus();
};

Suggester.prototype.showSuggestions = function(words) {
  var div = this.suggestionsDiv;
  while (div.childNodes.length > 0)
    div.removeChild(div.lastChild);

  for (var i = 0; i < words.length; i++) {
    if (div.childNodes.length > 0)
      div.appendChild(document.createTextNode(' '));
    var span = document.createElement('span');
    span.innerText = words[i];
    span.addEventListener('click', this.handleSuggestionClicked.bind(this, words[i]), false);
    div.appendChild(span);
  }

  addClassName(this.suggestionsDiv, 'shown');
};

Suggester.prototype.hideSuggestions = function() {
  removeClassName(this.suggestionsDiv, 'shown');
};
