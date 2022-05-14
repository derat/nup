// Copyright 2020 Daniel Erat.
// All rights reserved.

// Empty GIF: https://stackoverflow.com/a/14115340
export const emptyImg =
  'data:image/gif;base64,R0lGODlhAQABAAD/ACwAAAAAAQABAAACADs=';

// Width/height in pixels of the image returned by getScaledCoverUrl(). To cut
// down on extra work in the server and make it easier to preload images, this
// should be the max of all of the sizes needed by the app:
//
// - Notifications use 192x192 per
//   https://developers.google.com/web/fundamentals/push-notifications/display-a-notification:
//   "Sadly there aren't any solid guidelines for what size image to use for an
//   icon. Android seems to want a 64dp image (which is 64px multiples by the
//   device pixel ratio). If we assume the highest pixel ratio for a device will
//   be 3, an icon size of 192px or more is a safe bet."
// - mediaSession on Chrome for Android uses 512x512 per
//   https://developers.google.com/web/updates/2017/02/media-session. Chrome OS
//   media notifications display album art at a substantially smaller size,
//   maybe around 230x230, and unfortunately use nearest-neighbor downsampling.
//   The code still specifies that 512x512 is the desired size, though -- see
//   kMediaSessionNotificationArtworkDesiredSize in
//   components/media_message_center/media_notification_constants.h.
// - <music-player> uses 70x70 CSS pixels for the current song's cover.
// - <presentation-layer> uses 80x80 CSS pixels for the next song's cover.
// - Favicons allegedly take a wide variety of sizes:
//   https://stackoverflow.com/a/26807004
export const scaledCoverSize = 512;

// Returns the element under |root| with ID |id|.
export function $(id, root) {
  return (root || document).getElementById(id);
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

// Returns a URL for a scaled, square version of the cover image identified by
// |filename| (corresponding to a song's |coverFilename| property). If
// |filename| is empty, an empty string is returned.
export function getScaledCoverUrl(filename) {
  if (!filename) return '';
  return getAbsUrl(
    `/cover?filename=${encodeURIComponent(filename)}` +
      `&size=${scaledCoverSize}`
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
