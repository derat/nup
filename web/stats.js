// Copyright 2021 Daniel Erat.
// All rights reserved.

import {
  $,
  createElement,
  createShadow,
  createTemplate,
  handleFetchError,
} from './common.js';

const template = createTemplate(`
<style>
  @import 'common.css';
  @import 'dialog.css';
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
  #years-table th:not(:first-child), #years-div td:not(:first-child) {
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
  <table id="main-table">
    <tr><td>Songs</td><td id="songs"></td></tr>
    <tr><td>★★★★★</td><td id="rating-1.00"></td></tr>
    <tr><td>★★★★</td><td id="rating-0.75"></td></tr>
    <tr><td>★★★</td><td id="rating-0.50"></td></tr>
    <tr><td>★★</td><td id="rating-0.25"></td></tr>
    <tr><td>★</td><td id="rating-0.00"></td></tr>
    <tr><td>Unrated</td><td id="rating--1.00"></td></tr>
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

<div id="updated-div"></div>

<div class="button-container">
  <button id="dismiss-button">Dismiss</button>
</div>
`);

export function showStats(manager) {
  const container = manager.createDialog();
  container.classList.add('stats'); // for tests
  const shadow = createShadow(container, template);

  // TODO: Display loading message.
  fetch('stats', { method: 'GET' })
    .then((res) => handleFetchError(res))
    .then((res) => res.json())
    .then((stats) => {
      // This corresponds to the Stats struct in server/db/stats.go.
      $('songs', shadow).innerText = parseInt(stats.songs).toLocaleString();
      for (const rating of ['-1.00', '0.00', '0.25', '0.50', '0.75', '1.00']) {
        const td = $(`rating-${rating}`, shadow);
        const songs = stats.ratings[rating];
        td.innerText = songs ? parseInt(songs).toLocaleString() : '0';
      }
      $('albums', shadow).innerText = parseInt(stats.albums).toLocaleString();
      $('duration', shadow).innerText =
        parseFloat(stats.totalSec / 86400).toFixed(1) + ' days';

      const tbody = shadow.querySelector('#years-table tbody');
      for (const [year, ystats] of Object.entries(stats.years).sort()) {
        const row = createElement('tr', null, tbody);
        createElement('td', null, row, year);
        createElement('td', null, row, parseInt(ystats.plays).toLocaleString());
        const days = parseFloat(ystats.totalSec / 86400).toFixed(1);
        createElement('td', null, row, days);
      }

      const sec = (Date.now() - Date.parse(stats.updateTime)) / 1000;
      const days = sec / 86400;
      const hours = sec / 3600;
      const min = sec / 60;
      const age =
        days >= 1.5
          ? `${Math.round(days)} days ago`
          : days >= 1
          ? '1 day ago'
          : hours >= 1.5
          ? `${Math.round(hours)} hours ago`
          : hours >= 1
          ? '1 hour ago'
          : min >= 1.5
          ? `${Math.round(min)} minutes ago`
          : min >= 1
          ? '1 minute ago'
          : 'just now';
      $('updated-div', shadow).innerText = `Updated ${age}`;
    });

  const dismiss = $('dismiss-button', shadow);
  dismiss.addEventListener('click', () => manager.closeChild(container));
  dismiss.focus();
}

const formatDuration = (sec) => `${parseFloat(sec / 86400).toFixed(1)} days`;
