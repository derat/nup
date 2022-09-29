// Copyright 2021 Daniel Erat.
// All rights reserved.

import {
  $,
  createTemplate,
  formatDuration,
  getCoverUrl,
  getRatingString,
  largeCoverSize,
  smallCoverSize,
} from './common.js';
import { createDialog } from './dialog.js';

const template = createTemplate(`
<style>
  :host {
  }
  hr.title {
    margin-bottom: var(--margin);
  }
  #container {
    align-items: flex-start;
    display: flex;
  }
  #cover-div {
    background-size: cover;
    cursor: pointer;
    height: 192px;
    margin-bottom: var(--margin);
    outline: solid 1px var(--border-color);
    width: 192px;
  }
  #cover-div.hidden {
    display: none;
  }
  #cover-img {
    height: 100%;
    object-fit: cover;
    width: 100%;
  }
  .info-table {
    border-collapse: collapse;
    line-height: 1.2em;
    margin-bottom: var(--margin);
    width: 256px;
  }
  .info-table td:first-child {
    color: var(--text-label-color);
    display: inline-block;
    margin-right: 0.5em;
    text-align: right;
    user-select: none;
    width: 4em;
  }
  #rating.rated {
    letter-spacing: 3px;
  }
</style>

<div class="title">Song info</div>
<hr class="title" />

<div id="container">
  <div id="cover-div">
    <a id="cover-link"><img id="cover-img" alt="" /></a>
  </div>
  <!-- prettier-ignore -->
  <table class="info-table">
    <tr><td>Artist</td><td id="artist"></td></tr>
    <tr><td>Title</td><td id="title"></td></tr>
    <tr><td>Album</td><td id="album"></td></tr>
    <tr><td>Track</td><td id="track"></td></tr>
    <tr><td>Date</td><td id="date"></td></tr>
    <tr><td>Length</td><td id="length"></td></tr>
    <tr><td>Rating</td><td id="rating"></td></tr>
    <tr><td>Tags</td><td id="tags"></td></tr>
  </table>
</div>

<form method="dialog">
  <div class="button-container">
    <button id="dismiss-button" autofocus>Dismiss</button>
  </div>
</form>
`);

// Displays a modal dialog containing information about |song|.
export function showSongInfoDialog(song: Song, isCurrent = false) {
  const dialog = createDialog(template, 'song-info');
  const shadow = dialog.firstElementChild!.shadowRoot!;

  const coverDiv = $('cover-div', shadow);
  const coverImg = $('cover-img', shadow) as HTMLImageElement;
  if (song.coverFilename) {
    const small = getCoverUrl(song.coverFilename, smallCoverSize);
    const large = getCoverUrl(song.coverFilename, largeCoverSize);
    coverImg.src = large;
    coverImg.srcset = `${small} ${smallCoverSize}w, ${large} ${largeCoverSize}w`;
    coverImg.sizes = '192px';

    // Display the small image as a placeholder if we know it's cached.
    if (isCurrent) {
      coverDiv.style.backgroundImage = `url("${encodeURI(small)}")`;
    }

    const link = $('cover-link', shadow) as HTMLAnchorElement;
    link.href = getCoverUrl(song.coverFilename);
    link.target = '_blank';
  } else {
    coverDiv.classList.add('hidden');
  }

  $('artist', shadow).innerText = song.artist;
  $('title', shadow).innerText = song.title;
  $('album', shadow).innerText = song.album;
  $('track', shadow).innerText =
    (song.track >= 1 ? song.track.toString() : '') +
    (song.disc > 1 ? ` (Disc ${song.disc})` : '');
  $('date', shadow).innerText = song.date?.substring(0, 10) ?? '';
  $('length', shadow).innerText = formatDuration(song.length);
  $('rating', shadow).innerText = getRatingString(song.rating);
  if (song.rating) $('rating', shadow).classList.add('rated');
  $('tags', shadow).innerText = song.tags?.join(' ') ?? '';
  $('dismiss-button', shadow).addEventListener('click', () => dialog.close());
}
