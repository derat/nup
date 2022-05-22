// Copyright 2020 Daniel Erat.
// All rights reserved.

// Empty GIF: https://stackoverflow.com/a/14115340
export const emptyImg =
  'data:image/gif;base64,R0lGODlhAQABAAD/ACwAAAAAAQABAAACADs=';

// Returns the element under |root| with ID |id|.
export function $(id, root) {
  return (root ?? document).getElementById(id);
}

// Clamps number |val| between |min| and |max|.
export function clamp(val, min, max) {
  return Math.min(Math.max(val, min), max);
}

function pad(num, width) {
  let str = num + '';
  while (str.length < width) str = '0' + str;
  return str;
}

// Formats |sec| as "m:ss".
export function formatTime(sec) {
  return parseInt(sec / 60) + ':' + pad(parseInt(sec % 60), 2);
}

// Returns the number of milliseconds since the Unix epoch.
export function getCurrentTimeSec() {
  return new Date().getTime() / 1000.0;
}

// Sets |element|'s 'title' attribute to |text| if the row's content overflows
// its area or removes it otherwise.
//
// Note that this can be slow, as accessing |scrollWidth| and |offsetWidth| may
// trigger a reflow: https://stackoverflow.com/a/70871905/6882947
export function updateTitleAttributeForTruncation(element, text) {
  if (element.scrollWidth > element.offsetWidth) element.title = text;
  else element.removeAttribute('title');
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
  const shadow = el.attachShadow({ mode: 'open' });
  if (template) shadow.appendChild(template.content.cloneNode(true));
  return shadow;
}

// Creates and returns a new <template> containing the supplied HTML.
export function createTemplate(html) {
  const template = document.createElement('template');
  template.innerHTML = html;
  return template;
}

// Returns an absolute URL for the song specified by |filename| (corresponding
// to a song's |filename| property).
export function getSongUrl(filename) {
  return getAbsUrl(`/song?filename=${encodeURIComponent(filename)}`);
}

// Image sizes that can be passed to getScaledCoverUrl().
export const smallCoverSize = 256;
export const largeCoverSize = 512;

// Returns a URL for a scaled, square version of the cover image identified by
// |filename| (corresponding to a song's |coverFilename| property). If
// |filename| is empty, an empty string is returned. |size| should be either
// |smallCoverSize| or |largeCoverSize|.
export function getScaledCoverUrl(filename, size) {
  if (!filename) return '';
  return getAbsUrl(
    `/cover?filename=${encodeURIComponent(filename)}&size=${size}&webp=1`
  );
}

// Returns a URL for the full-size, possibly non-square cover image identified
// by |filename|.
export function getFullCoverUrl(filename) {
  if (!filename) return '';
  return getAbsUrl(`/cover?filename=${encodeURIComponent(filename)}`);
}

// Returns a URL for dumping information about the song identified by |songId|.
export function getDumpSongUrl(songId) {
  return `/dump_song?songId=${songId}`;
}

// Returns an absolute version of |url| if it's relative.
// If it's already absolute, it is returned unchanged.
export function getAbsUrl(url) {
  return new URL(url, document.baseURI).href;
}

// Throws if |response| failed due to the server returning an error status.
export function handleFetchError(response) {
  if (!response.ok) {
    return response.text().then((text) => {
      throw new Error(`${response.status}: ${text}`, response);
    });
  }
  return response;
}

// Converts a rating in the range [1, 5] (or 0 for unrated) to a string.
export function getRatingString(
  rating,
  filledStar = '★',
  emptyStar = '☆',
  unrated = 'Unrated',
  ratedPrefix = ''
) {
  rating = clamp(parseInt(rating), 0, 5);
  if (rating === 0 || isNaN(rating)) return unrated;

  let str = ratedPrefix;
  for (let i = 1; i <= 5; ++i) str += i <= rating ? filledStar : emptyStar;
  return str;
}

// Moves the item at index |from| in |array| to index |to|.
// If |idx| is passed, it is adjusted if needed and returned.
export function moveItem(array, from, to, idx) {
  if (from === to) return idx;

  // https://stackoverflow.com/a/2440723
  array.splice(to, 0, array.splice(from, 1)[0]);

  if (typeof idx !== 'undefined' && idx >= 0) {
    if (from === idx) idx = to;
    else if (from < idx && to >= idx) idx--;
    else if (from > idx && to <= idx) idx++;
  }
  return idx;
}
