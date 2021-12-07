// Copyright 2021 Daniel Erat.
// All rights reserved.

import { error, fatal, suite, test } from './test.js';

// This suite contains example tests for exercising test.js.
suite('example', () => {
  test('sync', () => {
    //error('error');
    //fatal('fatal');
    //throw new Error('exception');
  });

  test('done', (done) => {
    //error('error');
    //fatal('fatal');
    //throw new Error('exception');
    //Promise.reject('reject');
    window.setTimeout(() => {
      console.log('Timeout fired');
      //error('error');
      //fatal('fatal');
      //throw new Error('exception');
      //Promise.reject('reject');
      done();
    }, 100);
  });

  test('async', async () => {
    //error('error');
    //fatal('fatal');
    //throw new Error('exception');
    //Promise.reject('reject');
    await new Promise((resolve, reject) => {
      window.setTimeout(() => {
        console.log('Timeout fired');
        //throw new Error('exception');
        //reject();
        resolve();
      }, 100);
    });
  });
});
