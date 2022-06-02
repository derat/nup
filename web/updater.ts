// Copyright 2015 Daniel Erat.
// All rights reserved.

import { handleFetchError } from './common.js';

export default class Updater {
  static QUEUED_PLAY_REPORTS_KEY_ = 'queued_play_reports';
  static IN_PROGRESS_PLAY_REPORTS_KEY_ = 'in_progress_play_reports';
  static QUEUED_RATINGS_AND_TAGS_KEY_ = 'queued_ratings_and_tags';
  static IN_PROGRESS_RATINGS_AND_TAGS_KEY_ = 'in_progress_ratings_and_tags';

  static MIN_RETRY_DELAY_MS_ = 500; // Half a second.
  static MAX_RETRY_DELAY_MS_ = 300 * 1000; // Five minutes.

  retryTimeoutId_: number | null = null; // for doRetry_()
  lastRetryDelayMs_ = 0; // used by scheduleRetry_()

  queuedPlayReports_ = readObject(
    Updater.QUEUED_PLAY_REPORTS_KEY_,
    []
  ) as PlayReport[];

  queuedRatingsAndTags_ = readObject(
    Updater.QUEUED_RATINGS_AND_TAGS_KEY_,
    {}
  ) as SongUpdateMap;

  inProgressPlayReports_: PlayReport[] = [];
  inProgressRatingsAndTags_: SongUpdateMap = {};

  initialRetryDone_: Promise<void>;

  constructor() {
    // Move updates that were in-progress during the last run into the queue.
    for (const play of readObject(
      Updater.IN_PROGRESS_PLAY_REPORTS_KEY_,
      []
    ) as PlayReport[]) {
      this.queuedPlayReports_.push(play);
    }

    for (const [songId, data] of Object.entries(
      readObject(Updater.IN_PROGRESS_RATINGS_AND_TAGS_KEY_, {}) as SongUpdateMap
    )) {
      this.queuedRatingsAndTags_[songId] = data;
    }

    this.writeState_();

    // Start sending queued updates.
    this.initialRetryDone_ = this.doRetry_();

    window.addEventListener('online', this.onOnline_);
  }

  // Releases resources. Should be called if destroying the object.
  destroy() {
    if (this.retryTimeoutId_) window.clearTimeout(this.retryTimeoutId_);
    this.retryTimeoutId_ = null;

    window.removeEventListener('online', this.onOnline_);
  }

  // Returns a promise that is resolved once the initial retry attempt in the
  // constructor is completed.
  get initialRetryDoneForTest() {
    return this.initialRetryDone_;
  }

  // Asynchronously notifies the server that song |songId| was played starting
  // at |startTime| seconds since the Unix expoch. Returns a promise that is
  // resolved once the reporting attempt is completed (possibly unsuccessfully).
  reportPlay(songId: string, startTime: number): Promise<void> {
    // Move from queued (if present) to in-progress.
    removePlayReport(this.queuedPlayReports_, songId, startTime);
    addPlayReport(this.inProgressPlayReports_, songId, startTime);
    this.writeState_();

    const url =
      `played?songId=${encodeURIComponent(songId)}` +
      `&startTime=${encodeURIComponent(startTime)}`;
    console.log(`Reporting play: ${url}`);

    return fetch(url, { method: 'POST' })
      .then((res) => handleFetchError(res))
      .then(() => {
        removePlayReport(this.inProgressPlayReports_, songId, startTime);
        this.writeState_();
        this.scheduleRetry_(true /* immediate */);
      })
      .catch((err) => {
        console.error(`Reporting to ${url} failed: ${err}`);
        removePlayReport(this.inProgressPlayReports_, songId, startTime);
        addPlayReport(this.queuedPlayReports_, songId, startTime);
        this.writeState_();
        this.scheduleRetry_(false /* immediate */);
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
    const queued = this.queuedRatingsAndTags_[songId];
    if (queued) {
      if (rating === null && queued.rating !== null) rating = queued.rating;
      if (tags === null && queued.tags !== null) tags = queued.tags;
      delete this.queuedRatingsAndTags_[songId];
    }

    if (this.inProgressRatingsAndTags_.hasOwnProperty(songId)) {
      addRatingAndTags(this.queuedRatingsAndTags_, songId, rating, tags);
      return Promise.resolve();
    }

    addRatingAndTags(this.inProgressRatingsAndTags_, songId, rating, tags);
    this.writeState_();

    let url = `rate_and_tag?songId=${encodeURIComponent(songId)}`;
    if (rating !== null) url += `&rating=${rating}`;
    if (tags !== null) url += `&tags=${encodeURIComponent(tags.join(' '))}`;
    console.log(`Rating/tagging song: ${url}`);

    return fetch(url, { method: 'POST' })
      .then((res) => handleFetchError(res))
      .then(() => {
        delete this.inProgressRatingsAndTags_[songId];
        this.writeState_();
        this.scheduleRetry_(true /* immediate */);
      })
      .catch((err) => {
        console.log(`Rating/tagging to ${url} failed: ${err}`);
        delete this.inProgressRatingsAndTags_[songId];

        // If another update was queued in the meantime, don't overwrite it.
        const queued = this.queuedRatingsAndTags_[songId];
        if (queued) {
          if (queued.rating === null && rating !== null) queued.rating = rating;
          if (queued.tags === null && tags !== null) queued.tags = tags;
        } else {
          addRatingAndTags(this.queuedRatingsAndTags_, songId, rating, tags);
        }

        this.writeState_();
        this.scheduleRetry_(false /* immediate */);
      });
  }

  onOnline_ = () => {
    // Automatically try to send queued updates when we come back online.
    this.scheduleRetry_(true);
  };

  // Persists the current state to local storage.
  writeState_() {
    localStorage.setItem(
      Updater.QUEUED_PLAY_REPORTS_KEY_,
      JSON.stringify(this.queuedPlayReports_)
    );
    localStorage.setItem(
      Updater.QUEUED_RATINGS_AND_TAGS_KEY_,
      JSON.stringify(this.queuedRatingsAndTags_)
    );
    localStorage.setItem(
      Updater.IN_PROGRESS_PLAY_REPORTS_KEY_,
      JSON.stringify(this.inProgressPlayReports_)
    );
    localStorage.setItem(
      Updater.IN_PROGRESS_RATINGS_AND_TAGS_KEY_,
      JSON.stringify(this.inProgressRatingsAndTags_)
    );
  }

  // Schedules a doRetry_() call if needed.
  scheduleRetry_(immediate: boolean) {
    // If we're not online, don't bother trying.
    // We'll be called again when the system comes back online.
    if (navigator.onLine === false) return;

    // Already scheduled.
    if (this.retryTimeoutId_) {
      if (!immediate) return;
      window.clearTimeout(this.retryTimeoutId_);
      this.retryTimeoutId_ = null;
    }

    // Nothing to do.
    if (
      !this.queuedPlayReports_.length &&
      !Object.keys(this.queuedRatingsAndTags_).length
    ) {
      return;
    }

    let delayMs = immediate
      ? 0
      : this.lastRetryDelayMs_ > 0
      ? this.lastRetryDelayMs_ * 2
      : Updater.MIN_RETRY_DELAY_MS_;
    delayMs = Math.min(delayMs, Updater.MAX_RETRY_DELAY_MS_);

    console.log('Scheduling retry in ' + delayMs + ' ms');
    this.retryTimeoutId_ = window.setTimeout(() => {
      this.retryTimeoutId_ = null;
      return this.doRetry_();
    }, delayMs);
    this.lastRetryDelayMs_ = delayMs;
  }

  // Sends queued plays and ratings/tags to the server.
  doRetry_() {
    // Already have an in-progress update; try again in a bit.
    if (
      this.inProgressPlayReports_.length ||
      Object.keys(this.inProgressRatingsAndTags_).length
    ) {
      this.lastRetryDelayMs_ = 0; // use min retry delay
      this.scheduleRetry_(false);
      return Promise.resolve();
    }

    if (Object.keys(this.queuedRatingsAndTags_).length) {
      const songId = Object.keys(this.queuedRatingsAndTags_)[0];
      const entry = this.queuedRatingsAndTags_[songId];
      return this.rateAndTag(songId, entry.rating, entry.tags);
    }

    if (this.queuedPlayReports_.length) {
      const entry = this.queuedPlayReports_[0];
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
