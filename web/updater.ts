// Copyright 2015 Daniel Erat.
// All rights reserved.

import { handleFetchError } from './common.js';

const QUEUED_PLAY_REPORTS_KEY = 'queued_play_reports';
const IN_PROGRESS_PLAY_REPORTS_KEY = 'in_progress_play_reports';
const QUEUED_RATINGS_AND_TAGS_KEY = 'queued_ratings_and_tags';
const IN_PROGRESS_RATINGS_AND_TAGS_KEY = 'in_progress_ratings_and_tags';
const MIN_RETRY_DELAY_MS = 500;
const MAX_RETRY_DELAY_MS = 300 * 1000;

export default class Updater {
  #retryTimeoutId: number | null = null; // for #doRetry()
  #lastRetryDelayMs = 0; // used by #scheduleRetry()

  #queuedPlayReports = readObject(QUEUED_PLAY_REPORTS_KEY, []) as PlayReport[];

  #queuedRatingsAndTags = readObject(
    QUEUED_RATINGS_AND_TAGS_KEY,
    {}
  ) as SongUpdateMap;

  #inProgressPlayReports: PlayReport[] = [];
  #inProgressRatingsAndTags: SongUpdateMap = {};

  #initialRetryDone: Promise<void>;

  constructor() {
    // Move updates that were in-progress during the last run into the queue.
    for (const play of readObject(
      IN_PROGRESS_PLAY_REPORTS_KEY,
      []
    ) as PlayReport[]) {
      this.#queuedPlayReports.push(play);
    }

    for (const [songId, data] of Object.entries(
      readObject(IN_PROGRESS_RATINGS_AND_TAGS_KEY, {}) as SongUpdateMap
    )) {
      this.#queuedRatingsAndTags[songId] = data;
    }

    this.#writeState();

    // Start sending queued updates.
    this.#initialRetryDone = this.#doRetry();

    window.addEventListener('online', this.#onOnline);
  }

  // Releases resources. Should be called if destroying the object.
  destroy() {
    if (this.#retryTimeoutId) window.clearTimeout(this.#retryTimeoutId);
    this.#retryTimeoutId = null;

    window.removeEventListener('online', this.#onOnline);
  }

  // Returns a promise that is resolved once the initial retry attempt in the
  // constructor is completed.
  get initialRetryDoneForTest() {
    return this.#initialRetryDone;
  }

  // Asynchronously notifies the server that song |songId| was played starting
  // at |startTime| seconds since the Unix expoch. Returns a promise that is
  // resolved once the reporting attempt is completed (possibly unsuccessfully).
  reportPlay(songId: string, startTime: number): Promise<void> {
    // Move from queued (if present) to in-progress.
    removePlayReport(this.#queuedPlayReports, songId, startTime);
    addPlayReport(this.#inProgressPlayReports, songId, startTime);
    this.#writeState();

    const url =
      `played?songId=${encodeURIComponent(songId)}` +
      `&startTime=${encodeURIComponent(startTime)}`;
    console.log(`Reporting play: ${url}`);

    return fetch(url, { method: 'POST' })
      .then((res) => handleFetchError(res))
      .then(() => {
        removePlayReport(this.#inProgressPlayReports, songId, startTime);
        this.#writeState();
        this.#scheduleRetry(true /* immediate */);
      })
      .catch((err) => {
        console.error(`Reporting to ${url} failed: ${err}`);
        removePlayReport(this.#inProgressPlayReports, songId, startTime);
        addPlayReport(this.#queuedPlayReports, songId, startTime);
        this.#writeState();
        this.#scheduleRetry(false /* immediate */);
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

    // Handle the case where there's a queued rating and we're only updating
    // tags, or queued tags and we're only updating the rating.
    const queued = this.#queuedRatingsAndTags[songId];
    if (queued) {
      if (rating === null && queued.rating !== null) rating = queued.rating;
      if (tags === null && queued.tags !== null) tags = queued.tags;
      delete this.#queuedRatingsAndTags[songId];
    }

    if (this.#inProgressRatingsAndTags.hasOwnProperty(songId)) {
      addRatingAndTags(this.#queuedRatingsAndTags, songId, rating, tags);
      return Promise.resolve();
    }

    addRatingAndTags(this.#inProgressRatingsAndTags, songId, rating, tags);
    this.#writeState();

    let url = `rate_and_tag?songId=${encodeURIComponent(songId)}`;
    if (rating !== null) url += `&rating=${rating}`;
    if (tags !== null) url += `&tags=${encodeURIComponent(tags.join(' '))}`;
    console.log(`Rating/tagging song: ${url}`);

    return fetch(url, { method: 'POST' })
      .then((res) => handleFetchError(res))
      .then(() => {
        delete this.#inProgressRatingsAndTags[songId];
        this.#writeState();
        this.#scheduleRetry(true /* immediate */);
      })
      .catch((err) => {
        console.log(`Rating/tagging to ${url} failed: ${err}`);
        delete this.#inProgressRatingsAndTags[songId];

        // If another update was queued in the meantime, don't overwrite it.
        const queued = this.#queuedRatingsAndTags[songId];
        if (queued) {
          if (queued.rating === null && rating !== null) queued.rating = rating;
          if (queued.tags === null && tags !== null) queued.tags = tags;
        } else {
          addRatingAndTags(this.#queuedRatingsAndTags, songId, rating, tags);
        }

        this.#writeState();
        this.#scheduleRetry(false /* immediate */);
      });
  }

  #onOnline = () => {
    // Automatically try to send queued updates when we come back online.
    this.#scheduleRetry(true);
  };

  // Persists the current state to local storage.
  #writeState() {
    localStorage.setItem(
      QUEUED_PLAY_REPORTS_KEY,
      JSON.stringify(this.#queuedPlayReports)
    );
    localStorage.setItem(
      QUEUED_RATINGS_AND_TAGS_KEY,
      JSON.stringify(this.#queuedRatingsAndTags)
    );
    localStorage.setItem(
      IN_PROGRESS_PLAY_REPORTS_KEY,
      JSON.stringify(this.#inProgressPlayReports)
    );
    localStorage.setItem(
      IN_PROGRESS_RATINGS_AND_TAGS_KEY,
      JSON.stringify(this.#inProgressRatingsAndTags)
    );
  }

  // Schedules a #doRetry() call if needed.
  #scheduleRetry(immediate: boolean) {
    // If we're not online, don't bother trying.
    // We'll be called again when the system comes back online.
    if (navigator.onLine === false) return;

    // Already scheduled.
    if (this.#retryTimeoutId) {
      if (!immediate) return;
      window.clearTimeout(this.#retryTimeoutId);
      this.#retryTimeoutId = null;
    }

    // Nothing to do.
    if (
      !this.#queuedPlayReports.length &&
      !Object.keys(this.#queuedRatingsAndTags).length
    ) {
      return;
    }

    let delayMs = immediate
      ? 0
      : this.#lastRetryDelayMs > 0
      ? this.#lastRetryDelayMs * 2
      : MIN_RETRY_DELAY_MS;
    delayMs = Math.min(delayMs, MAX_RETRY_DELAY_MS);

    console.log(`Scheduling retry in ${delayMs} ms`);
    this.#retryTimeoutId = window.setTimeout(() => {
      this.#retryTimeoutId = null;
      return this.#doRetry();
    }, delayMs);
    this.#lastRetryDelayMs = delayMs;
  }

  // Sends queued plays and ratings/tags to the server.
  #doRetry() {
    // Already have an in-progress update; try again in a bit.
    if (
      this.#inProgressPlayReports.length ||
      Object.keys(this.#inProgressRatingsAndTags).length
    ) {
      this.#lastRetryDelayMs = 0; // use min retry delay
      this.#scheduleRetry(false);
      return Promise.resolve();
    }

    if (Object.keys(this.#queuedRatingsAndTags).length) {
      const songId = Object.keys(this.#queuedRatingsAndTags)[0];
      const entry = this.#queuedRatingsAndTags[songId];
      return this.rateAndTag(songId, entry.rating, entry.tags);
    }

    if (this.#queuedPlayReports.length) {
      const entry = this.#queuedPlayReports[0];
      return this.reportPlay(entry.songId, entry.startTime);
    }

    return Promise.resolve();
  }
}

interface PlayReport {
  songId: string;
  startTime: number;
}

interface SongUpdate {
  rating: number | null;
  tags: string[] | null;
}

type SongUpdateMap = Record<string, SongUpdate>;

// Reads |key| from local storage and parses it as JSON.
// |defaultObject| is returned if the key is unset.
function readObject(key: string, defaultObject: Object) {
  const value = localStorage.getItem(key);
  return value !== null ? JSON.parse(value) : defaultObject;
}

// Appends a play report to |list|.
function addPlayReport(list: PlayReport[], songId: string, startTime: number) {
  list.push({ songId, startTime });
}

// Removes the specified play report from |list|.
function removePlayReport(
  list: PlayReport[],
  songId: string,
  startTime: number
) {
  for (let i = 0; i < list.length; i++) {
    if (list[i].songId === songId && list[i].startTime === startTime) {
      list.splice(i, 1);
      return;
    }
  }
}

// Sets |songId|'s rating and tags within |map|.
function addRatingAndTags(
  map: SongUpdateMap,
  songId: string,
  rating: number | null,
  tags: string[] | null
) {
  map[songId] = { rating: rating, tags: tags };
}
