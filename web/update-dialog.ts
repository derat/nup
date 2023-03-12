// Copyright 2022 Daniel Erat.
// All rights reserved.

import {
  $,
  clamp,
  createTemplate,
  emptyStarIcon,
  setIcon,
  starIcon,
  xIcon,
} from './common.js';
import { createDialog } from './dialog.js';
import type { TagSuggester } from './tag-suggester.js';

const template = createTemplate(`
<style>
  #close-icon {
    padding: 6px;
    position: absolute;
    right: 0;
    top: 0;
  }
  #heading-row {
    display: flex;
    margin-bottom: 6px;
    max-width: 215px;
  }
  #heading-dash {
    white-space: pre;
  }
  #artist, #title {
    font-weight: bold;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  #rating-row {
    align-items: center;
    display: flex;
    margin-bottom: -1px;
    margin-top: 1px;
    height: 18px;
  }
  #rating {
    display: flex;
    margin-left: 2px;
    margin-top: 2px;
  }
  #rating svg {
    cursor: pointer;
    fill: var(--text-color);
    height: 20px;
    margin-right: -3px;
    opacity: 0.6;
    width: 20px;
  }
  #rating svg:hover {
    opacity: 0.9;
  }
  #tags-textarea {
    font-family: Arial, Helvetica, sans-serif;
    height: 64px;
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

<svg id="close-icon" title="Close"></svg>
<div id="heading-row">
  <span id="artist"></span>
  <span id="heading-dash"> - </span>
  <span id="title"></span>
</div>
<div id="rating-row">
  Rating:
  <div id="rating" tabindex="0">
    <a><svg></svg></a>
    <a><svg></svg></a>
    <a><svg></svg></a>
    <a><svg></svg></a>
    <a><svg></svg></a>
  </div>
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
    const style = getComputedStyle(this.#dialog);
    const margin = style.getPropertyValue('--margin');
    const radius = style.getPropertyValue('--control-border-radius');
    this.#dialog.style.borderRadius = radius;
    this.#dialog.style.margin = '0'; // needed to avoid centering
    this.#dialog.style.overflow = 'visible'; // for suggestion popup
    this.#dialog.style.padding = '8px';
    this.#dialog.style.position = 'absolute';
    // Prevent the cover image from awkwardly peeking out behind the top-left
    // rounded corner.
    this.#dialog.style.left = this.#dialog.style.top = `calc(${margin} - 1px)`;

    $('artist', this.#shadow).innerText = song.artist;
    $('title', this.#shadow).innerText = song.title;

    setIcon($('close-icon', this.#shadow), xIcon).addEventListener(
      'click',
      () => this.close(true)
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

    this.#callback(this.#song, rating, tags);
  }

  #setRating(rating: number) {
    this.#rating = rating;
    for (let i = 1; i <= 5; i++) {
      setIcon(
        this.#ratingSpan.children[i - 1]!.firstChild as HTMLElement,
        i <= rating ? starIcon : emptyStarIcon
      );
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

type UpdateCallback = (
  song: Song,
  rating: number | null,
  tags: string[] | null
) => void;
