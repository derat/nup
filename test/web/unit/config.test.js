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
import { Config, ConfigKey, GainType, Pref, Theme } from './config.js';

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
    cfg.set(Pref.THEME, Theme.DARK);
    cfg.set(Pref.GAIN_TYPE, GainType.NONE);
    cfg.set(Pref.PRE_AMP, 0.3);
    cfg.save();

    cfg = new Config();
    expectEq(cfg.get(Pref.THEME), Theme.DARK, 'Theme');
    expectEq(cfg.get(Pref.GAIN_TYPE), GainType.NONE, 'GainType');
    expectEq(cfg.get(Pref.PRE_AMP), 0.3, 'PreAmp');
  });

  test('ignoreInvalidConfig', () => {
    window.localStorage.setItem(ConfigKey, 'not json');

    // These match the defaults from the c'tor.
    const cfg = new Config();
    expectEq(cfg.get(Pref.THEME), Theme.AUTO);
    expectEq(cfg.get(Pref.GAIN_TYPE), GainType.AUTO);
    expectEq(cfg.get(Pref.PRE_AMP), 0);
  });

  test('skipInvalidPrefs', () => {
    // This matches #load().
    window.localStorage.setItem(
      ConfigKey,
      JSON.stringify({
        bogus: 2.3,
        [Pref.THEME]: Theme.DARK,
      })
    );

    // We should still load the valid theme pref.
    const cfg = new Config();
    expectEq(cfg.get(Pref.THEME), Theme.DARK);
  });

  test('addCallback', () => {
    const cfg = new Config();
    const seen = []; // [name, value] pairs
    cfg.addCallback((n, v) => seen.push([n, v]));

    cfg.set(Pref.THEME, Theme.DARK);
    cfg.set(Pref.GAIN_TYPE, GainType.NONE);
    cfg.set(Pref.PRE_AMP, 0.3);
    expectEq(seen, [
      [Pref.THEME, Theme.DARK],
      [Pref.GAIN_TYPE, GainType.NONE],
      [Pref.PRE_AMP, 0.3],
    ]);
  });

  test('invalid', () => {
    const cfg = new Config();
    cfg.set(Pref.THEME, Theme.DARK);
    cfg.set(Pref.PRE_AMP, 0.3);

    expectThrows(() => cfg.set(Pref.THEME, 'abc'), 'Setting int to string');
    expectThrows(() => cfg.set(Pref.THEME, null), 'Setting int to null');
    expectThrows(
      () => cfg.set(Pref.THEME, undefined),
      'Setting int to undefined'
    );

    expectThrows(() => cfg.set(Pref.PRE_AMP, 'abc'), 'Setting float to string');
    expectThrows(() => cfg.set(Pref.PRE_AMP, null), 'Setting float to null');
    expectThrows(
      () => cfg.set(Pref.PRE_AMP, undefined),
      'Setting float to undefined'
    );

    expectThrows(() => cfg.get('bogus'), 'Getting unknown pref');
    expectThrows(() => cfg.set('bogus', 2), 'Setting unknown pref');

    // The original values should be retained.
    expectEq(cfg.get(Pref.THEME), Theme.DARK);
    expectEq(cfg.get(Pref.PRE_AMP), 0.3);
  });
});
