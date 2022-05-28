// Copyright 2021 Daniel Erat.
// All rights reserved.

import { expectEq, error, fatal, suite, test } from './test.js';
import {
  createElement,
  formatDuration,
  formatRelativeTime,
  getRatingString,
  moveItem,
} from './common.js';

suite('common', () => {
  test('formatDuration', () => {
    for (const [sec, want] of [
      [0, '0:00'],
      [1, '0:01'],
      [59, '0:59'],
      [60, '1:00'],
      [61, '1:01'],
      [599, '9:59'],
      [600, '10:00'],
      [601, '10:01'],
      [3599, '59:59'],
      [3600, '60:00'],
      [3601, '60:01'],
    ]) {
      const got = formatDuration(sec);
      if (got !== want) {
        error(`formatDuration(${sec}) = "${got}"; want "${want}"`);
      }
    }
  });

  test('formatRelativeTime', () => {
    for (const [sec, wantBase] of [
      [0, '0 seconds'],
      [1, '1 second'],
      [1.49, '1 second'],
      [1.51, '2 seconds'], // -1.5 rounds to -1, not -2
      [59.49, '59 seconds'],
      [59.51, '1 minute'],
      [60, '1 minute'],
      [89, '1 minute'],
      [91, '2 minutes'],
      [3569, '59 minutes'],
      [3571, '1 hour'],
      [3600, '1 hour'],
      [5399, '1 hour'],
      [5401, '2 hours'],
      [23 * 3600 + 1799, '23 hours'],
      [23 * 3600 + 1801, '1 day'],
      [86400, '1 day'],
      [86400 + 43199, '1 day'],
      [86400 + 43201, '2 days'],
    ]) {
      const gotPos = formatRelativeTime(sec);
      const wantPos = `in ${wantBase}`;
      if (gotPos !== wantPos) {
        error(`formatRelativeTime(${sec}) = "${gotPos}"; want "${wantPos}"`);
      }

      if (sec !== 0) {
        const gotNeg = formatRelativeTime(-sec);
        const wantNeg = `${wantBase} ago`;
        if (gotNeg !== wantNeg) {
          error(`formatRelativeTime(${-sec}) = "${gotNeg}"; want "${wantNeg}"`);
        }
      }
    }
  });

  test('createElement', () => {
    const p = createElement('p', 'foo bar', null, 'Hi there');
    expectEq(p.nodeName, 'P', 'nodeName');
    expectEq(p.className, 'foo bar', 'className');
    expectEq(p.innerText, 'Hi there', 'innerText');
    expectEq(p.parentElement, null, 'parentElement');

    const br = createElement('br', null, p, null);
    expectEq(br.nodeName, 'BR', 'nodeName');
    expectEq(br.className, '', 'className');
    expectEq(br.innerText, '', 'innerText');
    expectEq(br.parentElement, p, 'parentElement');
  });

  test('getRatingString', () => {
    for (const [args, want] of [
      [[0], 'Unrated'],
      [[1], '★☆☆☆☆'],
      [[2], '★★☆☆☆'],
      [[3], '★★★☆☆'],
      [[4], '★★★★☆'],
      [[5], '★★★★★'],
      [[3, '*', ''], '***'],
      [[0, '★', '☆', ''], ''],
      [[2, '*', '', 'Unrated', 'Rated: '], 'Rated: **'],
      [[0, '*', '', 'Unrated', 'Rated: '], 'Unrated'],
    ]) {
      const got = getRatingString.apply(null, args);
      if (got !== want) {
        error(`getRatingString(${args.join(', ')}) = ${got}; want ${want}`);
      }
    }
  });

  test('moveItem', () => {
    for (const [array, from, to, idx, wantArray, wantIdx] of [
      [[0, 1, 2, 3], 0, 0, 0, [0, 1, 2, 3], 0],
      [[0, 1, 2, 3], 0, 1, 0, [1, 0, 2, 3], 1],
      [[0, 1, 2, 3], 0, 2, 1, [1, 2, 0, 3], 0],
      [[0, 1, 2, 3], 0, 3, 1, [1, 2, 3, 0], 0],
      [[0, 1, 2, 3], 0, 3, 3, [1, 2, 3, 0], 2],
      [[0, 1, 2, 3], 1, 0, 0, [1, 0, 2, 3], 1],
      [[0, 1, 2, 3], 3, 0, 2, [3, 0, 1, 2], 3],
      [[0, 1, 2, 3], 2, 1, 1, [0, 2, 1, 3], 2],
      [[0, 1, 2, 3], 2, 1, undefined, [0, 2, 1, 3], undefined],
    ]) {
      const gotArray = array.slice();
      const gotIdx = moveItem(gotArray, from, to, idx);
      const desc = `moveItem([${array.join(',')}], ${from}, ${to}, ${idx})`;
      expectEq(gotArray, wantArray, desc);
      expectEq(gotIdx, wantIdx, desc);
    }
  });
});
