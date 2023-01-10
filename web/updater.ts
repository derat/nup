// Copyright 2015 Daniel Erat.
// All rights reserved.

import { handleFetchError, underTest } from './common.js';

// localStorage prefixes.
const QUEUED_PLAYS = 'queued_plays';
const ACTIVE_PLAYS = 'active_plays';
const QUEUED_UPDATES = 'queued_updates';
const ACTIVE_UPDATES = 'active_updates';
const LAST_ACTIVE = 'last_active';

const MIN_SEND_DELAY_MS = 500;
const MAX_SEND_DELAY_MS = 300 * 1000;
const ONLINE_SEND_DELAY_MS = 1000;

// Updater sends play reports and rating and tag updates to the server.
export default class Updater {
  #suffix = '.' + Math.random().toString().slice(2, 10).toString();
  #sendTimeoutId: number | null = null; // for #doSend()
  #lastSendDelayMs = 0; // used by #scheduleSend()
  #initialSendDone: Promise<void>;

  // Adopt records regardless of their age when running in tests.
  // This behavior is hard to inject since the play-view component creates an
  // Updater as soon as it's connected to the DOM.
  #minAdoptAgeMs = underTest() ? 0 : MAX_SEND_DELAY_MS;

  constructor() {
    console.log(`Starting updater with suffix ${this.#suffix}`);
    this.#adoptOldRecords();

    // Start sending adopted records.
    this.#initialSendDone = this.#doSend();
    window.addEventListener('online', this.#onOnline);
  }

  // Releases resources. Should be called if destroying the object.
  destroy() {
    if (this.#sendTimeoutId) window.clearTimeout(this.#sendTimeoutId);
    this.#sendTimeoutId = null;
    window.removeEventListener('online', this.#onOnline);
  }

  // Returns a promise that is resolved once the initial send attempt in the
  // constructor is completed.
  get initialSendDoneForTest() {
    return this.#initialSendDone;
  }

  // Asynchronously notifies the server that song |songId| was played starting
  // at |startTime|. Returns a promise that is resolved once the reporting
  // attempt is completed (possibly unsuccessfully).
  reportPlay(songId: string, startTime: Date): Promise<void> {
    // Move from queued (if present) to active.
    this.#addPlay(ACTIVE_PLAYS, songId, startTime);
    this.#removePlay(QUEUED_PLAYS, songId, startTime);

    const url =
      `played?songId=${encodeURIComponent(songId)}` +
      `&startTime=${encodeURIComponent(startTime.toISOString())}`;
    console.log(`Reporting play: ${url}`);

    return fetch(url, { method: 'POST' })
      .then((res) => handleFetchError(res))
      .then(() => {
        // Success: remove it from active and try to send more.
        this.#removePlay(ACTIVE_PLAYS, songId, startTime);
        this.#scheduleSend(0);
      })
      .catch((err) => {
        // Failed: move it from active to queued and schedule a retry.
        console.error(`Reporting to ${url} failed: ${err}`);
        this.#addPlay(QUEUED_PLAYS, songId, startTime);
        this.#removePlay(ACTIVE_PLAYS, songId, startTime);
        this.#scheduleSend();
      });
  }

  // Asynchronously notifies the server that song |songId| was given |rating|
  // (int in [1, 5] or 0 for unrated) and |tags| (string array). Either |rating|
  // or |tags| can be null to leave them unchanged. Returns a promise that is
  // resolved once the update attempt is completed (possibly unsuccessfully).
  rateAndTag(
    songId: string,
    rating: number | null,
    tags: string[] | null
  ): Promise<void> {
    if (rating === null && tags === null) return Promise.resolve();

    // If there's a queued update for the same song, incorporate its data.
    const queued = this.#readUpdates(QUEUED_UPDATES)[songId];
    if (queued) {
      rating = rating ?? queued.rating;
      tags = tags ?? queued.tags;
      this.#removeUpdate(QUEUED_UPDATES, songId);
    }

    // If there's an active update for the song, queue this update.
    if (this.#readUpdates(ACTIVE_UPDATES)[songId]) {
      this.#addUpdate(QUEUED_UPDATES, songId, rating, tags, true);
      return Promise.resolve();
    }

    this.#addUpdate(ACTIVE_UPDATES, songId, rating, tags, true);
    let url = `rate_and_tag?songId=${encodeURIComponent(songId)}`;
    if (rating !== null) url += `&rating=${rating}`;
    if (tags !== null) url += `&tags=${encodeURIComponent(tags.join(' '))}`;
    console.log(`Rating/tagging song: ${url}`);
    return fetch(url, { method: 'POST' })
      .then((res) => handleFetchError(res))
      .then(() => {
        // Success: remove the update from the active map and immediately look
        // for more stuff to send.
        this.#removeUpdate(ACTIVE_UPDATES, songId);
        this.#scheduleSend(0);
      })
      .catch((err) => {
        // Failure: queue the update and retry. If another update was queued in
        // the meantime, merge our data into it.
        console.log(`Rating/tagging to ${url} failed: ${err}`);
        this.#addUpdate(QUEUED_UPDATES, songId, rating, tags, false);
        this.#removeUpdate(ACTIVE_UPDATES, songId);
        this.#scheduleSend();
      });
  }

  #onOnline = () => {
    // Automatically try to send queued updates when we come back online.
    const delayMs = underTest() ? 0 : this.#getOnlineSendDelayMs();
    if (delayMs >= 0) this.#scheduleSend(delayMs);
    else console.log('Online, but not scheduling send');
  };

  // Schedules a #doSend() call if needed.
  // If |delayMs| is null, 2*|#lastSendDelayMs| is used.
  #scheduleSend(delayMs: number | null = null) {
    // If we're not online, don't bother trying.
    // We'll be called again when the system comes back online.
    if (navigator.onLine === false) return;

    // Already scheduled.
    if (this.#sendTimeoutId) {
      if (delayMs !== 0) return;
      window.clearTimeout(this.#sendTimeoutId);
      this.#sendTimeoutId = null;
    }

    // Periodically look for old records to adopt.
    this.#adoptOldRecords();

    // Nothing to do.
    if (
      !this.#readPlays(QUEUED_PLAYS).length &&
      !Object.keys(this.#readUpdates(QUEUED_UPDATES)).length
    ) {
      return;
    }

    delayMs ??= Math.min(
      Math.max(this.#lastSendDelayMs * 2, MIN_SEND_DELAY_MS),
      MAX_SEND_DELAY_MS
    );

    console.log(`Scheduling send in ${delayMs} ms`);
    this.#sendTimeoutId = window.setTimeout(() => {
      this.#sendTimeoutId = null;
      return this.#doSend();
    }, delayMs);
    this.#lastSendDelayMs = delayMs;
  }

  // Sends queued plays and updates to the server.
  #doSend() {
    // Already have an active update; try again in a bit.
    if (
      this.#readPlays(ACTIVE_PLAYS).length ||
      Object.keys(this.#readUpdates(ACTIVE_UPDATES)).length
    ) {
      this.#lastSendDelayMs = 0; // use min retry delay
      this.#scheduleSend();
      return Promise.resolve();
    }

    const update = Object.entries(this.#readUpdates(QUEUED_UPDATES))[0] ?? null;
    if (update) {
      return this.rateAndTag(update[0], update[1].rating, update[1].tags);
    }

    const play = this.#readPlays(QUEUED_PLAYS)[0] ?? null;
    if (play) return this.reportPlay(play.songId, new Date(play.startTime));

    return Promise.resolve();
  }

  // Incorporates old plays and updates from localStorage into QUEUED_PLAYS and
  // QUEUED_UPDATES.
  #adoptOldRecords() {
    for (const [key, iso] of Object.entries(localStorage)) {
      if (!key.startsWith(LAST_ACTIVE) || key.endsWith(this.#suffix)) continue;

      // Don't adopt the records if they're too recent.
      const suffix = key.slice(LAST_ACTIVE.length);
      const ageMs = new Date().getTime() - Date.parse(iso);
      if (ageMs < this.#minAdoptAgeMs) continue;

      for (const prefix of [QUEUED_PLAYS, ACTIVE_PLAYS]) {
        const key = prefix + suffix;
        const plays = this.#readPlays(prefix, suffix);
        if (plays.length) {
          console.log(`Adopting ${plays.length} play(s) from ${key} (${iso})`);
          this.#addPlays(QUEUED_PLAYS, plays);
        }
        localStorage.removeItem(key);
      }

      for (const prefix of [QUEUED_UPDATES, ACTIVE_UPDATES]) {
        const key = prefix + suffix;
        const updates = this.#readUpdates(prefix, suffix);
        const count = Object.keys(updates).length;
        if (count) {
          console.log(`Adopting ${count} updates(s) from ${key} (${iso})`);
          this.#addUpdates(QUEUED_UPDATES, updates, false);
        }
        localStorage.removeItem(key);
      }

      // Remove the last-active item too.
      localStorage.removeItem(key);
    }
  }

  // Reads an array of PlayReports from localStorage.
  #readPlays(prefix: string, suffix: string = this.#suffix): PlayReport[] {
    const value = localStorage.getItem(prefix + suffix);
    return value !== null ? JSON.parse(value) : [];
  }

  // Reads a SongUpdateMap from localStorage.
  #readUpdates(prefix: string, suffix: string = this.#suffix): SongUpdateMap {
    const value = localStorage.getItem(prefix + suffix);
    return value !== null ? JSON.parse(value) : {};
  }

  // Writes |obj| to localStorage. If null, the item is removed instead.
  #writeObject(prefix: string, obj: PlayReport[] | SongUpdateMap | null) {
    localStorage.setItem(LAST_ACTIVE + this.#suffix, new Date().toISOString());
    if (obj === null) localStorage.removeItem(prefix + this.#suffix);
    else localStorage.setItem(prefix + this.#suffix, JSON.stringify(obj));
  }

  // Saves |plays| to localStorage.
  #addPlays(prefix: string, plays: PlayReport[]) {
    const existing = this.#readPlays(prefix);
    existing.push(...plays);
    this.#writeObject(prefix, existing);
  }

  // Saves a single play report to localStorage.
  #addPlay(prefix: string, songId: string, startTime: Date | string) {
    if (typeof startTime !== 'string') startTime = startTime.toISOString();
    this.#addPlays(prefix, [{ songId, startTime }]);
  }

  // Removes a single play report from localStorage.
  #removePlay(prefix: string, songId: string, startTime: Date | string) {
    if (typeof startTime !== 'string') startTime = startTime.toISOString();
    const plays = this.#readPlays(prefix).filter(
      (p) => p.songId !== songId || p.startTime !== startTime
    );
    this.#writeObject(prefix, plays.length ? plays : null);
  }

  // Saves |updates| to localStorage.
  // If |overwrite| is true, the new values are preferred if a song already has
  // an entry; otherwise the existing entries are preferred.
  #addUpdates(prefix: string, updates: SongUpdateMap, overwrite: boolean) {
    const existing = this.#readUpdates(prefix);
    for (const [songId, { rating, tags }] of Object.entries(updates)) {
      const update = (existing[songId] ||= { rating: null, tags: null });
      if (overwrite) {
        update.rating = rating ?? update.rating;
        update.tags = tags ?? update.tags;
      } else {
        update.rating = update.rating ?? rating;
        update.tags = update.tags ?? tags;
      }
    }
    this.#writeObject(prefix, existing);
  }

  // Saves a single update to localStorage.
  #addUpdate(
    prefix: string,
    songId: string,
    rating: number | null,
    tags: string[] | null,
    overwrite: boolean
  ) {
    this.#addUpdates(prefix, { [songId]: { rating, tags } }, overwrite);
  }

  // Removes |songId|'s rating and/or tags update from localStorage.
  #removeUpdate(prefix: string, songId: string) {
    const updates = this.#readUpdates(prefix);
    delete updates[songId];
    this.#writeObject(prefix, Object.keys(updates).length ? updates : null);
  }

  // Returns time to wait before sending when the network goes online.
  // This tries to prevent different tabs from stepping on each others' toes.
  // Returns -1 if the updater should not automatically try to send.
  #getOnlineSendDelayMs(): number {
    // Find all of the instances by looking for last-active keys in
    // localStorage, and then choose an ordering by sorting.
    const instances = Object.keys(localStorage)
      .filter((k) => k.startsWith(LAST_ACTIVE))
      .map((k) => k.slice(LAST_ACTIVE.length))
      .sort();
    const index = instances.indexOf(this.#suffix);
    return index < 0 ? -1 : (index + 1) * ONLINE_SEND_DELAY_MS;
  }
}

// PlayReport represents a song being played at a specific time.
interface PlayReport {
  songId: string;
  startTime: string; // ISO 8601
}

// SongUpdate contains an update to a song's rating and/or tags.
interface SongUpdate {
  rating: number | null; // int in [1, 5] or 0 for unrated
  tags: string[] | null;
}

// SongUpdateMap holds multiple song updates keyed by song ID.
type SongUpdateMap = Record<string, SongUpdate>;
