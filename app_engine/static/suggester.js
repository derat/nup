// Copyright 2015 Daniel Erat.
// All rights reserved.

function Suggester(textarea, suggestionsDiv, words) {
  this.textarea = textarea;
  this.suggestionsDiv = suggestionsDiv;
  this.words = words;

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

Suggester.prototype.handleTextareaKeyDown = function(e) {
  this.hideSuggestions();

  if (e.keyCode != KeyCodes.TAB)
    return;

  var text = this.textarea.value;

  var wordStart = this.textarea.selectionStart;
  while (wordStart > 0 && text[wordStart - 1] != ' ')
    wordStart--;

  var wordEnd = this.textarea.selectionStart;
  while (wordEnd < text.length && text[wordEnd] != ' ')
    wordEnd++;

  var before = text.substring(0, wordStart);
  var after = text.substring(wordEnd, text.length);
  var wordPrefix = text.substring(wordStart, wordEnd);
  var matchingWords = this.findWordsWithPrefix(this.words, wordPrefix);

  if (matchingWords.length == 1) {
    var word = matchingWords[0];
    text = before + word + (after.length == 0 ? ' ' : after);
    this.textarea.value = text;

    var nextWordStart = wordStart + word.length;
    while (nextWordStart < text.length && text[nextWordStart] == ' ')
      nextWordStart++;
    this.textarea.selectionStart = this.textarea.selectionEnd = nextWordStart;
  } else if (matchingWords.length > 1) {
    var longestSharedPrefix = wordPrefix;
    for (var length = wordPrefix.length + 1; length <= matchingWords[0].length; ++length) {
      var newPrefix = matchingWords[0].substring(0, length);
      if (this.findWordsWithPrefix(matchingWords, newPrefix).length == matchingWords.length)
        longestSharedPrefix = newPrefix;
      else
        break;
    }

    this.textarea.value = before + longestSharedPrefix + after;
    this.textarea.selectionStart = this.textarea.selectionEnd = before.length + longestSharedPrefix.length;
    this.showSuggestions(matchingWords);
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

Suggester.prototype.showSuggestions = function(words) {
  this.suggestionsDiv.innerText = words.sort().join(' ');
  this.suggestionsDiv.className = 'shown';
};

Suggester.prototype.hideSuggestions = function() {
  this.suggestionsDiv.className = '';
};
