// Copyright 2022 Daniel Erat.
// All rights reserved.

import { $, clamp, createTemplate } from './common.js';
import { createDialog } from './dialog.js';

const template = createTemplate(`
<style>
  #close-icon {
    cursor: pointer;
    position: absolute;
    right: 5px;
    top: 5px;
  }
  #rating {
    font-family: var(--icon-font-family);
    font-size: 16px;
  }
  #rating a {
    color: var(--text-color);
    cursor: pointer;
    display: inline-block;
    min-width: 17px; /* black and white stars have different sizes :-/ */
    opacity: 0.6;
  }
  #rating a:hover {
    opacity: 0.9;
  }
  #tags-textarea {
    font-family: Arial, Helvetica, sans-serif;
    height: 48px;
    margin-bottom: -4px;
    margin-top: 8px;
    resize: none;
    width: 220px;
  }
  #tag-suggester {
    bottom: 52px;
    left: 4px;
    max-height: 26px;
    max-width: 210px;
    position: absolute;
  }
</style>

<span id="close-icon" class="x-icon" title="Close"></span>
<div>
  Rating:
  <span id="rating" tabindex="0">
    <a></a><a></a><a></a><a></a><a></a>
  </span>
</div>
<tag-suggester id="tag-suggester">
  <textarea id="tags-textarea" slot="text" placeholder="Tags"></textarea>
</tag-suggester>
`);

// UpdateDialog displays a dialog to update a song's rating and tags.
export default class UpdateDialog {
  // |song| is the song to update, and |tags| is an array of available tags.
  // When the dialog is closed, |closeCallback| is invoked with the updated
  // rating (null if unchanged) and an array containing the updated tags (null
  // if unchanged).
  constructor(song, tags, closeCallback) {
    this.song_ = song;
    this.tags_ = tags;
    this.closeCallback_ = closeCallback;
    this.rating_ = -1; // rating set in dialog
    this.dialog_ = createDialog(template, 'update');

    // This sucks, but I don't want to put this styling in index.html.
    this.dialog_.style.borderRadius = '4px';
    this.dialog_.style.margin = 0; // needed to avoid centering
    this.dialog_.style.padding = '8px';
    this.dialog_.style.left = this.dialog_.style.top = getComputedStyle(
      this.dialog_
    ).getPropertyValue('--margin');
    this.dialog_.style.position = 'absolute';

    const shadow = this.dialog_.firstChild.shadowRoot;
    const get = (id) => $(id, shadow);

    get('close-icon').addEventListener('click', () => this.close(true));
    get('tag-suggester').words = tags;

    this.ratingSpan_ = get('rating');
    this.ratingSpan_.addEventListener('keydown', this.onRatingSpanKeyDown_);
    for (let i = 1; i <= 5; i++) {
      const anchor = this.ratingSpan_.childNodes[i - 1];
      const rating = i;
      anchor.addEventListener('click', () => this.setRating_(rating));
    }
    this.setRating_(song.rating);

    this.tagsTextarea_ = get('tags-textarea');
    this.tagsTextarea_.value = song.tags.length
      ? song.tags.sort().join(' ') + ' ' // append space to ease editing
      : '';
    this.tagsTextarea_.selectionStart = this.tagsTextarea_.selectionEnd =
      this.tagsTextarea_.value.length;

    document.body.addEventListener('keydown', this.onBodyKeyDown_);
  }

  focusRating() {
    this.ratingSpan_.focus();
  }
  focusTags() {
    this.tagsTextarea_.focus();
  }

  close(save) {
    document.body.removeEventListener('keydown', this.onBodyKeyDown_);
    this.dialog_.close();

    let rating = null;
    let tags = null;

    if (save) {
      if (this.rating_ !== this.song_.rating) rating = this.rating_;

      const rawTags = this.tagsTextarea_.value.trim().split(/\s+/);
      tags = [];
      for (let i = 0; i < rawTags.length; ++i) {
        const tag = rawTags[i].toLowerCase();
        if (tag === '') continue;
        if (this.tags_.includes(tag) || this.song_.tags.includes(tag)) {
          tags.push(tag);
        } else if (tag[0] === '+' && tag.length > 1) {
          tags.push(tag.substring(1));
        } else {
          console.log(`Skipping unknown tag "${tag}"`);
        }
      }
      // Remove duplicates.
      tags = tags
        .sort()
        .filter((item, pos, self) => self.indexOf(item) === pos);
      if (tags.join(' ') === this.song_.tags.sort().join(' ')) tags = null;
    }

    this.closeCallback_(rating, tags);
  }

  setRating_(rating) {
    this.rating_ = rating;
    for (let i = 1; i <= 5; i++) {
      this.ratingSpan_.childNodes[i - 1].innerText = i <= rating ? '★' : '☆';
    }
  }

  onBodyKeyDown_ = (e) => {
    if (e.key === 'Enter') {
      this.close(true);
    } else if (e.key === 'Escape') {
      this.close(false);
    } else if (e.altKey && e.key === 'r') {
      this.focusRating();
    } else if (e.altKey && e.key === 't') {
      this.focusTags();
    }
  };

  onRatingSpanKeyDown_ = (e) => {
    if (['0', '1', '2', '3', '4', '5'].includes(e.key)) {
      this.setRating_(parseInt(e.key));
      e.preventDefault();
      e.stopPropagation();
    } else if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
      const rating = this.rating_ + (e.key === 'ArrowLeft' ? -1 : 1);
      this.setRating_(clamp(rating, 0, 5));
      e.preventDefault();
      e.stopPropagation();
    }
  };
}
