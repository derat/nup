// Copyright 2015 Daniel Erat.
// All rights reserved.

import { handleFetchError } from './common.js';

// localStorage keys.
const QUEUED_PLAYS = 'queued_plays';
const ACTIVE_PLAYS = 'active_plays';
const QUEUED_UPDATES = 'queued_updates';
const ACTIVE_UPDATES = 'active_updates';

const MIN_SEND_DELAY_MS = 500;
const MAX_SEND_DELAY_MS = 300 * 1000;

export default class Updater {
  #sendTimeoutId: number | null = null; // for #doSend()
  #lastSendDelayMs = 0; // used by #scheduleSend()
  #initialSendDone: Promise<void>;

  constructor() {
    // Move updates that were active during the last run into the queue.
    // TODO: Only do this if the old objects haven't been touched recently.
    // TODO: Is doing this one element at a time excessively slow?
    for (const play of readPlays(ACTIVE_PLAYS)) {
      addPlay(QUEUED_PLAYS, play.songId, play.startTime);
      removePlay(ACTIVE_PLAYS, play.songId, play.startTime);
    }
    for (const [songId, data] of Object.entries(readUpdates(ACTIVE_UPDATES))) {
      addUpdate(QUEUED_UPDATES, songId, data.rating, data.tags, false);
      removeUpdate(ACTIVE_UPDATES, songId);
    }

    // Start sending queued updates.
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
    addPlay(ACTIVE_PLAYS, songId, startTime);
    removePlay(QUEUED_PLAYS, songId, startTime);

    const url =
      `played?songId=${encodeURIComponent(songId)}` +
      `&startTime=${encodeURIComponent(startTime.toISOString())}`;
    console.log(`Reporting play: ${url}`);

    return fetch(url, { method: 'POST' })
      .then((res) => handleFetchError(res))
      .then(() => {
        // Success: remove it from active and try to send more.
        removePlay(ACTIVE_PLAYS, songId, startTime);
        this.#scheduleSend(true /* immediate */);
      })
      .catch((err) => {
        // Failed: move it from active to queued and schedule a retry.
        console.error(`Reporting to ${url} failed: ${err}`);
        addPlay(QUEUED_PLAYS, songId, startTime);
        removePlay(ACTIVE_PLAYS, songId, startTime);
        this.#scheduleSend(false /* immediate */);
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

    // If there's a queued update for the same song, incorporate its data
    // and remove it from the queued map.
    const queued = readUpdates(QUEUED_UPDATES)[songId];
    if (queued) {
      if (rating === null && queued.rating !== null) rating = queued.rating;
      if (tags === null && queued.tags !== null) tags = queued.tags;
      removeUpdate(QUEUED_UPDATES, songId);
    }

    // If there's an active update for the song, queue this update.
    if (readUpdates(ACTIVE_UPDATES)[songId]) {
      addUpdate(QUEUED_UPDATES, songId, rating, tags, true /* overwrite */);
      return Promise.resolve();
    }

    addUpdate(ACTIVE_UPDATES, songId, rating, tags, true /* overwrite */);
    let url = `rate_and_tag?songId=${encodeURIComponent(songId)}`;
    if (rating !== null) url += `&rating=${rating}`;
    if (tags !== null) url += `&tags=${encodeURIComponent(tags.join(' '))}`;
    console.log(`Rating/tagging song: ${url}`);
    return fetch(url, { method: 'POST' })
      .then((res) => handleFetchError(res))
      .then(() => {
        // Success: remove the update from the active map and immediately look
        // for more stuff to send.
        removeUpdate(ACTIVE_UPDATES, songId);
        this.#scheduleSend(true /* immediate */);
      })
      .catch((err) => {
        // Failure: queue the update and retry. If another update was queued in
        // the meantime, merge our data into it.
        console.log(`Rating/tagging to ${url} failed: ${err}`);
        addUpdate(QUEUED_UPDATES, songId, rating, tags, false /* overwrite */);
        removeUpdate(ACTIVE_UPDATES, songId);
        this.#scheduleSend(false /* immediate */);
      });
  }

  #onOnline = () => {
    // Automatically try to send queued updates when we come back online.
    this.#scheduleSend(true);
  };

  // Schedules a #doSend() call if needed.
  #scheduleSend(immediate: boolean) {
    // If we're not online, don't bother trying.
    // We'll be called again when the system comes back online.
    if (navigator.onLine === false) return;

    // Already scheduled.
    if (this.#sendTimeoutId) {
      if (!immediate) return;
      window.clearTimeout(this.#sendTimeoutId);
      this.#sendTimeoutId = null;
    }

    // Nothing to do.
    if (
      !readPlays(QUEUED_PLAYS).length &&
      !Object.keys(readUpdates(QUEUED_UPDATES)).length
    ) {
      return;
    }

    let delayMs = immediate
      ? 0
      : this.#lastSendDelayMs > 0
      ? this.#lastSendDelayMs * 2
      : MIN_SEND_DELAY_MS;
    delayMs = Math.min(delayMs, MAX_SEND_DELAY_MS);

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
      readPlays(ACTIVE_PLAYS).length ||
      Object.keys(readUpdates(ACTIVE_UPDATES)).length
    ) {
      this.#lastSendDelayMs = 0; // use min retry delay
      this.#scheduleSend(false);
      return Promise.resolve();
    }

    const update = Object.entries(readUpdates(QUEUED_UPDATES))[0] ?? null;
    if (update) {
      return this.rateAndTag(update[0], update[1].rating, update[1].tags);
    }

    const play = readPlays(QUEUED_PLAYS)[0] ?? null;
    if (play) return this.reportPlay(play.songId, new Date(play.startTime));

    return Promise.resolve();
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

// Reads the array of PlayReports at |key| in localStorage.
function readPlays(key: string): PlayReport[] {
  const value = localStorage.getItem(key);
  return value !== null ? JSON.parse(value) : [];
}

// Reads the SongUpdateMap at |key| in localStorage.
function readUpdates(key: string): SongUpdateMap {
  const value = localStorage.getItem(key);
  return value !== null ? JSON.parse(value) : {};
}

// Appends a play report to the array at |key| in localStorage.
function addPlay(key: string, songId: string, startTime: Date | string) {
  const isoTime = getIsoTime(startTime);
  const plays = readPlays(key);
  plays.push({ songId, startTime: isoTime });
  localStorage.setItem(key, JSON.stringify(plays));
}

// Removes the specified play report from the array at |key| in localStorage.
function removePlay(key: string, songId: string, startTime: Date | string) {
  const isoTime = getIsoTime(startTime);
  const plays = readPlays(key).filter(
    (p) => p.songId !== songId || p.startTime !== isoTime
  );
  localStorage.setItem(key, JSON.stringify(plays));
}

// Converts |time| to an ISO 8601 string if it isn't already a string.
function getIsoTime(time: Date | string): string {
  return typeof time === 'string' ? time : time.toISOString();
}

// Sets |songId|'s rating and tags in the map at |key| in localStorage.
// If |overwrite| is true, the new values are preferred if the song already has
// an entry; otherwise the existing entries are preferred.
function addUpdate(
  key: string,
  songId: string,
  rating: number | null,
  tags: string[] | null,
  overwrite: boolean
) {
  const updates = readUpdates(key);
  const update = (updates[songId] ||= { rating: null, tags: null });
  if (overwrite) {
    update.rating = rating ?? update.rating;
    update.tags = tags ?? update.tags;
  } else {
    update.rating = update.rating ?? rating;
    update.tags = update.tags ?? tags;
  }
  localStorage.setItem(key, JSON.stringify(updates));
}

// Removes |songId| from the map identified by |key| in localStorage.
function removeUpdate(key: string, songId: string) {
  const updates = readUpdates(key);
  delete updates[songId];
  localStorage.setItem(key, JSON.stringify(updates));
}
