// Copyright 2021 Daniel Erat.
// All rights reserved.

import {
  $,
  createElement,
  createTemplate,
  formatRelativeTime,
  handleFetchError,
} from './common.js';
import { createDialog } from './dialog.js';

const template = createTemplate(`
<style>
  :host {
    width: 25em;
  }
  hr.title {
    margin-bottom: var(--margin);
  }

  #top-div {
    align-items: flex-start;
    display: flex;
    gap: 30px;
    visibility: hidden;
  }
  #top-div.ready {
    visibility: visible;
  }

  table {
    border-collapse: collapse;
    width: 100%;
  }

  #main-table {
    line-height: 1.4em;
    margin-bottom: var(--margin);
  }
  #main-table td:first-child {
    font-weight: bold;
  }
  #main-table td:not(:first-child) {
    text-align: right;
  }

  #years-div {
    line-height: 1.2em;
    margin-bottom: var(--margin);
    max-height: 180px;
    overflow: scroll;
    width: 100%;
  }
  #years-table th {
    background-color: var(--bg-color);
    position: sticky;
    top: 0;
    z-index: 1;
  }
  /* Gross hack from https://stackoverflow.com/a/57170489/6882947 to keep
   * border from scrolling along with table contents. */
  #years-table th:after {
    border-bottom: solid 1px var(--border-color);
    bottom: 0;
    content: '';
    position: absolute;
    left: 0;
    width: 100%;
  }
  #years-table th:not(:first-child),
  #years-div td:not(:first-child) {
    text-align: right;
  }

  #updated-div {
    font-size: 90%;
    opacity: 50%;
  }
</style>

<div class="title">Stats</div>
<hr class="title" />

<div id="top-div">
  <!-- prettier-ignore -->
  <table id="main-table">
    <tr><td>Songs</td><td id="songs"></td></tr>
    <tr><td>★★★★★</td><td id="rating-5"></td></tr>
    <tr><td>★★★★</td><td id="rating-4"></td></tr>
    <tr><td>★★★</td><td id="rating-3"></td></tr>
    <tr><td>★★</td><td id="rating-2"></td></tr>
    <tr><td>★</td><td id="rating-1"></td></tr>
    <tr><td>Unrated</td><td id="rating-0"></td></tr>
    <tr><td>Albums</td><td id="albums"></td></tr>
    <tr><td>Duration</td><td id="duration"></td></tr>
  </table>

  <div id="years-div">
    <table id="years-table">
      <thead>
        <tr>
          <th>Year</th>
          <th>Plays</th>
          <th>Playtime</th>
        </tr>
      </thead>
      <tbody></tbody>
    </table>
  </div>
</div>

<div id="updated-div">Loading stats...</div>

<form method="dialog">
  <div class="button-container">
    <button id="dismiss-button" autofocus>Dismiss</button>
  </div>
</form>
`);

// Shows a modal dialog containing stats fetched from the server.
export function showStats() {
  const dialog = createDialog(template, 'stats');
  const shadow = dialog.firstElementChild.shadowRoot;
  $('dismiss-button', shadow).addEventListener('click', () => dialog.close());

  fetch('stats', { method: 'GET' })
    .then((res) => handleFetchError(res))
    .then((res) => res.json())
    .then((stats: Stats) => {
      // This corresponds to the Stats struct in server/db/stats.go.
      $('songs', shadow).innerText = stats.songs.toLocaleString();
      for (const rating of ['0', '1', '2', '3', '4', '5']) {
        const td = $(`rating-${rating}`, shadow);
        const songs = stats.ratings[rating];
        td.innerText = songs ? songs.toLocaleString() : '0';
      }
      $('albums', shadow).innerText = stats.albums.toLocaleString();
      $('duration', shadow).innerText = formatDays(stats.totalSec);

      const tbody = shadow.querySelector('#years-table tbody') as HTMLElement;
      for (const [year, ystats] of Object.entries(stats.years).sort()) {
        const row = createElement('tr', null, tbody);
        createElement('td', null, row, year);
        createElement('td', null, row, ystats.plays.toLocaleString());
        createElement('td', null, row, formatDays(ystats.totalSec));
      }

      const updateTime = formatRelativeTime(
        (Date.parse(stats.updateTime) - Date.now()) / 1000
      );
      $('updated-div', shadow).innerText = `Updated ${updateTime}`;

      $('top-div', shadow).classList.add('ready');
    })
    .catch((err) => {
      $('updated-div', shadow).innerText = err.toString();
    });
}

interface Stats {
  songs: number;
  albums: number;
  totalSec: number;
  ratings: Record<string, number>;
  tags: Record<string, number>;
  years: Record<string, PlayStats>;
  updateTime: string;
}

interface PlayStats {
  plays: number;
  totalSec: number;
}

const formatDays = (sec: number) => `${(sec / 86400).toFixed(1)} days`;
