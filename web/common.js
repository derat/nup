// Copyright 2020 Daniel Erat.
// All rights reserved.

// Empty GIF: https://stackoverflow.com/a/14115340
export const emptyImg =
  'data:image/gif;base64,R0lGODlhAQABAAD/ACwAAAAAAQABAAACADs=';

// Width/height in pixels of the image returned by getScaledCoverUrl(). To cut
// down on extra work in the server and make it easier to preload images, this
// should be the max of all of the sizes needed by the app:
//
// Notifications use 192x192 per
// https://developers.google.com/web/fundamentals/push-notifications/display-a-notification:
// "Sadly there aren't any solid guidelines for what size image to use for an
// icon. Android seems to want a 64dp image (which is 64px multiples by the
// device pixel ratio). If we assume the highest pixel ratio for a device will
// be 3, an icon size of 192px or more is a safe bet."
//
// mediaSession on Chrome for Android uses 512x512 per
// https://developers.google.com/web/updates/2017/02/media-session. The image
// looks like it's substantially smaller on Chrome OS.
//
// <music-player> uses 70x70 CSS pixels, and <presentation-layer> uses 80x80.
//
// Favicons are usually 48x48.
export const scaledCoverSize = 512;

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

// Returns a URL for a scaled, square version of the cover image identified by
// |filename| (corresponding to a song's |coverFilename| property). If
// |filename| is empty, an empty string is returned.
export function getScaledCoverUrl(filename) {
  if (!filename) return '';
  return `/cover?filename=${encodeURIComponent(
    filename,
  )}&size=${scaledCoverSize}`;
}
