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
  static GAIN_AUTO = 3;

  static CONFIG_KEY = 'config'; // localStorage key; public for tests
  static #FLOAT_NAMES = new Set([Config.PRE_AMP]);
  static #INT_NAMES = new Set([Config.THEME, Config.GAIN_TYPE]);

  #callbacks: ConfigCallback[] = [];
  #values: Record<string, number> = {
    [Config.THEME]: Config.THEME_AUTO,
    [Config.GAIN_TYPE]: Config.GAIN_AUTO,
    [Config.PRE_AMP]: 0,
  };

  constructor() {
    this.#load();
  }

  // Adds a function that will be invoked whenever a preference changes.
  //
  // |cb| will be invoked with two arguments: a string containing the pref's
  // name (see constants above) and an appropriately-typed second argument
  // containing the pref's value.
  addCallback(cb: ConfigCallback) {
    this.#callbacks.push(cb);
  }

  // Gets the value of the preference identified by |name|. An error is thrown
  // if an invalid name is supplied.
  get(name: string): number {
    if (this.#values.hasOwnProperty(name)) return this.#values[name];
    throw new Error(`Unknown pref "${name}"`);
  }

  // Sets |name| to |value|. An error is thrown if an invalid name is supplied
  // or the value is of an inappropriate type.
  set(name: string, value: any) {
    let parsed = 0;
    if (Config.#FLOAT_NAMES.has(name)) {
      parsed = parseFloat(value);
      if (isNaN(parsed)) throw new Error(`Non-float "${name}" "${value}"`);
    } else if (Config.#INT_NAMES.has(name)) {
      parsed = parseInt(value);
      if (isNaN(parsed)) throw new Error(`Non-int "${name}" "${value}"`);
    } else {
      throw new Error(`Unknown pref "${name}"`);
    }
    this.#values[name] = parsed;
    this.#callbacks.forEach((cb) => cb(name, parsed));
  }

  // Loads and validates prefs from local storage.
  #load() {
    const json = localStorage.getItem(Config.CONFIG_KEY);
    if (!json) return;

    let loaded = {};
    try {
      loaded = JSON.parse(json);
    } catch (e) {
      console.error(`Ignoring bad config ${JSON.stringify(json)}: ${e}`);
      return;
    }

    Object.entries(loaded).forEach(([name, value]) => {
      try {
        this.set(name, value);
      } catch (e) {
        console.error(`Skipping bad pref "${name}": ${e}`);
      }
    });
  }

  // Saves all prefs to local storage.
  save() {
    localStorage.setItem(Config.CONFIG_KEY, JSON.stringify(this.#values));
  }
}

type ConfigCallback = (name: string, value: number) => void;

let defaultConfig: Config | null = null;

// Returns a default singleton Config instance.
export function getConfig(): Config {
  if (!defaultConfig) defaultConfig = new Config();
  return defaultConfig;
}
