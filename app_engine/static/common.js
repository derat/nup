function $(id) { return document.getElementById(id); }

function pad(num, width) {
  var str = num + '';
  while (str.length < width)
    str = '0' + str;
  return str;
}

function formatTime(sec) {
  return parseInt(sec / 60) + ':' + pad(parseInt(sec % 60), 2);
}

function getCurrentTimeMs() {
  return new Date().getTime();
}

function getCurrentTimeSec() {
  return getCurrentTimeMs() / 1000;
}

function updateTitleAttributeForTruncation(element, text) {
  element.title = (element.scrollWidth > element.offsetWidth) ? text : '';
}

var KeyCodes = {
  ENTER:  13,
  ESCAPE: 27,
  LEFT:   37,
  RIGHT:  39,
  SPACE:  32,
  TAB:     9,

  N:      78,
  P:      80,
  R:      82,
  T:      84,

  ZERO:   48,
  FIVE:   53
};
