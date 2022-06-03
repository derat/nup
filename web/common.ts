// Copyright 2020 Daniel Erat.
// All rights reserved.

// Empty GIF: https://stackoverflow.com/a/14115340
export const emptyImg =
  'data:image/gif;base64,R0lGODlhAQABAAD/ACwAAAAAAQABAAACADs=';

// Returns the element under |root| with ID |id|.
export function $(
  id: string,
  root: Document | ShadowRoot = document
): HTMLElement {
  const el = root.getElementById(id);
  if (!el) throw new Error(`Didn't find element #${id}`);
  return el;
}

// Clamps |val| between |min| and |max|.
export const clamp = (val: number, min: number, max: number) =>
  Math.min(Math.max(val, min), max);

function pad(num: number, width: number) {
  let str = num.toString();
  while (str.length < width) str = '0' + str;
  return str;
}

// Formats |sec| as 'm:ss'.
export const formatDuration = (sec: number) =>
  `${Math.floor(sec / 60)}:${pad(Math.floor(sec % 60), 2)}`;

// Formats |sec| as a rounded relative time, e.g. '1 second ago' for -1
// or 'in 2 hours' for 7200.
export function formatRelativeTime(sec: number) {
  const rtf = new Intl.RelativeTimeFormat('en', { style: 'long' });
  const fmt = (n: number, u: Intl.RelativeTimeFormatUnit) =>
    rtf.format(Math.round(n), u);
  const days = sec / 86400;
  const hours = sec / 3600;
  const min = sec / 60;

  if (Math.abs(Math.round(hours)) >= 24) return fmt(days, 'day');
  if (Math.abs(Math.round(min)) >= 60) return fmt(hours, 'hour');
  if (Math.abs(Math.round(sec)) >= 60) return fmt(min, 'minute');
  return fmt(sec, 'second');
}

// Returns the number of fractional milliseconds since the Unix epoch.
export const getCurrentTimeSec = () => Date.now() / 1000;

// Sets |element|'s 'title' attribute to |text| if the row's content overflows
// its area or removes it otherwise.
//
// Note that this can be slow, as accessing |scrollWidth| and |offsetWidth| may
// trigger a reflow: https://stackoverflow.com/a/70871905/6882947
export function updateTitleAttributeForTruncation(
  element: HTMLElement,
  text: string
) {
  if (element.scrollWidth > element.offsetWidth) element.title = text;
  else element.removeAttribute('title');
}

// Creates and returns a new |type| element. All other parameters are optional.
export function createElement(
  type: string,
  className: string | null = null,
  parentElement: HTMLElement | ShadowRoot | null = null,
  text: string | null = null
) {
  const element = document.createElement(type);
  if (className) element.className = className;
  if (parentElement) parentElement.appendChild(element);
  if (text || text === '') element.appendChild(document.createTextNode(text));
  return element;
}

// Creates and returns a new shadow DOM attached to |el|. If |template| is
// supplied, a copy of it is attached as a child of the root node.
export function createShadow(el: HTMLElement, template?: HTMLTemplateElement) {
  const shadow = el.attachShadow({ mode: 'open' });
  if (template) shadow.appendChild(template.content.cloneNode(true));
  return shadow;
}

// Creates and returns a new <template> containing the supplied HTML.
export function createTemplate(html: string) {
  const template = document.createElement('template');
  template.innerHTML = html;
  return template;
}

// Returns an absolute URL for the song specified by |filename| (corresponding
// to a song's |filename| property).
export const getSongUrl = (filename: string) =>
  getAbsUrl(`/song?filename=${encodeURIComponent(filename)}`);

// Image sizes that can be passed to getCoverUrl().
export const smallCoverSize = 256;
export const largeCoverSize = 512;

// Returns a URL for the cover image identified by |filename|.
// If |filename| is null or empty, an empty string is returned.
// If |size| isn't supplied, returns the full-size, possibly-non-square image.
// Otherwise (i.e. |smallCoverSize| or |largeCoverSize|), returns a scaled,
// square version.
export function getCoverUrl(filename: string | null, size = 0) {
  if (!filename) return '';
  let path = `/cover?filename=${encodeURIComponent(filename)}`;
  if (size) path += `&size=${size}&webp=1`;
  return getAbsUrl(path);
}

// Fetches an image at |src| so it can be loaded from the cache later.
export const preloadImage = (src: string) => (new Image().src = src);

// Returns a URL for dumping information about the song identified by |songId|.
export const getDumpSongUrl = (songId: string) => `/dump_song?songId=${songId}`;

// Returns an absolute version of |url| if it's relative.
// If it's already absolute, it is returned unchanged.
const getAbsUrl = (url: string) => new URL(url, document.baseURI).href;

// Throws if |response| failed due to the server returning an error status.
export function handleFetchError(response: Response) {
  if (!response.ok) {
    return response.text().then((text) => {
      throw new Error(`${response.status}: ${text}`);
    });
  }
  return response;
}

// Converts a rating in the range [1, 5] (or 0 for unrated) to a string.
export function getRatingString(
  rating: number,
  filledStar = '★',
  emptyStar = '☆',
  unrated = 'Unrated',
  ratedPrefix = ''
) {
  rating = clamp(Math.round(rating), 0, 5);
  if (rating === 0 || isNaN(rating)) return unrated;

  let str = ratedPrefix;
  for (let i = 1; i <= 5; ++i) str += i <= rating ? filledStar : emptyStar;
  return str;
}

// Moves the item at index |from| in |array| to index |to|.
// If |idx| is passed, it is adjusted if needed and returned.
export function moveItem<T>(
  array: Array<T>,
  from: number,
  to: number,
  idx?: number
) {
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

// Common CSS used in the document and shadow roots.
export const commonStyles = new CSSStyleSheet();
commonStyles.replaceSync(`
/* With Chrome using cache partitioning since version 85
 * (https://developer.chrome.com/blog/http-cache-partitioning/), there doesn't
 * seem to be much benefit to using Google Fonts, and doing so also requires an
 * extra round trip for CSS before font files can be fetched. So, self-host:
 * https://google-webfonts-helper.herokuapp.com/fonts/roboto?subsets=latin */
@font-face {
  font-family: 'Roboto';
  font-style: normal;
  font-weight: 400;
  /* prettier-ignore */
  src: local(''),
    url('fonts/roboto-v30-latin-regular.woff2') format('woff2'), /* Chrome 26+, Opera 23+, Firefox 39+ */
    url('fonts/roboto-v30-latin-regular.woff') format('woff'); /* Chrome 6+, Firefox 3.6+, IE 9+, Safari 5.1+ */
}

/* Star characters don't appear to be provided by either Verdana or Arial. They
 * *are* provided by DejaVu Sans, but that's not present on Chrome OS -- Noto
 * Sans SC is used instead. Unfortunately, the metrics are different in
 * different fonts, which leads to inconsistent sizing and padding. So, use a
 * handy custom font generated using fontello.com. */
@font-face {
  font-family: 'fontello';
  src: url('fonts/fontello-v1.woff2') format('woff2'),
    url('fonts/fontello-v1.woff') format('woff');
  font-display: block; /* avoid showing fallback fonts */
  font-style: normal;
  font-weight: normal;
}

:root {
  --font-family: 'Roboto', sans-serif;
  --font-size: 13.3333px;

  --control-border: 1px solid var(--control-color);
  --control-border-radius: 4px;
  --control-line-height: 16px;

  --margin: 10px; /* margin around groups of elements */
  --button-spacing: 6px; /* horizontal spacing between buttons */

  --icon-font-family: fontello, sans-serif;

  --bg-color: #fff;
  --bg-active-color: #eee; /* song row with context menu */
  --text-color: #000;
  --text-label-color: #666; /* song detail field names, menu hotkeys */
  --text-hover-color: #666;
  --accent-color: #42a5f5; /* song row highlight, material blue 400 */
  --accent-active-color: #1976d2; /* material blue 700 */
  --accent-text-color: #fff;
  --border-color: #ddd; /* between frames */
  --button-color: #aaa;
  --button-hover-color: #666;
  --button-disabled-color: #ddd;
  --control-color: #ddd;
  --control-active-color: #999; /* checked checkbox */
  --cover-missing-color: #f5f5f5;
  --dialog-title-color: var(--accent-color);
  --frame-border-color: var(--bg-color); /* dialogs, menus, rating/tags */
  --header-color: #f5f5f5; /* song table header */
  --icon-color: #aaa; /* clear button, select arrow */
  --icon-hover-color: #666;
  --menu-hover-color: #eee;
  --suggestions-color: #eee; /* tag suggestions background */
}

[data-theme='dark'] {
  --bg-color: #222;
  --bg-active-color: #333;
  --text-color: #ccc;
  --text-label-color: #999;
  --text-hover-color: #eee;
  --accent-color: #1f517a;
  --accent-active-color: #296ea6;
  --accent-text-color: #fff;
  --border-color: #555;
  --button-color: #888;
  --button-hover-color: #aaa;
  --button-disabled-color: #555;
  --control-color: #555;
  --control-active-color: #888;
  --cover-missing-color: #333;
  --frame-border-color: #444;
  --dialog-title-color: #42a5f5; /* material blue 400 */
  --header-color: #333;
  --icon-color: #aaa;
  --icon-hover-color: #ccc;
  --menu-hover-color: #444;
  --suggestions-color: #444;
}

span.x-icon {
  color: var(--icon-color);
  font-family: fontello, sans-serif;
  font-size: 10px;
  padding: 4px;
}
span.x-icon:hover {
  color: var(--icon-hover-color);
}
span.x-icon::before {
  content: '×';
}

button {
  background-color: var(--button-color);
  border: none;
  border-radius: var(--control-border-radius);
  color: var(--bg-color);
  cursor: pointer;
  font-family: var(--font-family);
  font-size: 12px;
  font-weight: bold;
  height: 28px;
  letter-spacing: 0.0892857143em;
  line-height: 28px;
  overflow: hidden; /* prevent icon from extending focus ring */
  padding: 1px 12px 0 12px;
  text-transform: uppercase;
  user-select: none;
}
button:hover {
  background-color: var(--button-hover-color);
}
button:disabled {
  background-color: var(--button-disabled-color);
  box-shadow: none;
  cursor: default;
}

input[type='text'],
textarea {
  appearance: none;
  -moz-appearance: none;
  -ms-appearance: none;
  -webkit-appearance: none;
  background-color: var(--bg-color);
  border: var(--control-border);
  border-radius: var(--control-border-radius);
  color: var(--text-color);
  font-family: var(--font-family);
  line-height: var(--control-line-height);
  padding: 6px 4px 4px 6px;
}

input[type='checkbox'] {
  appearance: none;
  -moz-appearance: none;
  -ms-appearance: none;
  -webkit-appearance: none;
  background-color: var(--bg-color);
  border: solid 1px var(--control-color);
  border-radius: 2px;
  height: 14px;
  position: relative;
  width: 14px;
}
input[type='checkbox']:checked {
  border-color: var(--control-active-color);
  color: var(--control-active-color);
}
input[type='checkbox']:checked:before {
  content: '✓';
  font-family: var(--icon-font-family);
  font-size: 9px;
  margin-left: 2px;
  margin-top: 2px;
  position: absolute;
}

/* Make a minor tweak to the checkmark position for low-DPI displays.
 * Chrome DevTools' rendering of this on a high-DPI display doesn't seem
 * accurate when using an emulated device with a device pixel ratio of 1. */
@media (-webkit-max-device-pixel-ratio: 1) {
  input[type='checkbox']:checked:before {
    margin-top: 1px;
  }
}

input[type='checkbox'].small {
  height: 12px;
  width: 12px;
}
input[type='checkbox'].small:checked:before {
  font-size: 7px;
  margin-left: 2px;
  margin-top: 1px;
}

input:disabled {
  opacity: 0.5;
}

/* To avoid spacing differences between minified and non-minified code, omit
 * whitespace between </select> and the closing .select-wrapper </span> tag.
 * I think that https://github.com/tdewolff/minify/issues/240 is related. */
.select-wrapper {
  display: inline-block;
  margin-left: 4px;
  margin-right: -2px;
}
.select-wrapper:after {
  color: var(--icon-color);
  font-family: fontello, sans-serif;
  font-size: 10px;
  position: relative;
  top: 0;
  right: 18px;
  content: '⌄';
  pointer-events: none;
}
select {
  appearance: none;
  -moz-appearance: none;
  -ms-appearance: none;
  -webkit-appearance: none;
  background-color: var(--bg-color);
  border: var(--control-border);
  border-radius: var(--control-border-radius);
  color: var(--text-color);
  font-family: var(--font-family);
  line-height: var(--control-line-height);
  padding: 6px 24px 4px 6px;
}
select:disabled {
  opacity: 0.5;
}

/* TODO: Also style range inputs since they're used by <options-dialog>. */
`);
