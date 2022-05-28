// Copyright 2021 Daniel Erat.
// All rights reserved.

import { afterEach, beforeEach, expectEq, suite, test } from './test.js';
import MockWindow from './mock-window.js';

import { formatDuration } from './common.js';
import {} from './song-table.js';

suite('songTable', () => {
  const makeSong = (n) => ({
    songId: `${n}`,
    artist: `ar${n}`,
    title: `ti${n}`,
    album: `al${n}`,
    albumId: `ai${n}`,
    length: n,
  });
  const song1 = makeSong(1);
  const song2 = makeSong(2);
  const song3 = makeSong(3);
  const song4 = makeSong(4);
  const song5 = makeSong(5);

  // Returns an array of song objects returned by getSong().
  const getSongs = () =>
    [...Array(table.numSongs).keys()].map((i) => table.getSong(i));

  // Returns an array of objects describing HTML song rows.
  const getRows = () =>
    Array.from(table.shadowRoot.querySelectorAll('tbody tr')).map((tr) => {
      const td = tr.querySelector('td.checkbox');
      const checked =
        window.getComputedStyle(td).getPropertyValue('display') === 'none'
          ? undefined
          : td.querySelector("input[type='checkbox']").checked;

      return {
        artist: tr.querySelector('td.artist').innerText,
        title: tr.querySelector('td.title').innerText,
        album: tr.querySelector('td.album').innerText,
        time: tr.querySelector('td.time').innerText,
        checked, // undefined if checkbox not shown, true/false otherwise
      };
    });

  // Convert a song from makeSong() to a row as returned by getRows().
  const songToRow = (s, checked) => ({
    artist: s.artist,
    title: s.title,
    album: s.album,
    time: formatDuration(s.length),
    checked,
  });

  // Returns the checkbox from the header.
  const getHeaderCheckbox = () =>
    table.shadowRoot.querySelector(`thead input[type='checkbox']`);

  // Returns the checkbox from the row with 0-based index |i|.
  const getRowCheckbox = (i) =>
    table.shadowRoot.querySelector(
      `tbody tr:nth-of-type(${i + 1}) input[type='checkbox']`
    );

  let w = null;
  let table = null;

  beforeEach(() => {
    w = new MockWindow();
    table = document.createElement('song-table');
    document.body.appendChild(table);
  });
  afterEach(() => {
    document.body.removeChild(table);
    table = null;
    w.finish();
  });

  test('setSongs', () => {
    expectEq(getSongs(), []);
    expectEq(getRows(), []);

    for (const songs of [
      [song1],
      [song1, song2],
      [song3, song1, song2, song4],
      [song3, song1, song5, song2, song4],
      [song1, song2, song3, song4, song5],
      [song2, song3, song4],
      [song2, song4],
      [],
    ]) {
      table.setSongs(songs);
      expectEq(table.numSongs, songs.length, 'Num songs');
      expectEq(getSongs(), songs, 'Song list');
      expectEq(
        getRows(),
        songs.map((s) => songToRow(s))
      );
    }
  });

  test('setSongs (title attributes)', () => {
    const longSong = {
      songId: 6,
      artist:
        'Very very long artist name, really far too long to fit in any reasonably-sized window',
      title:
        "This song also has a very long title, I can't believe it, can you, probably not",
      album:
        'Even the album name is too long, I kid you not. Why would someone do this?',
      albumId: 'ai6',
      length: 360,
    };
    table.setSongs([song1, longSong, song2]);

    // Longs strings should be copied to the title attribute so they'll be
    // displayed in tooltips, but short ones shouldn't.
    const rowAttrs = Array.from(
      table.shadowRoot.querySelectorAll('tbody tr')
    ).map((tr) => [
      tr.querySelector('td.artist').getAttribute('title'),
      tr.querySelector('td.title').getAttribute('title'),
      tr.querySelector('td.album').getAttribute('title'),
    ]);
    expectEq(rowAttrs, [
      [null, null, null],
      [longSong.artist, longSong.title, longSong.album],
      [null, null, null],
    ]);

    console.log(JSON.stringify(rowAttrs));
  });

  test('checkboxes', () => {
    // All of the rows should initially be unchecked.
    table.setAttribute('use-checkboxes', '');
    const songs = [song1, song2, song3];
    table.setSongs(songs);
    expectEq(
      getRows(),
      songs.map((s) => songToRow(s, false))
    );
    expectEq(table.checkedSongs, []);

    // Click the second checkbox.
    getRowCheckbox(1).click();
    expectEq(getRows(), [
      songToRow(song1, false),
      songToRow(song2, true),
      songToRow(song3, false),
    ]);
    expectEq(table.checkedSongs, [song2]);

    // Click the third checkbox.
    getRowCheckbox(2).click();
    expectEq(getRows(), [
      songToRow(song1, false),
      songToRow(song2, true),
      songToRow(song3, true),
    ]);
    expectEq(table.checkedSongs, [song2, song3]);

    // Click the header's checkbox to uncheck all rows.
    getHeaderCheckbox().click();
    expectEq(
      getRows(),
      songs.map((s) => songToRow(s, false))
    );
    expectEq(table.checkedSongs, []);

    // Click the header's checkbox again to check all rows.
    getHeaderCheckbox().click();
    expectEq(
      getRows(),
      songs.map((s) => songToRow(s, true))
    );
    expectEq(table.checkedSongs, songs);

    // Replacing the songs should uncheck all rows.
    const songs2 = [song4, song5];
    table.setSongs(songs2);
    expectEq(
      getRows(),
      songs2.map((s) => songToRow(s, false))
    );
    expectEq(table.checkedSongs, []);

    // Programatically check all rows.
    table.setAllCheckboxes(true);
    expectEq(
      getRows(),
      songs2.map((s) => songToRow(s, true))
    );
    expectEq(table.checkedSongs, songs2);
  });
});
