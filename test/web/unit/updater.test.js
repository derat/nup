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
  function rateAndTagUrl(songId, rating, tags) {
    let url = `rate_and_tag?songId=${songId}`;
    if (rating != null) url += `&rating=${rating}`;
    if (tags != null) url += `&tags=${encodeURIComponent(tags.join(' '))}`;
    return url;
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

  test('reportPlay (backoff)', async () => {
    // Make the initial attempt fail.
    const updater = new Updater();
    w.expectFetch(playedUrl('1', 2), 'POST', 'whoops', 500);
    await updater.reportPlay('1', 2);

    // The retry time should double up to 5 minutes.
    for (const ms of [
      500, 1_000, 2_000, 4_000, 8_000, 16_000, 32_000, 64_000, 128_000, 256_000,
      300_000, 300_000, 300_000,
    ]) {
      w.expectFetch(playedUrl('1', 2), 'POST', 'fail', 500);
      await w.runTimeouts(ms);
    }

    // Try to report a second play and check that it doesn't reset the delay.
    w.expectFetch(playedUrl('1', 4), 'POST', 'fail', 500);
    await updater.reportPlay('1', 4);
    expectEq(w.numUnsatisfiedFetches, 0, 'Unsatisfied fetches');
    await w.runTimeouts(299_000);

    // Wait the final second and let the next attempt succeed.
    w.expectFetch(playedUrl('1', 2), 'POST', 'ok');
    w.expectFetch(playedUrl('1', 4), 'POST', 'ok');
    await w.runTimeouts(1_000);

    // Report another play and check that it's sent immediately.
    w.expectFetch(playedUrl('1', 6), 'POST', 'ok');
    await updater.reportPlay('1', 6);
    expectEq(w.numTimeouts, 0, 'numTimeouts');
  });

  test('reportPlay (retry queued at startup)', async () => {
    // Make the initial playback report fail.
    let updater = new Updater();
    w.expectFetch(playedUrl('123', 456), 'POST', 'fail', 500);
    await updater.reportPlay('123', 456);

    // Clear timeouts to make sure the old updater isn't doing anything
    // and create a new updater. It should pick up the old report from
    // localStorage and send it again, but make it fail again.
    w.clearTimeouts();
    w.expectFetch(playedUrl('123', 456), 'POST', 'fail', 500);
    updater = new Updater();
    await updater.initialRetryDoneForTest;

    // Let the next attempt succeed.
    w.clearTimeouts();
    w.expectFetch(playedUrl('123', 456), 'POST', 'ok');
    updater = new Updater();
    await updater.initialRetryDoneForTest;
    expectEq(w.numTimeouts, 0, 'numTimeouts');

    // If we create a new updater again, nothing should be sent.
    updater = new Updater();
    await updater.initialRetryDoneForTest;
    expectEq(w.numTimeouts, 0, 'numTimeouts');
  });

  test('rateAndTag (success)', async () => {
    const updater = new Updater();
    w.expectFetch(rateAndTagUrl('123', 0.75, null), 'POST', 'ok');
    await updater.rateAndTag('123', 0.75, null);
    w.expectFetch(rateAndTagUrl('123', null, ['abc', 'def']), 'POST', 'ok');
    await updater.rateAndTag('123', null, ['abc', 'def']);
    w.expectFetch(rateAndTagUrl('123', 1.0, ['ijk']), 'POST', 'ok');
    await updater.rateAndTag('123', 1.0, ['ijk']);
    expectEq(w.numTimeouts, 0, 'numTimeouts');
  });

  test('rateAndTag (retry)', async () => {
    const updater = new Updater();

    // Rate and tag a song and have the server report failure.
    w.expectFetch(rateAndTagUrl('123', 0.25, ['old']), 'POST', 'bad', 500);
    await updater.rateAndTag('123', 0.25, ['old']);

    // Try to send an updated rating and tag for the same song.
    w.expectFetch(rateAndTagUrl('123', 0.75, ['new']), 'POST', 'bad', 500);
    await updater.rateAndTag('123', 0.75, ['new']);

    // Send a rating and tag for another song.
    w.expectFetch(rateAndTagUrl('456', 1.0, ['other']), 'POST', 'bad', 500);
    await updater.rateAndTag('456', 1.0, ['other']);

    // After a 500 ms delay, the latest data for each song should be sent.
    w.expectFetch(rateAndTagUrl('123', 0.75, ['new']), 'POST', 'ok');
    w.expectFetch(rateAndTagUrl('456', 1.0, ['other']), 'POST', 'ok');
    await w.runTimeouts(500);
    expectEq(w.numTimeouts, 0, 'numTimeouts');
  });

  test('rateAndTag (retry queued at startup)', async () => {
    // Make the initial playback report fail.
    let updater = new Updater();
    w.expectFetch(rateAndTagUrl('123', 1.0, ['tag']), 'POST', 'bad', 500);
    await updater.rateAndTag('123', 1.0, ['tag']);

    // Fail again with a new updater.
    w.clearTimeouts();
    w.expectFetch(rateAndTagUrl('123', 1.0, ['tag']), 'POST', 'bad', 500);
    updater = new Updater();
    await updater.initialRetryDoneForTest;

    // Let the update work this time.
    w.clearTimeouts();
    w.expectFetch(rateAndTagUrl('123', 1.0, ['tag']), 'POST', 'ok');
    updater = new Updater();
    await updater.initialRetryDoneForTest;
    expectEq(w.numTimeouts, 0, 'numTimeouts');

    // If we create a new updater again, nothing should be sent.
    updater = new Updater();
    await updater.initialRetryDoneForTest;
    expectEq(w.numTimeouts, 0, 'numTimeouts');
  });

  // TODO: Test navigator.onLine property and 'online' events.
});
