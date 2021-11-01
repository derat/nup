// Copyright 2015 Daniel Erat.
// All rights reserved.

// Config provides persistent storage for preferences.
export default class Config {
  // Names to pass to get() or set().
  static THEME = 'theme';
  static GAIN_TYPE = 'gainType';
  static PRE_AMP = 'preAmp';

  // Values for THEME.
  static THEME_AUTO = 0;
  static THEME_LIGHT = 1;
  static THEME_DARK = 2;

  // Values for GAIN_TYPE.
  static GAIN_ALBUM = 0;
  static GAIN_TRACK = 1;
  static GAIN_NONE = 2;

  static CONFIG_KEY_ = 'config'; // localStorage key
  static FLOAT_NAMES_ = new Set([Config.PRE_AMP]);
  static INT_NAMES_ = new Set([Config.THEME, Config.GAIN_TYPE]);

  constructor() {
    this.callbacks_ = [];
    this.values_ = {
      [Config.THEME]: Config.THEME_AUTO,
      [Config.GAIN_TYPE]: Config.GAIN_ALBUM,
      [Config.PRE_AMP]: 0.0,
    };
    this.load_();
  }

  // Adds a function that will be invoked whenever a preference changes.
  //
  // |cb| will be invoked with two arguments: a string containing the pref's
  // name (see constants above) and an appropriately-typed second argument
  // containing the pref's value.
  addCallback(cb) {
    this.callbacks_.push(cb);
  }

  // Gets the value of the preference identified by |name|. An error is thrown
  // if an invalid name is supplied.
  get(name) {
    if (this.values_.hasOwnProperty(name)) return this.values_[name];
    throw new Error(`Unknown pref '${name}'`);
  }

  // Sets |name| to |value|. An error is thrown if an invalid name is supplied
  // or the value is of an inappropriate type.
  set(name, value) {
    const origValue = value;
    if (Config.FLOAT_NAMES_.has(name)) {
      value = parseFloat(value);
      if (isNaN(value)) throw new Error(`Non-float '${name}' '${origValue}'`);
      this.values_[name] = value;
    } else if (Config.INT_NAMES_.has(name)) {
      value = parseInt(value);
      if (isNaN(value)) throw new Error(`Non-int '${name}' '${origValue}'`);
      this.values_[name] = value;
    } else {
      throw new Error(`Unknown pref '${name}'`);
    }
    this.callbacks_.forEach((cb) => cb(name, value));
  }

  // Loads and validates prefs from local storage.
  load_() {
    const json = localStorage[Config.CONFIG_KEY_];
    if (!json) return;

    let loaded = {};
    try {
      loaded = JSON.parse(json);
    } catch (e) {
      console.error(`Ignoring bad config ${json}: ${e}`);
      return;
    }

    Object.entries(loaded).forEach(([name, value]) => {
      try {
        this.set(name, value);
      } catch (e) {
        console.error(`Skipping bad pref ${name}: ${e}`);
      }
    });
  }

  // Saves all prefs to local storage.
  save() {
    localStorage[Config.CONFIG_KEY_] = JSON.stringify(this.values_);
  }
}
