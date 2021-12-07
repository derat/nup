// Copyright 2021 Daniel Erat.
// All rights reserved.

import { addSuite, expectEq, error, fatal, test } from './test.js';
import {
  createElement,
  formatTime,
  getRatingString,
  numStarsToRating,
  ratingToNumStars,
} from './common.js';

addSuite('common', () => {
  test('formatTime', () => {
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
      const got = formatTime(sec);
      if (got !== want) {
        error(`formatTime(${sec}) = "${got}"; want "${want}"`);
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

  test('numStarsToRating', () => {
    for (const [stars, want] of [
      [-1, -1],
      [0, -1],
      [1, 0],
      [2, 0.25],
      [3, 0.5],
      [4, 0.75],
      [5, 1.0],
    ]) {
      const got = numStarsToRating(stars);
      if (got !== want) {
        error(`numStarsToRating(${stars}) = ${got}; want ${want}`);
      }
    }
  });

  test('ratingToNumStars', () => {
    for (const [rating, want] of [
      [-1, 0],
      [0, 1],
      [0.25, 2],
      [0.5, 3],
      [0.75, 4],
      [1, 5],
    ]) {
      const got = ratingToNumStars(rating);
      if (got !== want) {
        error(`ratingToNumStars(${rating}) = ${got}; want ${want}`);
      }
    }
  });

  test('getRatingString', () => {
    for (const [args, want] of [
      [[-1], 'Unrated'],
      [[0], '★☆☆☆☆'],
      [[0.25], '★★☆☆☆'],
      [[0.5], '★★★☆☆'],
      [[0.75], '★★★★☆'],
      [[1], '★★★★★'],
      [[0.5, '*', ''], '***'],
      [[-1, '★', '☆', ''], ''],
      [[0.25, '*', '', 'Unrated', 'Rated: '], 'Rated: **'],
      [[-1, '*', '', 'Unrated', 'Rated: '], 'Unrated'],
    ]) {
      const got = getRatingString.apply(null, args);
      if (got !== want) {
        error(`getRatingString(${args.join(', ')}) = ${got}; want ${want}`);
      }
    }
  });
});
