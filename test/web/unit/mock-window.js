// Copyright 2021 Daniel Erat.
// All rights reserved.

import { error } from './test.js';

export default class MockWindow {
  constructor() {
    this.old_ = {}; // orginal window properties as name -> value
    this.fetches_ = {}; // resource -> array of Promises to hand out
    this.timeouts_ = {}; // id -> { func, delay }
    this.nextTimeoutId_ = 1;
    this.localStorage_ = {};
    this.listeners_ = {}; // event name -> array of funcs

    this.replace_('addEventListener', (type, func, capture) => {
      const ls = this.listeners_[type] || [];
      ls.push(func);
      this.listeners_[type] = ls;
    });

    this.replace_('setTimeout', (func, delay) => {
      const id = this.nextTimeoutId_++;
      this.timeouts_[id] = { func, delay: Math.max(delay, 0) };
      return id;
    });

    this.replace_('clearTimeout', (id) => {
      delete this.timeouts_[id];
    });

    this.replace_('fetch', (resource, init) => {
      const method = (init && init.method) || 'GET';
      const promise = this.getFetchPromise_(resource, method);
      if (!promise) {
        error(`Unexpected ${method} ${resource} fetch()`);
        return Promise.reject();
      }
      return promise;
    });

    this.replace_('localStorage', {
      getItem: (key) =>
        this.localStorage_.hasOwnProperty(key) ? this.localStorage_[key] : null,
      setItem: (key, value) => (this.localStorage_[key] = value),
      removeItem: (key) => delete this.localStorage_[key],
      clear: () => (this.localStorage_ = {}),
    });

    this.replace_('navigator', {
      onLine: true,
    });
  }

  // Restores the window object's original properties and verifies that
  // expectations were satisfied.
  finish() {
    Object.entries(this.old_).forEach(([name, value]) =>
      Object.defineProperty(window, name, { value })
    );
    Object.entries(this.fetches_).forEach(([key, promises]) => {
      error(`${promises.length} unsatisfied ${key} fetch()`);
    });
  }

  // Sets window.navigator.onLine to |v| and emits an 'online' or 'offline'
  // event if the state changed.
  set online(v) {
    if (v === window.navigator.onLine) return;
    window.navigator.onLine = v;
    this.emit(new Event(v ? 'online' : 'offline'));
  }

  // Emits event |ev| to all listeners registered for |ev.type|.
  emit(ev) {
    for (const f of this.listeners_[ev.type] || []) f(ev);
  }

  // Expects |resource| (a URL) to be fetched once via |method| (e.g. "POST").
  // |text| will be returned as the response body with an HTTP status code of
  // |status|.
  expectFetch(resource, method, text, status = 200) {
    const done = this.expectFetchDeferred(resource, method, text, status);
    done();
  }

  // Like expectFetch() but returns a function that must be run to resolve the
  // promise returned to the fetch() call.
  expectFetchDeferred(resource, method, text, status = 200) {
    let resolve = null;
    const promise = new Promise((r) => (resolve = r));

    const key = fetchKey(resource, method);
    const promises = this.fetches_[key] || [];
    promises.push(promise);
    this.fetches_[key] = promises;

    return () =>
      resolve({
        ok: status === 200,
        status,
        text: () => Promise.resolve(text),
        json: () => Promise.resolve(JSON.parse(text)),
      });
  }

  // Removes and returns the first promise from |fetches_| with the supplied
  // resource and method, or null if no matching promise is found.
  getFetchPromise_(resource, method) {
    const key = fetchKey(resource, method);
    const promises = this.fetches_[key];
    if (!promises || !promises.length) return null;

    const promise = promises[0];
    promises.splice(0, 1);
    if (!promises.length) delete this.fetches_[key];
    return promise;
  }

  // Number of fetch calls registered via expectFetch() that haven't been seen.
  get numUnsatisfiedFetches() {
    return Object.values(this.fetches_).reduce((s, f) => s + f.length, 0);
  }

  // Number of pending timeouts added via setTimeout().
  get numTimeouts() {
    return Object.keys(this.timeouts_).length;
  }

  // Advances time and runs timeouts that are scheduled to run within |millis|
  // seconds. If any timeouts returned promises, the promise returned by this
  // method will wait for them to be fulfilled.
  runTimeouts(millis) {
    // Advance by the minimum amount needed for the earliest timeout to fire.
    const advance = Math.min(
      millis,
      ...Object.values(this.timeouts_).map((i) => i.delay)
    );

    // Run all timeouts that are firing.
    // TODO: Should this also sort by ascending ID to break ties?
    const results = [];
    for (const [id, info] of Object.entries(this.timeouts_)) {
      info.delay -= advance;
      if (info.delay <= 0) {
        results.push(info.func());
        delete this.timeouts_[id];
      }
    }

    // If no timeouts fired, we're done.
    if (!results.length) return Promise.resolve();

    // Wait for the timeouts that we ran to finish, and then call ourselves
    // again with the remaining time (if any) to run the next round (which
    // might include new timeouts that were added in this round).
    return Promise.all(results).then(() => this.runTimeouts(millis - advance));
  }

  // Clears all scheduled timeouts.
  // This can be useful when simulating an object being recreated.
  clearTimeouts() {
    this.timeouts_ = {};
  }

  // Replaces the window property |name| with |val|.
  // The original value is restored in finish().
  replace_(name, value) {
    this.old_[name] = window[name];

    // This approach is needed for window.localStorage:
    // https://github.com/KaiSforza/mock-local-storage/issues/17
    Object.defineProperty(window, name, { value, configurable: true });
  }
}

function fetchKey(resource, method) {
  return `${method} ${resource}`;
}
