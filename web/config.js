// Copyright 2015 Daniel Erat.
// All rights reserved.

// Config provides persistent storage for preferences.
export default class Config {
  // Names to pass to get() or set().
  VOLUME = 'volume';

  CONFIG_KEY = 'config'; // localStorage key

  constructor() {
    this.callbacks_ = [];
    this.floatNames_ = new Set([this.VOLUME]);
    this.values_ = {
      [this.VOLUME]: 0.7,
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
    if (this.floatNames_.has(name) != -1) {
      value = parseFloat(value);
      if (isNaN(value)) {
        throw new Error(`Non-float '${name}' value '${origValue}'`);
      }
      this.values_[name] = value;
    } else {
      throw new Error(`Unknown pref '${name}'`);
    }
    this.callbacks_.forEach(cb => cb(name, value));
  }

  // Loads and validates prefs from local storage.
  load_() {
    const json = localStorage[this.CONFIG_KEY];
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
    localStorage[this.CONFIG_KEY] = JSON.stringify(this.values_);
  }
}
