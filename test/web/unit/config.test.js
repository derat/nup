// Copyright 2021 Daniel Erat.
// All rights reserved.

import {
  afterEach,
  beforeEach,
  expectEq,
  expectThrows,
  suite,
  test,
} from './test.js';
import MockWindow from './mock-window.js';
import Config from './config.js';

suite('config', () => {
  let w = null;
  beforeEach(() => {
    w = new MockWindow();
  });
  afterEach(() => {
    w.finish();
  });

  test('saveAndReload', () => {
    let cfg = new Config();
    cfg.set(Config.THEME, Config.THEME_DARK);
    cfg.set(Config.GAIN_TYPE, Config.GAIN_NONE);
    cfg.set(Config.PRE_AMP, 0.3);
    cfg.save();

    cfg = new Config();
    expectEq(cfg.get(Config.THEME), Config.THEME_DARK, 'THEME');
    expectEq(cfg.get(Config.GAIN_TYPE), Config.GAIN_NONE, 'GAIN_TYPE');
    expectEq(cfg.get(Config.PRE_AMP), 0.3, 'PRE_AMP');
  });

  test('ignoreInvalidConfig', () => {
    window.localStorage.setItem(Config.CONFIG_KEY_, 'not json');

    // These match the defaults from the c'tor.
    const cfg = new Config();
    expectEq(cfg.get(Config.THEME), Config.THEME_AUTO);
    expectEq(cfg.get(Config.GAIN_TYPE), Config.GAIN_AUTO);
    expectEq(cfg.get(Config.PRE_AMP), 0);
  });

  test('skipInvalidPrefs', () => {
    // This matches load_().
    window.localStorage.setItem(
      Config.CONFIG_KEY_,
      JSON.stringify({
        bogus: 2.3,
        [Config.THEME]: Config.THEME_DARK,
      })
    );

    // We should still load the valid theme pref.
    const cfg = new Config();
    expectEq(cfg.get(Config.THEME), Config.THEME_DARK);
  });

  test('addCallback', () => {
    const cfg = new Config();
    const seen = []; // [name, value] pairs
    cfg.addCallback((n, v) => seen.push([n, v]));

    cfg.set(Config.THEME, Config.THEME_DARK);
    cfg.set(Config.GAIN_TYPE, Config.GAIN_NONE);
    cfg.set(Config.PRE_AMP, 0.3);
    expectEq(seen, [
      [Config.THEME, Config.THEME_DARK],
      [Config.GAIN_TYPE, Config.GAIN_NONE],
      [Config.PRE_AMP, 0.3],
    ]);
  });

  test('invalid', () => {
    const cfg = new Config();
    cfg.set(Config.THEME, Config.THEME_DARK);
    cfg.set(Config.PRE_AMP, 0.3);

    expectThrows(() => cfg.set(Config.THEME, 'abc'), 'Setting int to string');
    expectThrows(() => cfg.set(Config.THEME, null), 'Setting int to null');
    expectThrows(
      () => cfg.set(Config.THEME, undefined),
      'Setting int to undefined'
    );

    expectThrows(
      () => cfg.set(Config.PRE_AMP, 'abc'),
      'Setting float to string'
    );
    expectThrows(() => cfg.set(Config.PRE_AMP, null), 'Setting float to null');
    expectThrows(
      () => cfg.set(Config.PRE_AMP, undefined),
      'Setting float to undefined'
    );

    expectThrows(() => cfg.get('bogus'), 'Getting unknown pref');
    expectThrows(() => cfg.set('bogus', 2), 'Setting unknown pref');

    // The original values should be retained.
    expectEq(cfg.get(Config.THEME), Config.THEME_DARK);
    expectEq(cfg.get(Config.PRE_AMP), 0.3);
  });
});
