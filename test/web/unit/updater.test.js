// Copyright 2021 Daniel Erat.
// All rights reserved.

import { afterEach, beforeEach, expectEq, suite, test } from './test.js';
import MockWindow from './mock-window.js';
import Updater from './updater.js';

suite('updater', () => {
  let w = null;
  let updater = null;

  const t1 = new Date('2021-04-05T23:12:05.234Z');
  const t2 = new Date('2021-04-06T12:05:03Z');
  const t3 = new Date('2021-04-07T13:45:12.5Z');

  beforeEach(() => {
    w = new MockWindow();
  });
  afterEach(() => {
    updater?.destroy();
    updater = null;
    w.finish();
  });

  function initUpdater() {
    updater?.destroy();
    updater = new Updater();
  }
  function playedUrl(songId, startTime) {
    return (
      `played?songId=${encodeURIComponent(songId)}` +
      `&startTime=${encodeURIComponent(startTime.toISOString())}`
    );
  }
  function rateAndTagUrl(songId, rating, tags) {
    let url = `rate_and_tag?songId=${songId}`;
    if (rating != null) url += `&rating=${rating}`;
    if (tags != null) url += `&tags=${encodeURIComponent(tags.join(' '))}`;
    return url;
  }

  test('reportPlay (success)', async () => {
    initUpdater();
    w.expectFetch(playedUrl('123', t1), 'POST', 'ok');
    await updater.reportPlay('123', t1);
    expectEq(w.numTimeouts, 0, 'numTimeouts');
  });

  test('reportPlay (retry)', async () => {
    initUpdater();

    // Report a play and have the server return a 500 error.
    const id1 = '123';
    w.expectFetch(playedUrl(id1, t1), 'POST', 'whoops', 500);
    await updater.reportPlay(id1, t1);

    // Report a second play and have it also fail.
    const id2 = '456';
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
    initUpdater();
    w.expectFetch(playedUrl('1', t1), 'POST', 'whoops', 500);
    await updater.reportPlay('1', t1);

    // The retry time should double up to 5 minutes.
    for (const ms of [
      500, 1_000, 2_000, 4_000, 8_000, 16_000, 32_000, 64_000, 128_000, 256_000,
      300_000, 300_000, 300_000,
    ]) {
      w.expectFetch(playedUrl('1', t1), 'POST', 'fail', 500);
      await w.runTimeouts(ms);
    }

    // Try to report a second play and check that it doesn't reset the delay.
    w.expectFetch(playedUrl('1', t2), 'POST', 'fail', 500);
    await updater.reportPlay('1', t2);
    expectEq(w.numUnsatisfiedFetches, 0, 'Unsatisfied fetches');
    await w.runTimeouts(299_000);

    // Wait the final second and let the next attempt succeed.
    w.expectFetch(playedUrl('1', t1), 'POST', 'ok');
    w.expectFetch(playedUrl('1', t2), 'POST', 'ok');
    await w.runTimeouts(1_000);

    // Report another play and check that it's sent immediately.
    w.expectFetch(playedUrl('1', t3), 'POST', 'ok');
    await updater.reportPlay('1', t3);
    expectEq(w.numTimeouts, 0, 'numTimeouts');
  });

  test('reportPlay (retry at startup)', async () => {
    // Make a playback report fail.
    const id = '1';
    initUpdater();
    w.expectFetch(playedUrl(id, t1), 'POST', 'fail', 500);
    await updater.reportPlay(id, t1);

    // Report a second playback, but leave the fetch() hanging. This should
    // leave the playback in the "in-progress" list in localStorage.
    w.expectFetchDeferred(playedUrl(id, t2), 'POST', 'fail', 500);
    updater.reportPlay(id, t2);

    // Clear timeouts to make sure the old updater isn't doing anything and
    // create a new updater. It should pick up both old reports from
    // localStorage and try to send the first one again, which again fails (and
    // gets moved to the end of the queue this time).
    w.clearTimeouts();
    w.expectFetch(playedUrl(id, t1), 'POST', 'fail', 500);
    initUpdater();
    await updater.initialRetryDoneForTest;

    // After creating another updater, it should send the second report first
    // and then the first one.
    w.clearTimeouts();
    w.expectFetch(playedUrl(id, t2), 'POST', 'ok');
    initUpdater();
    await updater.initialRetryDoneForTest;

    w.expectFetch(playedUrl(id, t1), 'POST', 'ok');
    await w.runTimeouts(0);
    expectEq(w.numTimeouts, 0, 'numTimeouts');

    // If we create a new updater again, nothing should be sent.
    initUpdater();
    await updater.initialRetryDoneForTest;
    expectEq(w.numTimeouts, 0, 'numTimeouts');
  });

  test('reportPlay (overlapping)', async () => {
    // Report a play, but leave the fetch() call hanging.
    const id = '1';
    initUpdater();
    const finishFetch = w.expectFetchDeferred(playedUrl(id, t1), 'POST', 'ok');
    const reportDone = updater.reportPlay(id, t1);

    // Successfully report a second play in the meantime.
    w.expectFetch(playedUrl(id, t2), 'POST', 'ok');
    await updater.reportPlay(id, t2);

    // Let the first fetch() finish.
    finishFetch();
    await reportDone;
    expectEq(w.numTimeouts, 0, 'numTimeouts');

    // If we create a new updater, nothing should be sent.
    initUpdater();
    await updater.initialRetryDoneForTest;
    expectEq(w.numTimeouts, 0, 'numTimeouts');
  });

  test('reportPlay (online/offline)', async () => {
    // Make the initial attempt file.
    const id = '1';
    initUpdater();
    w.expectFetch(playedUrl(id, t1), 'POST', 'fail', 500);
    await updater.reportPlay(id, t1);

    // If we're offline when the retry happens, another retry shouldn't be
    // scheduled.
    w.online = false;
    w.expectFetch(playedUrl(id, t1), 'POST', 'fail', 500);
    await w.runTimeouts(500);
    expectEq(w.numTimeouts, 0, 'numTimeouts');

    // As soon as we come back online, an immediate retry should be scheduled.
    w.expectFetch(playedUrl(id, t1), 'POST');
    w.online = true;
    await w.runTimeouts(0);
  });

  test('rateAndTag (success)', async () => {
    initUpdater();
    w.expectFetch(rateAndTagUrl('123', 4, null), 'POST', 'ok');
    await updater.rateAndTag('123', 4, null);
    w.expectFetch(rateAndTagUrl('123', null, ['abc', 'def']), 'POST', 'ok');
    await updater.rateAndTag('123', null, ['abc', 'def']);
    w.expectFetch(rateAndTagUrl('123', 5, ['ijk']), 'POST', 'ok');
    await updater.rateAndTag('123', 5, ['ijk']);
    expectEq(w.numTimeouts, 0, 'numTimeouts');
  });

  test('rateAndTag (retry)', async () => {
    initUpdater();

    // Rate and tag a song and have the server report failure.
    w.expectFetch(rateAndTagUrl('123', 2, ['old']), 'POST', 'bad', 500);
    await updater.rateAndTag('123', 2, ['old']);

    // Try to send an updated rating and tag for the same song.
    w.expectFetch(rateAndTagUrl('123', 4, ['new']), 'POST', 'bad', 500);
    await updater.rateAndTag('123', 4, ['new']);

    // Send a rating and tag for another song.
    w.expectFetch(rateAndTagUrl('456', 5, ['other']), 'POST', 'bad', 500);
    await updater.rateAndTag('456', 5, ['other']);

    // After a 500 ms delay, the latest data for each song should be sent.
    w.expectFetch(rateAndTagUrl('123', 4, ['new']), 'POST', 'ok');
    w.expectFetch(rateAndTagUrl('456', 5, ['other']), 'POST', 'ok');
    await w.runTimeouts(500);
    expectEq(w.numTimeouts, 0, 'numTimeouts');
  });

  test('rateAndTag (retry at startup)', async () => {
    // Make the initial attempt fail.
    initUpdater();
    w.expectFetch(rateAndTagUrl('123', 5, ['tag']), 'POST', 'bad', 500);
    await updater.rateAndTag('123', 5, ['tag']);

    // Send a second request, but leave the fetch() hanging. This should leave
    // the update in the "in-progress" list in localStorage.
    w.expectFetchDeferred(rateAndTagUrl('456', 1, ['a']), 'POST', 'fail', 500);
    updater.rateAndTag('456', 1, ['a']);

    // Fail again with a new updater.
    w.clearTimeouts();
    w.expectFetch(rateAndTagUrl('123', 5, ['tag']), 'POST', 'bad', 500);
    initUpdater();
    await updater.initialRetryDoneForTest;

    // Create another updater and let both updates get sent successfully.
    w.clearTimeouts();
    w.expectFetch(rateAndTagUrl('123', 5, ['tag']), 'POST', 'ok');
    w.expectFetch(rateAndTagUrl('456', 1, ['a']), 'POST', 'ok');
    initUpdater();
    await updater.initialRetryDoneForTest;
    await w.runTimeouts(0);
    expectEq(w.numTimeouts, 0, 'numTimeouts');

    // If we create a new updater again, nothing should be sent.
    initUpdater();
    await updater.initialRetryDoneForTest;
    expectEq(w.numTimeouts, 0, 'numTimeouts');
  });
});
