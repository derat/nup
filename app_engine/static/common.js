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

function getCurrentTimeSec() {
  return new Date().getTime() / 1000.0;
}

function updateTitleAttributeForTruncation(element, text) {
  element.title = (element.scrollWidth > element.offsetWidth) ? text : '';
}

function createClassNameRegExp(className) {
  return new RegExp('(^|\\s+)' + className + '($|\\s+)');
}

function addClassName(element, className) {
  var re = createClassNameRegExp(className);
  if (!element.className.match(re)) {
    element.className += ' ' + className;
  }
}

function removeClassName(element, className) {
  var re = createClassNameRegExp(className);
  element.className = element.className.replace(re, ' ');
}

var KeyCodes = {
  ENTER:   13,
  ESCAPE:  27,
  LEFT:    37,
  RIGHT:   39,
  SPACE:   32,
  TAB:      9,
  SLASH:  191,

  D:       68,
  N:       78,
  P:       80,
  R:       82,
  T:       84,

  ZERO:    48,
  FIVE:    53
};
