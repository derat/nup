// Copyright 2022 Daniel Erat.
// All rights reserved.

import { $, clamp, createTemplate } from './common.js';
import { createDialog } from './dialog.js';
import type { TagSuggester } from './tag-suggester.js';

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
  #song: Song;
  #tags: string[]; // all tags known by server
  #callback: UpdateCallback;
  #rating = -1; // rating set in dialog
  #dialog = createDialog(template, 'update');
  #shadow = this.#dialog.firstElementChild!.shadowRoot!;
  #ratingSpan = $('rating', this.#shadow);
  #tagsTextarea = $('tags-textarea', this.#shadow) as HTMLTextAreaElement;

  // |song| is the song to update, and |tags| is an array of available tags.
  // When the dialog is closed, |callback| is invoked with the updated rating
  // (null if unchanged) and an array containing the updated tags (null if
  // unchanged).
  constructor(song: Song, tags: string[], callback: UpdateCallback) {
    this.#song = song;
    this.#tags = tags;
    this.#callback = callback;

    // This sucks, but I don't want to put this styling in index.html.
    this.#dialog.style.borderRadius = '4px';
    this.#dialog.style.margin = '0'; // needed to avoid centering
    this.#dialog.style.padding = '8px';
    this.#dialog.style.left = this.#dialog.style.top = getComputedStyle(
      this.#dialog
    ).getPropertyValue('--margin');
    this.#dialog.style.position = 'absolute';

    $('close-icon', this.#shadow).addEventListener('click', () =>
      this.close(true)
    );
    ($('tag-suggester', this.#shadow) as TagSuggester).words = tags;

    this.#ratingSpan.addEventListener('keydown', this.#onRatingSpanKeyDown);
    for (let i = 1; i <= 5; i++) {
      const anchor = this.#ratingSpan.children[i - 1];
      const rating = i;
      anchor.addEventListener('click', () => this.#setRating(rating));
    }
    this.#setRating(song.rating);

    this.#tagsTextarea.value = song.tags.length
      ? song.tags.sort().join(' ') + ' ' // append space to ease editing
      : '';
    this.#tagsTextarea.selectionStart = this.#tagsTextarea.selectionEnd =
      this.#tagsTextarea.value.length;

    document.body.addEventListener('keydown', this.#onBodyKeyDown);
  }

  focusRating() {
    this.#ratingSpan.focus();
  }
  focusTags() {
    this.#tagsTextarea.focus();
  }

  close(save: boolean) {
    document.body.removeEventListener('keydown', this.#onBodyKeyDown);
    this.#dialog.close();

    let rating = null;
    let tags = null;

    if (save) {
      if (this.#rating !== this.#song.rating) rating = this.#rating;

      const rawTags = this.#tagsTextarea.value.trim().split(/\s+/);
      tags = [];
      for (let i = 0; i < rawTags.length; ++i) {
        const tag = rawTags[i].toLowerCase();
        if (tag === '') continue;
        if (this.#tags.includes(tag) || this.#song.tags.includes(tag)) {
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
      if (tags.join(' ') === this.#song.tags.sort().join(' ')) tags = null;
    }

    this.#callback(rating, tags);
  }

  #setRating(rating: number) {
    this.#rating = rating;
    for (let i = 1; i <= 5; i++) {
      const a = this.#ratingSpan.children[i - 1] as HTMLElement;
      a.innerText = i <= rating ? '★' : '☆';
    }
  }

  #onBodyKeyDown = (e: KeyboardEvent) => {
    // Stop the event if we're closing the dialog, since it could be handled
    // again by other components now that there's no longer a dialog open
    // otherwise.
    if (e.key === 'Enter') {
      this.close(true);
      e.preventDefault();
      e.stopPropagation();
    } else if (e.key === 'Escape') {
      this.close(false);
      e.preventDefault();
      e.stopPropagation();
    } else if (e.altKey && e.key === 'r') {
      this.focusRating();
    } else if (e.altKey && e.key === 't') {
      this.focusTags();
    }
  };

  #onRatingSpanKeyDown = (e: KeyboardEvent) => {
    if (['0', '1', '2', '3', '4', '5'].includes(e.key)) {
      this.#setRating(parseInt(e.key));
      e.preventDefault();
      e.stopPropagation();
    } else if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
      const rating = this.#rating + (e.key === 'ArrowLeft' ? -1 : 1);
      this.#setRating(clamp(rating, 0, 5));
      e.preventDefault();
      e.stopPropagation();
    }
  };
}

type UpdateCallback = (rating: number | null, tags: string[] | null) => void;
