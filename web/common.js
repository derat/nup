// Copyright 2020 Daniel Erat.
// All rights reserved.

export function $(id, root) {
  return (root || document).getElementById(id);
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

// Creates and returns a new |type| element. All other parameters are optional.
export function createElement(type, className, parentElement, text) {
  const element = document.createElement(type);
  if (className) element.className = className;
  if (parentElement) parentElement.appendChild(element);
  if (text || text === '') element.appendChild(document.createTextNode(text));
  return element;
}

// Creates and returns a new shadow DOM attached to |el|. If |template| is
// supplied, a copy of it is attached as a child of the root node.
export function createShadow(el, template) {
  const shadow = el.attachShadow({mode: 'open'});
  if (template) shadow.appendChild(template.content.cloneNode(true));
  return shadow;
}

// Creates and returns a new <template> containing the supplied HTML.
export function createTemplate(html) {
  const template = document.createElement('template');
  template.innerHTML = html;
  return template;
}

// Creates and returns a <style> element containing the supplied CSS.
export function createStyle(text) {
  const style = document.createElement('style');
  style.type = 'text/css';
  style.innerText = text;
  return style;
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
