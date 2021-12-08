// Copyright 2021 Daniel Erat.
// All rights reserved.

import { error, fatal, suite, test } from './test.js';

// This suite contains example tests, some of which intentionally fail to
// exercise the error-handling code in test.js. web_test.go inspects the errors
// from these tests.
suite('example', () => {
  test('sync', () => {});
  test('syncFatal', () => {
    fatal('Intentional');
  });
  test('syncException', () => {
    throw new Error('Intentional');
  });

  test('async', async () => {
    await new Promise((resolve, reject) => {
      window.setTimeout(() => {
        console.log('Timeout fired');
        resolve();
      }, 100);
    });
  });
  test('asyncEarlyFatal', async () => {
    fatal('Intentional');
  });
  test('asyncEarlyException', async () => {
    throw new Error('Intentional');
  });
  test('asyncEarlyReject', async () => {
    return Promise.reject('Intentional');
  });
  test('asyncTimeoutFatal', async () => {
    await new Promise((resolve, reject) => fatal('Intentional'));
  });
  test('asyncTimeoutException', async () => {
    await new Promise((resolve, reject) => {
      throw new Error('Intentional');
    });
  });
  test('asyncTimeoutReject', async () => {
    await Promise.reject('Intentional');
  });

  test('done', (done) => {
    window.setTimeout(() => {
      console.log('Timeout fired');
      done();
    }, 100);
  });
  test('doneEarlyFatal', (done) => {
    fatal('Intentional');
  });
  test('doneEarlyException', (done) => {
    throw new Error('Intentional');
  });
  test('doneTimeoutFatal', (done) => {
    window.setTimeout(() => fatal('Intentional'));
  });
  test('doneTimeoutException', (done) =>
    window.setTimeout(() => {
      throw new Error('Intentional');
    }));
  test('doneTimeoutReject', (done) =>
    window.setTimeout(() => {
      Promise.reject('Intentional');
    }));
});
