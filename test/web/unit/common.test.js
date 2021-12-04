// Copyright 2021 Daniel Erat.
// All rights reserved.

import { addSuite, error, fatal, test } from './test.js';
import { formatTime } from './common.js';

addSuite('common', () => {
  test('formatTime', () => {
    for (const [sec, want] of [
      [0, '0:00'],
      [1, '0:01'],
      [59, '0:59'],
      [60, '1:00'],
      [61, '1:01'],
    ]) {
      const got = formatTime(sec);
      if (got !== want) {
        error(`formatTime(${sec}) = "${got}"; want "${want}"`);
      }
    }
  });
});
