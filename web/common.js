// Copyright 2020 Daniel Erat.
// All rights reserved.

export function $(id) {
  return document.getElementById(id);
}

function pad(num, width) {
  let str = num + '';
  while (str.length < width) str = '0' + str;
  return str;
}

export function formatTime(sec) {
  return parseInt(sec / 60) + ':' + pad(parseInt(sec % 60), 2);
}

export function getCurrentTimeSec() {
  return new Date().getTime() / 1000.0;
}

export function updateTitleAttributeForTruncation(element, text) {
  element.title = element.scrollWidth > element.offsetWidth ? text : '';
}

export function createElement(type, className, parentElement, text) {
  const element = document.createElement(type);
  if (className) element.className = className;
  parentElement.appendChild(element);
  if (text != null) element.appendChild(document.createTextNode(text));
  return element;
}

export const KeyCodes = {
  ENTER: 13,
  ESCAPE: 27,
  LEFT: 37,
  RIGHT: 39,
  SPACE: 32,
  TAB: 9,
  SLASH: 191,

  D: 68,
  N: 78,
  O: 79,
  P: 80,
  R: 82,
  T: 84,
  V: 86,

  ZERO: 48,
  FIVE: 53,
};
