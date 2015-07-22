// Copyright 2015 Daniel Erat.
// All rights reserved.

function Updater() {
  this.retryTimeoutId = -1;

  this.lastRetryDelayMs = 0;

  // [{'songId': songId, 'startTime': startTime}, {'songId': songId, 'startTime': startTime}, ...]
  // Put reports that were in-progress during the last run into the queue.
  this.queuedPlayReports = this.readObject(Updater.QUEUED_PLAY_REPORTS_KEY, []).concat(
      this.readObject(Updater.IN_PROGRESS_PLAY_REPORTS_KEY, []));
  this.inProgressPlayReports = [];

  // {songId: {'rating': rating, 'tags': tags}, songId: {'rating': rating, 'tags': tags}, ...}
  // Either ratings or tags can be null.
  this.queuedRatingsAndTags = this.readObject(Updater.QUEUED_RATINGS_AND_TAGS_KEY, {});
  var oldInProgress = this.readObject(Updater.IN_PROGRESS_RATINGS_AND_TAGS_KEY, {});
  for (var songId in Object.keys(oldInProgress))
    this.queuedRatingsAndTags[songId] = oldInProgress[songId];
  this.inProgressRatingsAndTags = {};

  this.writeState();

  // Start sending queued updates.
  this.doRetry();
}

Updater.QUEUED_PLAY_REPORTS_KEY = 'queued_play_reports';
Updater.IN_PROGRESS_PLAY_REPORTS_KEY = 'in_progress_play_reports';
Updater.QUEUED_RATINGS_AND_TAGS_KEY = 'queued_ratings_and_tags';
Updater.IN_PROGRESS_RATINGS_AND_TAGS_KEY = 'in_progress_ratings_and_tags';

Updater.MIN_RETRY_DELAY_MS = 500;         // Half a second.
Updater.MAX_RETRY_DELAY_MS = 300 * 1000;  // Five minutes.

Updater.prototype.reportPlay = function(songId, startTime) {
  // Move from queued (if present) to in-progress.
  this.removePlayReportFromArray(songId, startTime, this.queuedPlayReports);
  this.addPlayReportToArray(songId, startTime, this.inProgressPlayReports);
  this.writeState();

  var url = 'report_played?songId=' + encodeURIComponent(songId) + '&startTime=' + encodeURIComponent(startTime);
  console.log('Reporting track: ' + url);
  var req = new XMLHttpRequest();

  var handleError = function() {
    console.log('Reporting to ' + url + ' failed; queuing to retry later');
    this.removePlayReportFromArray(songId, startTime, this.inProgressPlayReports);
    this.addPlayReportToArray(songId, startTime, this.queuedPlayReports);
    this.writeState();
    this.scheduleRetry(false);
  };

  req.onload = function() {
    if (req.status == 200) {
      this.removePlayReportFromArray(songId, startTime, this.inProgressPlayReports);
      this.writeState();
      this.scheduleRetry(true);
    } else {
      console.log('Got ' + req.status + ': ' + req.responseText);
      handleError.bind(this)();
    }
  }.bind(this);
  req.onerror = handleError.bind(this);

  req.open('POST', url, true);
  req.send();
};

Updater.prototype.rateAndTag = function(songId, rating, tags) {
  if (rating == null && tags == null)
    return;

  delete this.queuedRatingsAndTags[songId];

  if (this.inProgressRatingsAndTags[songId] != null) {
    this.addRatingAndTagsToObject(songId, rating, tags, this.queuedRatingsAndTags);
    return;
  }

  this.addRatingAndTagsToObject(songId, rating, tags, this.inProgressRatingsAndTags);
  this.writeState();

  var url = 'rate_and_tag?songId=' + encodeURIComponent(songId);
  if (rating != null)
    url += '&rating=' + encodeURIComponent(rating);
  if (tags != null)
    url += '&tags=' + encodeURIComponent(tags.join(' '));
  console.log('Rating/tagging track: ' + url);
  var req = new XMLHttpRequest();

  var handleError = function() {
    console.log('Rating/tagging to ' + url + ' failed; queuing to retry later');
    delete this.inProgressRatingsAndTags[songId];
    this.addRatingAndTagsToObject(songId, rating, tags, this.queuedRatingsAndTags);
    this.writeState();
    this.scheduleRetry(false);
  };

  req.onload = function() {
    if (req.status == 200) {
      delete this.inProgressRatingsAndTags[songId];
      this.writeState();
      this.scheduleRetry(true);
    } else {
      console.log('Got ' + req.status + ': ' + req.responseText);
      handleError.bind(this)();
    }
  }.bind(this);
  req.onerror = handleError.bind(this);

  req.open('POST', url, true);
  req.send();
};

Updater.prototype.readObject = function(key, defaultObject) {
  var value = localStorage[key];
  return value != null ? JSON.parse(value) : defaultObject;
};

Updater.prototype.writeState = function() {
  localStorage[Updater.QUEUED_PLAY_REPORTS_KEY] = JSON.stringify(this.queuedPlayReports);
  localStorage[Updater.QUEUED_RATINGS_AND_TAGS_KEY] = JSON.stringify(this.queuedRatingsAndTags);
  localStorage[Updater.IN_PROGRESS_PLAY_REPORTS_KEY] =  JSON.stringify(this.inProgressPlayReports);
  localStorage[Updater.IN_PROGRESS_RATINGS_AND_TAGS_KEY] =  JSON.stringify(this.inProgressRatingsAndTags);
};

Updater.prototype.scheduleRetry = function(lastWasSuccessful) {
  // Already scheduled.
  if (this.retryTimeoutId >= 0)
    return;

  // Nothing to do.
  if (!this.queuedPlayReports.length && !Object.keys(this.queuedRatingsAndTags).length)
    return;

  var delayMs = lastWasSuccessful ? 0 :
      (this.lastRetryDelayMs > 0 ? this.lastRetryDelayMs * 2 : Updater.MIN_RETRY_DELAY_MS);
  delayMs = Math.min(delayMs, Updater.MAX_RETRY_DELAY_MS);
  console.log('Scheduling retry in ' + delayMs + ' ms');
  this.retryTimeoutId = window.setTimeout(this.doRetry.bind(this), delayMs);
  this.lastRetryDelayMs = delayMs;
};

Updater.prototype.doRetry = function() {
  this.retryTimeoutId = -1;

  // Already have an in-progress update; try again in a bit.
  if (this.inProgressPlayReports.length || Object.keys(this.inProgressRatingsAndTags).length) {
    this.retryTimeoutId = window.setTimeout(this.doRetry.bind(this), Updater.MIN_RETRY_DELAY_MS);
    return;
  }

  if (Object.keys(this.queuedRatingsAndTags).length) {
    var songId = Object.keys(this.queuedRatingsAndTags)[0];
    var entry = this.queuedRatingsAndTags[songId];
    this.rateAndTag(songId, entry.rating, entry.tags);
    return;
  }

  if (this.queuedPlayReports.length) {
    var entry = this.queuedPlayReports[0];
    this.reportPlay(entry.songId, entry.startTime);
    return;
  }
};

Updater.prototype.addPlayReportToArray = function(songId, startTime, list) {
  list.push({'songId': songId, 'startTime': startTime});
};

Updater.prototype.removePlayReportFromArray = function(songId, startTime, list) {
  for (var i = 0; i < list.length; i++) {
    if (list[i].songId == songId && list[i].startTime == startTime) {
      list.splice(i, 1);
      return;
    }
  }
};

Updater.prototype.addRatingAndTagsToObject = function(songId, rating, tags, obj) {
  obj[songId] = {'rating': rating, 'tags': tags};
};
