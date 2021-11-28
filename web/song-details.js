// Copyright 2021 Daniel Erat.
// All rights reserved.

import {
  $,
  createShadow,
  createTemplate,
  formatTime,
  getRatingString,
} from './common.js';

const template = createTemplate(`
<style>
  @import 'common.css';
  @import 'dialog.css';
  :host {
    min-width: 15em;
    max-width: 25em;
  }
  hr.title {
    margin-bottom: 12px;
  }
  .details-table {
    border-collapse: collapse;
    line-height: 1.2em;
  }
  .details-table td:first-child {
    color: var(--text-label-color);
    display: inline-block;
    margin-right: 0.5em;
    text-align: right;
    width: 4em;
  }
</style>

<div class="title">Song details</div>
<hr class="title" />

<table class="details-table">
  <tr><td>Artist:</td><td id="artist"></td></tr>
  <tr><td>Title:</td><td id="title"></td></tr>
  <tr><td>Album:</td><td id="album"></td></tr>
  <tr><td>Track:</td><td id="track"></td></tr>
  <tr><td>Length:</td><td id="length"></td></tr>
  <tr><td>Rating:</td><td id="rating"></td></tr>
  <tr><td>Tags:</td><td id="tags"></td></tr>
</table>

<div class="button-container">
  <button id="dismiss-button">Dismiss</button>
</div>
`);

export function showSongDetails(manager, song) {
  const container = manager.createDialog();
  const shadow = createShadow(container, template);

  $('artist', shadow).innerText = song.artist;
  $('title', shadow).innerText = song.title;
  $('album', shadow).innerText = song.album;
  $('track', shadow).innerText =
    song.track + (song.disc > 1 ? ` (Disc ${song.disc})` : '');
  $('length', shadow).innerText = formatTime(parseFloat(song.length));
  $('rating', shadow).innerText = getRatingString(song.rating);
  $('tags', shadow).innerText = song.tags ? song.tags.join(' ') : '';

  const dismiss = $('dismiss-button', shadow);
  dismiss.addEventListener('click', () => manager.closeChild(container));
  dismiss.focus();
}
