// Copyright 2021 Daniel Erat.
// All rights reserved.

import { error } from './test.js';

export default class MockWindow {
  constructor() {
    this.old_ = {}; // orginal window properties as name -> value
    this.fetches_ = {}; // resource -> { method, text, status }
    this.timeouts_ = {}; // id -> { func, delay }
    this.nextTimeoutId_ = 1;

    this.replace_('addEventListener', (type, func, capture) => {
      // TODO: Implement this.
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
      const info = this.getFetch_(resource, method);
      if (!info) {
        error(`${method} ${resource} fetch() unexpected`);
        return Promise.reject();
      }
      return Promise.resolve({
        ok: info.status === 200,
        status: info.status,
        text: () => Promise.resolve(info.text),
        json: () => Promise.resolve(JSON.parse(info.text)),
      });
    });
  }

  // Restores the window object's original properties and verifies that
  // expectations were satisfied.
  finish() {
    Object.entries(this.old_).forEach(([n, f]) => (window[n] = f));
    Object.entries(this.fetches_).forEach(([resource, infos]) => {
      for (const info of infos) {
        error(`${method} ${resource} fetch() unsatisfied`);
      }
    });
  }

  // Expects |resource| (a URL) to be fetched once via |method| (e.g. "POST").
  // |text| will be returned as the response body.
  expectFetch(resource, method, text, status = 200) {
    const infos = this.fetches_[resource] || [];
    infos.push({ method, text, status });
    this.fetches_[resource] = infos;
  }

  // Removes and returns the first info from |fetches_| with the supplied
  // resource and method, or null if no matching info is found.
  getFetch_(resource, method) {
    const infos = this.fetches_[resource];
    if (!infos) return null;

    // TODO: Just check the first request to verify order.
    for (const [i, info] of infos.entries()) {
      if (info.method === method) {
        infos.splice(i, 1);
        return info;
      }
    }
    return null;
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

  // Replaces the window property |name| with |val|.
  // The original value is restored in finish().
  replace_(name, val) {
    this.old_[name] = window[name];
    window[name] = val;
  }
}
