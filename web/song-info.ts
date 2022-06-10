// Copyright 2021 Daniel Erat.
// All rights reserved.

import {
  $,
  createTemplate,
  formatDuration,
  getCoverUrl,
  getRatingString,
  largeCoverSize,
} from './common.js';
import { createDialog } from './dialog.js';

const template = createTemplate(`
<style>
  :host {
    min-width: 15em;
    max-width: 25em;
  }
  hr.title {
    margin-bottom: var(--margin);
  }
  #cover-link {
    display: block;
    text-align: center;
  }
  #cover-img {
    cursor: pointer;
    height: 192px;
    margin-bottom: var(--margin);
    object-fit: cover;
    outline: solid 1px var(--border-color);
    width: 192px;
  }
  #cover-img.hidden {
    display: none;
  }
  .info-table {
    border-collapse: collapse;
    line-height: 1.2em;
    margin-bottom: var(--margin);
  }
  .info-table td:first-child {
    color: var(--text-label-color);
    display: inline-block;
    margin-right: 0.5em;
    text-align: right;
    user-select: none;
    width: 4em;
  }
</style>

<div class="title">Song info</div>
<hr class="title" />

<a id="cover-link"><img id="cover-img" /></a>

<!-- prettier-ignore -->
<table class="info-table">
  <tr><td>Artist</td><td id="artist"></td></tr>
  <tr><td>Title</td><td id="title"></td></tr>
  <tr><td>Album</td><td id="album"></td></tr>
  <tr><td>Track</td><td id="track"></td></tr>
  <tr><td>Length</td><td id="length"></td></tr>
  <tr><td>Rating</td><td id="rating"></td></tr>
  <tr><td>Tags</td><td id="tags"></td></tr>
</table>

<form method="dialog">
  <div class="button-container">
    <button id="dismiss-button" autofocus>Dismiss</button>
  </div>
</form>
`);

// Displays a modal dialog containing information about |song|.
export function showSongInfo(song: Song) {
  const dialog = createDialog(template, 'song-info');
  const shadow = dialog.firstElementChild!.shadowRoot!;

  const cover = $('cover-img', shadow) as HTMLImageElement;
  if (song.coverFilename) {
    cover.src = getCoverUrl(song.coverFilename, largeCoverSize);
    const link = $('cover-link', shadow) as HTMLAnchorElement;
    link.href = getCoverUrl(song.coverFilename);
    link.target = '_blank';
  } else {
    cover.classList.add('hidden');
  }

  $('artist', shadow).innerText = song.artist;
  $('title', shadow).innerText = song.title;
  $('album', shadow).innerText = song.album;
  $('track', shadow).innerText =
    song.track + (song.disc > 1 ? ` (Disc ${song.disc})` : '');
  $('length', shadow).innerText = formatDuration(song.length);
  $('rating', shadow).innerText = getRatingString(song.rating);
  $('tags', shadow).innerText = song.tags ? song.tags.join(' ') : '';
  $('dismiss-button', shadow).addEventListener('click', () => dialog.close());
}
