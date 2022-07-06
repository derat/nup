// Copyright 2015 Daniel Erat.
// All rights reserved.

// Names to pass to Config.get() or Config.set().
export enum Pref {
  THEME = 'theme',
  FULLSCREEN_MODE = 'fullscreenMode',
  GAIN_TYPE = 'gainType',
  PRE_AMP = 'preAmp',
}

// Values for Pref.THEME.
export enum Theme {
  AUTO = 0,
  LIGHT = 1,
  DARK = 2,
}

// Values for Pref.FULLSCREEN_MODE.
export enum FullscreenMode {
  SCREEN = 0,
  WINDOW = 1,
}

// Values for Pref.GAIN_TYPE.
export enum GainType {
  ALBUM = 0,
  TRACK = 1,
  NONE = 2,
  AUTO = 3,
}

// localStorage key; exported for tests.
export const ConfigKey = 'config';

const FLOAT_NAMES = new Set([Pref.PRE_AMP]);
const INT_NAMES = new Set([Pref.THEME, Pref.FULLSCREEN_MODE, Pref.GAIN_TYPE]);

// Config provides persistent storage for preferences.
export class Config {
  #callbacks: ConfigCallback[] = [];
  #values: Record<Pref, any> = {
    [Pref.THEME]: Theme.AUTO,
    [Pref.FULLSCREEN_MODE]: FullscreenMode.SCREEN,
    [Pref.GAIN_TYPE]: GainType.AUTO,
    [Pref.PRE_AMP]: 0,
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
  get(name: Pref): any {
    if (this.#values.hasOwnProperty(name)) return this.#values[name];
    throw new Error(`Unknown pref "${name}"`);
  }

  // Sets |name| to |value|. An error is thrown if an invalid name is supplied
  // or the value is of an inappropriate type.
  set(name: Pref, value: any) {
    let parsed = 0;
    if (FLOAT_NAMES.has(name)) {
      parsed = parseFloat(value);
      if (isNaN(parsed)) throw new Error(`Non-float "${name}" "${value}"`);
    } else if (INT_NAMES.has(name)) {
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
    const json = localStorage.getItem(ConfigKey);
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
        this.set(name as Pref, value);
      } catch (e) {
        console.error(`Skipping bad pref "${name}": ${e}`);
      }
    });
  }

  // Saves all prefs to local storage.
  save() {
    localStorage.setItem(ConfigKey, JSON.stringify(this.#values));
  }
}

type ConfigCallback = (name: Pref, value: number) => void;

let defaultConfig: Config | null = null;

// Returns a default singleton Config instance.
export function getConfig(): Config {
  if (!defaultConfig) defaultConfig = new Config();
  return defaultConfig;
}
