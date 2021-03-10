// Copyright 2015 Daniel Erat.
// All rights reserved.

export default class Updater {
  static QUEUED_PLAY_REPORTS_KEY_ = 'queued_play_reports';
  static IN_PROGRESS_PLAY_REPORTS_KEY_ = 'in_progress_play_reports';
  static QUEUED_RATINGS_AND_TAGS_KEY_ = 'queued_ratings_and_tags';
  static IN_PROGRESS_RATINGS_AND_TAGS_KEY_ = 'in_progress_ratings_and_tags';

  static MIN_RETRY_DELAY_MS_ = 500; // Half a second.
  static MAX_RETRY_DELAY_MS_ = 300 * 1000; // Five minutes.

  constructor() {
    this.retryTimeoutId_ = -1; // for doRetry_()
    this.lastRetryDelayMs_ = 0; // used by scheduleRetry_()

    // [{'songId': songId, 'startTime': startTime}, {'songId': songId, 'startTime': startTime}, ...]
    // Put reports that were in-progress during the last run into the queue.
    this.queuedPlayReports_ = readObject(
      Updater.QUEUED_PLAY_REPORTS_KEY_,
      []
    ).concat(readObject(Updater.IN_PROGRESS_PLAY_REPORTS_KEY_, []));
    this.inProgressPlayReports_ = [];

    // {songId: {'rating': rating, 'tags': tags}, songId: {'rating': rating, 'tags': tags}, ...}
    // Either ratings or tags can be null.
    this.queuedRatingsAndTags_ = readObject(
      Updater.QUEUED_RATINGS_AND_TAGS_KEY_,
      {}
    );
    const oldInProgress = readObject(
      Updater.IN_PROGRESS_RATINGS_AND_TAGS_KEY_,
      {}
    );
    for (const songId in Object.keys(oldInProgress)) {
      this.queuedRatingsAndTags_[songId] = oldInProgress[songId];
    }
    this.inProgressRatingsAndTags_ = {};

    this.writeState_();

    // Start sending queued updates.
    this.doRetry_();
  }

  // Asynchronously notifies the server that song |songId| was played starting
  // at |startTime| seconds since the Unix expoch.
  reportPlay(songId, startTime) {
    // Move from queued (if present) to in-progress.
    removePlayReport(songId, startTime, this.queuedPlayReports_);
    addPlayReport(songId, startTime, this.inProgressPlayReports_);
    this.writeState_();

    const url =
      'report_played?songId=' +
      encodeURIComponent(songId) +
      '&startTime=' +
      encodeURIComponent(startTime);
    console.log('Reporting track: ' + url);
    const req = new XMLHttpRequest();

    const handleError = () => {
      console.log('Reporting to ' + url + ' failed; queuing to retry later');
      removePlayReport(songId, startTime, this.inProgressPlayReports_);
      addPlayReport(songId, startTime, this.queuedPlayReports_);
      this.writeState_();
      this.scheduleRetry_(false);
    };

    req.onload = () => {
      if (req.status == 200) {
        removePlayReport(songId, startTime, this.inProgressPlayReports_);
        this.writeState_();
        this.scheduleRetry_(true);
      } else {
        console.log('Got ' + req.status + ': ' + req.responseText);
        handleError();
      }
    };
    req.onerror = () => handleError();

    req.open('POST', url, true);
    req.send();
  }

  // Asynchronously notifies the server that song |songId| was given |rating| (a
  // float) and |tags| (a string array).
  rateAndTag(songId, rating, tags) {
    if (rating == null && tags == null) return;

    delete this.queuedRatingsAndTags_[songId];

    if (this.inProgressRatingsAndTags_[songId] != null) {
      addRatingAndTags(songId, rating, tags, this.queuedRatingsAndTags_);
      return;
    }

    addRatingAndTags(songId, rating, tags, this.inProgressRatingsAndTags_);
    this.writeState_();

    let url = 'rate_and_tag?songId=' + encodeURIComponent(songId);
    if (rating != null) url += '&rating=' + encodeURIComponent(rating);
    if (tags != null) url += '&tags=' + encodeURIComponent(tags.join(' '));
    console.log('Rating/tagging track: ' + url);
    const req = new XMLHttpRequest();

    const handleError = () => {
      console.log(
        'Rating/tagging to ' + url + ' failed; queuing to retry later'
      );
      delete this.inProgressRatingsAndTags_[songId];
      addRatingAndTags(songId, rating, tags, this.queuedRatingsAndTags_);
      this.writeState_();
      this.scheduleRetry_(false);
    };

    req.onload = () => {
      if (req.status == 200) {
        delete this.inProgressRatingsAndTags_[songId];
        this.writeState_();
        this.scheduleRetry_(true);
      } else {
        console.log('Got ' + req.status + ': ' + req.responseText);
        handleError();
      }
    };
    req.onerror = () => handleError();

    req.open('POST', url, true);
    req.send();
  }

  // Persists the current state to local storage.
  writeState_() {
    localStorage[Updater.QUEUED_PLAY_REPORTS_KEY_] = JSON.stringify(
      this.queuedPlayReports_
    );
    localStorage[Updater.QUEUED_RATINGS_AND_TAGS_KEY_] = JSON.stringify(
      this.queuedRatingsAndTags_
    );
    localStorage[Updater.IN_PROGRESS_PLAY_REPORTS_KEY_] = JSON.stringify(
      this.inProgressPlayReports_
    );
    localStorage[Updater.IN_PROGRESS_RATINGS_AND_TAGS_KEY_] = JSON.stringify(
      this.inProgressRatingsAndTags_
    );
  }

  // Schedules a doRetry_() call.
  scheduleRetry_(lastWasSuccessful) {
    // Already scheduled.
    if (this.retryTimeoutId_ >= 0) return;

    // Nothing to do.
    if (
      !this.queuedPlayReports_.length &&
      !Object.keys(this.queuedRatingsAndTags_).length
    ) {
      return;
    }

    let delayMs = lastWasSuccessful
      ? 0
      : this.lastRetryDelayMs_ > 0
      ? this.lastRetryDelayMs_ * 2
      : Updater.MIN_RETRY_DELAY_MS_;
    delayMs = Math.min(delayMs, Updater.MAX_RETRY_DELAY_MS_);
    console.log('Scheduling retry in ' + delayMs + ' ms');
    this.retryTimeoutId_ = window.setTimeout(() => this.doRetry_(), delayMs);
    this.lastRetryDelayMs_ = delayMs;
  }

  // Sends queued plays and ratings/tags to the server.
  doRetry_() {
    this.retryTimeoutId_ = -1;

    // Already have an in-progress update; try again in a bit.
    if (
      this.inProgressPlayReports_.length ||
      Object.keys(this.inProgressRatingsAndTags_).length
    ) {
      this.lastRetryDelayMs_ = 0; // use min retry delay
      this.scheduleRetry_(false);
      return;
    }

    if (Object.keys(this.queuedRatingsAndTags_).length) {
      const songId = Object.keys(this.queuedRatingsAndTags_)[0];
      const entry = this.queuedRatingsAndTags_[songId];
      this.rateAndTag(songId, entry.rating, entry.tags);
      return;
    }

    if (this.queuedPlayReports_.length) {
      const entry = this.queuedPlayReports_[0];
      this.reportPlay(entry.songId, entry.startTime);
      return;
    }
  }
}

// Reads |key| from local storage and parses it as JSON.
// |defaultObject| is returned if the key is unset.
function readObject(key, defaultObject) {
  const value = localStorage[key];
  return value != null ? JSON.parse(value) : defaultObject;
}

// Appends a play report to |list|.
function addPlayReport(songId, startTime, list) {
  list.push({ songId: songId, startTime: startTime });
}

// Removes the specified play report from |list|.
function removePlayReport(songId, startTime, list) {
  for (let i = 0; i < list.length; i++) {
    if (list[i].songId == songId && list[i].startTime == startTime) {
      list.splice(i, 1);
      return;
    }
  }
}

// Sets |songId|'s rating and tags within |obj|.
function addRatingAndTags(songId, rating, tags, obj) {
  obj[songId] = { rating: rating, tags: tags };
}
