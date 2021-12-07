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

  test('async', (done) => {
    //error('error');
    //fatal('fatal');
    //throw new Error('exception');

    window.setTimeout(() => {
      console.log('Running timeout');
      //error('error');
      //fatal('fatal');
      //throw new Error('exception');
      done();
    }, 100);
  });
});
