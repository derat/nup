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
    width: 400px;
  }
  hr.title {
    margin-bottom: var(--margin);
  }

  #stats-div {
    display: none;
    width: 100%;
  }
  #stats-div.ready {
    display: block;
  }

  #summary-div {
    display: flex;
    justify-content: space-between;
    margin-bottom: var(--margin);
  }
  #songs,
  #albums,
  #duration {
    font-weight: bold;
  }

  .chart-wrapper {
    display: flex;
    margin-bottom: var(--margin);
  }
  .chart-wrapper .label {
    min-width: 5em;
  }
  .chart {
    border-radius: 4px;
    display: flex;
    height: 16px;
    outline: solid 1px var(--border-color);
    overflow: hidden;
    width: 100%;
  }
  .chart span {
    color: var(--chart-text-color);
    font-size: 10px;
    overflow: hidden;
    padding-top: 2.5px;
    text-align: center;
    text-overflow: ellipsis;
    text-shadow: 0 0 4px black;
  }

  #years-div {
    line-height: 1.2em;
    margin-bottom: var(--margin);
    max-height: 180px;
    overflow: scroll;
    width: 100%;
  }
  #years-table {
    border-spacing: 0;
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
    border-collapse: collapse;
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

<div id="stats-div">
  <div id="summary-div">
    <span>Songs: <span id="songs"></span></span>
    <span>Albums: <span id="albums"></span></span>
    <span>Duration: <span id="duration"></span></span>
  </div>

  <div class="chart-wrapper">
    <span class="label">Decades:</span>
    <div id="decades-chart" class="chart"></div>
  </div>

  <div class="chart-wrapper">
    <span class="label">Ratings:</span>
    <div id="ratings-chart" class="chart"></div>
  </div>

  <div id="years-div">
    <table id="years-table">
      <thead>
        <tr>
          <th>Year</th>
          <th>First plays</th>
          <th>Last plays</th>
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
export function showStatsDialog() {
  const dialog = createDialog(template, 'stats');
  const shadow = dialog.firstElementChild!.shadowRoot!;
  $('dismiss-button', shadow).addEventListener('click', () => dialog.close());

  fetch('stats', { method: 'GET' })
    .then((res) => handleFetchError(res))
    .then((res) => res.json())
    .then((stats: Stats) => {
      // This corresponds to the Stats struct in server/db/stats.go.
      $('songs', shadow).innerText = stats.songs.toLocaleString();
      $('albums', shadow).innerText = stats.albums.toLocaleString();
      $('duration', shadow).innerText = formatDays(stats.totalSec);

      const decades = Object.entries(stats.songDecades).sort();
      fillChart(
        $('decades-chart', shadow),
        decades.map(([_, c]) => c),
        stats.songs,
        decades.map(([d, _]) => (d === '0' ? '-' : d.slice(2) + 's')),
        decades.map(
          ([d, c]) =>
            `${d === '0' ? 'Unset' : d + 's'} - ` +
            `${c} ${c !== 1 ? 'songs' : 'song'}`
        )
      );

      const ratingCounts = [...Array(6).keys()].map(
        (i) => stats.ratings[i] ?? 0
      );
      fillChart(
        $('ratings-chart', shadow),
        ratingCounts,
        stats.songs,
        ratingCounts.map((_, i) => (i === 0 ? '-' : `${i}`)),
        ratingCounts.map(
          (c, i) =>
            `${i === 0 ? 'Unrated' : '★'.repeat(i)} - ` +
            `${c} ${c !== 1 ? 'songs' : 'song'}`
        )
      );

      const tbody = shadow.querySelector('#years-table tbody') as HTMLElement;
      for (const [year, ystats] of Object.entries(stats.years).sort()) {
        const row = createElement('tr', null, tbody);
        createElement('td', null, row, year);
        createElement('td', null, row, ystats.firstPlays.toLocaleString());
        createElement('td', null, row, ystats.lastPlays.toLocaleString());
        createElement('td', null, row, ystats.plays.toLocaleString());
        createElement('td', null, row, formatDays(ystats.totalSec));
      }

      const updateTime = formatRelativeTime(
        (Date.parse(stats.updateTime) - Date.now()) / 1000
      );
      $('updated-div', shadow).innerText = `Updated ${updateTime}`;

      $('stats-div', shadow).classList.add('ready');
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
  songDecades: Record<string, number>;
  tags: Record<string, number>;
  years: Record<string, PlayStats>;
  updateTime: string;
}

interface PlayStats {
  plays: number;
  totalSec: number;
  firstPlays: number;
  lastPlays: number;
}

const formatDays = (sec: number) => `${(sec / 86400).toFixed(1)} days`;

// Adds spans within div corresponding to vals and titles.
function fillChart(
  div: HTMLElement,
  vals: number[],
  total: number,
  labels: string[],
  titles: string[]
) {
  if (total <= 0) return;
  for (let i = 0; i < vals.length; i++) {
    const pct = vals[i] / total;
    // Omit labels in tiny spans.
    const el = createElement('span', null, div, pct >= 0.04 ? labels[i] : null);
    el.style.width = `${100 * pct}%`;
    // Add slop to the final span to deal with rounding errors.
    if (i === vals.length - 1) el.style.width = `calc(${el.style.width} + 2px)`;
    const opacity = (i / (vals.length - 1)) ** 2;
    el.style.backgroundColor = `rgba(var(--chart-bar-rgb), ${opacity})`;
    el.title = titles[i];
  }
}
