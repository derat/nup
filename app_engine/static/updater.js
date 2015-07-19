// Copyright 2015 Daniel Erat.
// All rights reserved.

function Updater() {
}

Updater.prototype.reportPlay = function(songId, startTime) {
  var url = 'report_played?songId=' + encodeURIComponent(songId) + '&startTime=' + encodeURIComponent(startTime);
  console.log("Reporting track: " + url);
  var req = new XMLHttpRequest();
  req.open('POST', url, true);
  req.send();
};

Updater.prototype.rateAndTag = function(songId, rating, tags) {
  if (rating == null && tags == null)
    return;

  var url = 'rate_and_tag?songId=' + encodeURIComponent(songId);
  if (rating != null)
    url += '&rating=' + encodeURIComponent(rating);
  if (tags != null)
    url += '&tags=' + encodeURIComponent(tags.join(' '));
  console.log("Rating/tagging track: " + url);
  var req = new XMLHttpRequest();
  req.open('POST', url, true);
  req.send();
};
