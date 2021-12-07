// Copyright 2021 Daniel Erat.
// All rights reserved.

import { afterEach, beforeEach, expectEq, suite, test } from './test.js';
import MockWindow from './mock-window.js';
import Updater from './updater.js';

suite('updater', () => {
  let w = null;
  beforeEach(() => {
    w = new MockWindow();
  });
  afterEach(() => {
    w.finish();
  });

  function playedUrl(songId, startTime) {
    return `played?songId=${songId}&startTime=${startTime}`;
  }

  test('reportPlay (success)', async () => {
    const updater = new Updater();
    w.expectFetch(playedUrl('123', 456.5), 'POST', 'ok');
    await updater.reportPlay('123', 456.5);
    expectEq(w.numTimeouts, 0, 'numTimeouts');
  });

  test('reportPlay (retry)', async () => {
    const updater = new Updater();

    // Report a play and have the server return a 500 error.
    const id1 = '123';
    const t1 = 100123.5;
    w.expectFetch(playedUrl(id1, t1), 'POST', 'whoops', 500);
    await updater.reportPlay(id1, t1);

    // Report a second play and have it also fail.
    const id2 = '456';
    const t2 = 100456.8;
    w.expectFetch(playedUrl(id2, t2), 'POST', 'whoops', 500);
    await updater.reportPlay(id2, t2);

    // 200 ms later, nothing more should have happened.
    await w.runTimeouts(200);

    // We initially retry at 500 ms, so after 300 ms more, we should try to
    // report both plays again.
    w.expectFetch(playedUrl(id1, t1), 'POST', 'ok');
    w.expectFetch(playedUrl(id2, t2), 'POST', 'ok');
    await w.runTimeouts(300);
    expectEq(w.numTimeouts, 0, 'numTimeouts');
  });
});
