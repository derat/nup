// Copyright 2015 Daniel Erat.
// All rights reserved.

class Updater {
  QUEUED_PLAY_REPORTS_KEY = 'queued_play_reports';
  IN_PROGRESS_PLAY_REPORTS_KEY = 'in_progress_play_reports';
  QUEUED_RATINGS_AND_TAGS_KEY = 'queued_ratings_and_tags';
  IN_PROGRESS_RATINGS_AND_TAGS_KEY = 'in_progress_ratings_and_tags';

  MIN_RETRY_DELAY_MS = 500; // Half a second.
  MAX_RETRY_DELAY_MS = 300 * 1000; // Five minutes.

  constructor() {
    this.retryTimeoutId = -1;

    this.lastRetryDelayMs = 0;

    // [{'songId': songId, 'startTime': startTime}, {'songId': songId, 'startTime': startTime}, ...]
    // Put reports that were in-progress during the last run into the queue.
    this.queuedPlayReports = this.readObject(
      this.QUEUED_PLAY_REPORTS_KEY,
      [],
    ).concat(this.readObject(this.IN_PROGRESS_PLAY_REPORTS_KEY, []));
    this.inProgressPlayReports = [];

    // {songId: {'rating': rating, 'tags': tags}, songId: {'rating': rating, 'tags': tags}, ...}
    // Either ratings or tags can be null.
    this.queuedRatingsAndTags = this.readObject(
      this.QUEUED_RATINGS_AND_TAGS_KEY,
      {},
    );
    const oldInProgress = this.readObject(
      this.IN_PROGRESS_RATINGS_AND_TAGS_KEY,
      {},
    );
    for (const songId in Object.keys(oldInProgress)) {
      this.queuedRatingsAndTags[songId] = oldInProgress[songId];
    }
    this.inProgressRatingsAndTags = {};

    this.writeState();

    // Start sending queued updates.
    this.doRetry();
  }

  reportPlay(songId, startTime) {
    // Move from queued (if present) to in-progress.
    this.removePlayReportFromArray(songId, startTime, this.queuedPlayReports);
    this.addPlayReportToArray(songId, startTime, this.inProgressPlayReports);
    this.writeState();

    const url =
      'report_played?songId=' +
      encodeURIComponent(songId) +
      '&startTime=' +
      encodeURIComponent(startTime);
    console.log('Reporting track: ' + url);
    const req = new XMLHttpRequest();

    const handleError = () => {
      console.log('Reporting to ' + url + ' failed; queuing to retry later');
      this.removePlayReportFromArray(
        songId,
        startTime,
        this.inProgressPlayReports,
      );
      this.addPlayReportToArray(songId, startTime, this.queuedPlayReports);
      this.writeState();
      this.scheduleRetry(false);
    };

    req.onload = () => {
      if (req.status == 200) {
        this.removePlayReportFromArray(
          songId,
          startTime,
          this.inProgressPlayReports,
        );
        this.writeState();
        this.scheduleRetry(true);
      } else {
        console.log('Got ' + req.status + ': ' + req.responseText);
        handleError();
      }
    };
    req.onerror = () => handleError();

    req.open('POST', url, true);
    req.send();
  }

  rateAndTag(songId, rating, tags) {
    if (rating == null && tags == null) return;

    delete this.queuedRatingsAndTags[songId];

    if (this.inProgressRatingsAndTags[songId] != null) {
      this.addRatingAndTagsToObject(
        songId,
        rating,
        tags,
        this.queuedRatingsAndTags,
      );
      return;
    }

    this.addRatingAndTagsToObject(
      songId,
      rating,
      tags,
      this.inProgressRatingsAndTags,
    );
    this.writeState();

    let url = 'rate_and_tag?songId=' + encodeURIComponent(songId);
    if (rating != null) url += '&rating=' + encodeURIComponent(rating);
    if (tags != null) url += '&tags=' + encodeURIComponent(tags.join(' '));
    console.log('Rating/tagging track: ' + url);
    const req = new XMLHttpRequest();

    const handleError = () => {
      console.log(
        'Rating/tagging to ' + url + ' failed; queuing to retry later',
      );
      delete this.inProgressRatingsAndTags[songId];
      this.addRatingAndTagsToObject(
        songId,
        rating,
        tags,
        this.queuedRatingsAndTags,
      );
      this.writeState();
      this.scheduleRetry(false);
    };

    req.onload = () => {
      if (req.status == 200) {
        delete this.inProgressRatingsAndTags[songId];
        this.writeState();
        this.scheduleRetry(true);
      } else {
        console.log('Got ' + req.status + ': ' + req.responseText);
        handleError();
      }
    };
    req.onerror = () => handleError();

    req.open('POST', url, true);
    req.send();
  }

  readObject(key, defaultObject) {
    const value = localStorage[key];
    return value != null ? JSON.parse(value) : defaultObject;
  }

  writeState() {
    localStorage[this.QUEUED_PLAY_REPORTS_KEY] = JSON.stringify(
      this.queuedPlayReports,
    );
    localStorage[this.QUEUED_RATINGS_AND_TAGS_KEY] = JSON.stringify(
      this.queuedRatingsAndTags,
    );
    localStorage[this.IN_PROGRESS_PLAY_REPORTS_KEY] = JSON.stringify(
      this.inProgressPlayReports,
    );
    localStorage[this.IN_PROGRESS_RATINGS_AND_TAGS_KEY] = JSON.stringify(
      this.inProgressRatingsAndTags,
    );
  }

  scheduleRetry(lastWasSuccessful) {
    // Already scheduled.
    if (this.retryTimeoutId >= 0) return;

    // Nothing to do.
    if (
      !this.queuedPlayReports.length &&
      !Object.keys(this.queuedRatingsAndTags).length
    ) {
      return;
    }

    let delayMs = lastWasSuccessful
      ? 0
      : this.lastRetryDelayMs > 0
      ? this.lastRetryDelayMs * 2
      : this.MIN_RETRY_DELAY_MS;
    delayMs = Math.min(delayMs, this.MAX_RETRY_DELAY_MS);
    console.log('Scheduling retry in ' + delayMs + ' ms');
    this.retryTimeoutId = window.setTimeout(() => this.doRetry(), delayMs);
    this.lastRetryDelayMs = delayMs;
  }

  doRetry() {
    this.retryTimeoutId = -1;

    // Already have an in-progress update; try again in a bit.
    if (
      this.inProgressPlayReports.length ||
      Object.keys(this.inProgressRatingsAndTags).length
    ) {
      this.retryTimeoutId = window.setTimeout(
        () => this.doRetry(),
        this.MIN_RETRY_DELAY_MS,
      );
      return;
    }

    if (Object.keys(this.queuedRatingsAndTags).length) {
      const songId = Object.keys(this.queuedRatingsAndTags)[0];
      const entry = this.queuedRatingsAndTags[songId];
      this.rateAndTag(songId, entry.rating, entry.tags);
      return;
    }

    if (this.queuedPlayReports.length) {
      const entry = this.queuedPlayReports[0];
      this.reportPlay(entry.songId, entry.startTime);
      return;
    }
  }

  addPlayReportToArray(songId, startTime, list) {
    list.push({songId: songId, startTime: startTime});
  }

  removePlayReportFromArray(songId, startTime, list) {
    for (let i = 0; i < list.length; i++) {
      if (list[i].songId == songId && list[i].startTime == startTime) {
        list.splice(i, 1);
        return;
      }
    }
  }

  addRatingAndTagsToObject(songId, rating, tags, obj) {
    obj[songId] = {rating: rating, tags: tags};
  }
}
