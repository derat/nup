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

// Sets |element|'s 'title' attribute to |text| if its content overflows its
// area or removes it otherwise.
//
// Note that this can be slow, as accessing |scrollWidth| and |offsetWidth| may
// trigger a reflow.
export function updateTitleAttributeForTruncation(
  element: HTMLElement,
  text: string
) {
  // TODO: This can fail to set the attribute when the content just barely
  // overflows since |scrollWidth| is infuriatingly rounded to an integer:
  // https://stackoverflow.com/q/21666892
  //
  // This hasn't been changed due to compat issues:
  // https://crbug.com/360889
  // https://groups.google.com/a/chromium.org/g/blink-dev/c/_Q7A4AQBFKY
  //
  // getClientBoundingRect() and getClientRects() use fractional units but only
  // report the actual layout size, so we get the same width for all elements
  // regardless of the content size.
  //
  // It sounds like it may be possible to get the actual size by setting
  // 'width: max-content' and then calling getClientBoundingRect() as described
  // at https://github.com/w3c/csswg-drafts/issues/4123, but that seems like it
  // might be slow.
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

// Converts a rating in the range [0, 5] (0 for unrated) to a string.
export function getRatingString(rating: number) {
  rating = clamp(Math.round(rating), 0, 5);
  return rating === 0 || isNaN(rating)
    ? 'Unrated'
    : '★'.repeat(rating) + '☆'.repeat(5 - rating);
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

// Returns true if the code is currently being tested.
// navigator.webdriver is set by Chrome when running under Selenium, while
// navigator.unitTest is injected by the unit test code's fake implementation.
export const underTest = () =>
  !!(navigator as any).webdriver || !!(navigator as any).unitTest;

// "icon-cross_mark" from MFG Labs.
export const xIcon = createTemplate(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1480.6939 2545.2094" class="x-icon" width="32" height="32"><path d="M0 1788q0-73 53-126l400-400L53 861Q0 809 0 736t53-125 125.5-52T303 611l401 401 400-401q52-52 125-52t125 52 52 125-52 125l-400 401 400 400q52 53 52 126t-52 125q-51 52-125 52t-125-52l-400-400-401 400q-51 52-125 52t-125-52q-53-52-53-125z"/></svg>
`);

// "icon-star" from MFG Labs.
export const starIcon = createTemplate(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1907 2545.2094" width="32" height="32"><path d="M3 1031q24-76 208-76h462l141-425v-2q38-114 89-154.5t101.5 0T1093 528l1 2 140 425h462q120 0 174 35t30.5 94.5T1779 1214l-10 8-362 259 139 424 3 4q56 174-9 222.5t-214-58.5l-6-4-5-3-362-260-367 263-4 4q-149 107-215 58.5t-8-222.5l1-2v-3l139-423-362-259-4-4-5-4Q-21 1107 3 1031z"/></svg>
`);

// "icon-star_empty" from MFG Labs.
export const emptyStarIcon = createTemplate(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 2043 2545.2094" width="32" height="32"><path d="M6.5 1079Q-19 1015 39 977.5T226 940h495l151-456v-2q30-92 69-139t81-47q41 0 80 47t69 139l2 2 149 456h495q129 0 187 37.5t32.5 101.5-130.5 139l-10 8-389 278 150 452 2 7q39 122 22.5 186.5T1600 2214q-75 0-179-77l-12-7-387-278-393 281-6 4q-107 79-180 79-65 0-81.5-65.5T384 1963l2-7 150-452-389-278-4-5-6-3Q32 1143 6.5 1079zm338.5 53l416 297-43 134-117 354 421-301 420 301-116-354-44-134 416-297h-514l-43-132-119-359-163 491H345z"/></svg>
`);

// "spin6" from Fontelico.
export const spinnerIcon = createTemplate(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1000 1000" width="32" height="32" class="spinner"><path class="fil0" d="M854.569 841.338c-188.268 189.444 -519.825 171.223 -704.157 -13.109 -190.56 -190.56 -200.048 -493.728 -28.483 -695.516 10.739 -12.623 21.132 -25.234 34.585 -33.667 36.553 -22.89 85.347 -18.445 117.138 13.347 30.228 30.228 35.737 75.83 16.531 111.665 -4.893 9.117 -9.221 14.693 -16.299 22.289 -140.375 150.709 -144.886 378.867 -7.747 516.005 152.583 152.584 406.604 120.623 541.406 -34.133 106.781 -122.634 142.717 -297.392 77.857 -451.04 -83.615 -198.07 -305.207 -291.19 -510.476 -222.476l-.226 -.226c235.803 -82.501 492.218 23.489 588.42 251.384 70.374 166.699 36.667 355.204 -71.697 493.53 -11.48 14.653 -23.724 28.744 -36.852 41.948z"/></svg>
`);

// setIcon clones |tmpl| (e.g. |xIcon|) and uses it to replace |orig|.
export function setIcon(orig: HTMLElement, tmpl: HTMLTemplateElement) {
  const icon = tmpl.content.firstElementChild!.cloneNode(true) as HTMLElement;
  if (orig.id) icon.id = orig.id;
  if (orig.title) icon.title = orig.title;
  icon.classList.add(...orig.classList.values());
  orig.replaceWith(icon);
  return icon;
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
  font-display: swap;
  font-family: 'Roboto';
  font-style: normal;
  font-weight: 400;
  /* prettier-ignore */
  src: local('Roboto'),
    url('roboto-v30-latin-regular.woff2') format('woff2'), /* Chrome 26+, Opera 23+, Firefox 39+ */
    url('roboto-v30-latin-regular.woff') format('woff'); /* Chrome 6+, Firefox 3.6+, IE 9+, Safari 5.1+ */
}
@font-face {
  font-display: swap;
  font-family: 'Fontello';
  font-style: normal;
  font-weight: 400;
  /* prettier-ignore */
  src: local(''),
    url('fontello-v1.woff2') format('woff2'),
    url('fontello-v1.woff') format('woff');
}

:root {
  --font-family: 'Roboto', 'Fontello', sans-serif;
  --font-size: 13.3333px;

  --control-border: 1px solid var(--control-color);
  --control-border-radius: 4px;
  --control-line-height: 16px;

  --margin: 10px; /* margin around groups of elements */
  --button-spacing: 6px; /* horizontal spacing between buttons */

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
  --button-text-color: #fff;
  --chart-bar-rgb: 66, 165, 245; /* #42a5f5, material blue 400 */
  --chart-text-color: #fff;
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
  --button-text-color: #000;
  --chart-bar-rgb: 66, 165, 245; /* #42a5f5, material blue 400 */
  --chart-text-color: #fff;
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

html {
  color-scheme: light;
}
html[data-theme='dark'] {
  color-scheme: dark;
}

svg.x-icon {
  cursor: pointer;
  fill: var(--icon-color);
  height: 12px;
  width: 12px;
}
svg.x-icon:hover {
  fill: var(--icon-hover-color);
}

svg.spinner {
  animation: spin 1s infinite linear;
  transform-origin: 50% 50%;
}
@keyframes spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

button {
  background-color: var(--button-color);
  border: none;
  border-radius: var(--control-border-radius);
  color: var(--button-text-color);
  cursor: pointer;
  display: inline-block;
  font-family: var(--font-family);
  font-size: 12px;
  font-weight: bold;
  height: 28px;
  letter-spacing: 0.09em;
  overflow: hidden; /* prevent icon from extending focus ring */
  padding: 1px 12px 0 12px;
  text-transform: uppercase;
  user-select: none;
}
button:hover:not(:disabled) {
  background-color: var(--button-hover-color);
}
button:disabled {
  box-shadow: none;
  cursor: default;
  opacity: 0.4;
}
button svg {
  fill: var(--button-text-color);
  vertical-align: middle;
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
  background-color: var(--control-active-color);
  content: '';
  height: 12px;
  left: calc(50% - 6px);
  /* "icon-check" from MFG Labs */
  /* Chrome 104 seems to still need the vendor-prefixed version of 'mask':
   * https:/crbug.com/432153 */
  mask-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 1886 2545.2094' width='12' height='12'%3E%3Cpath d='M0 1278.5Q0 1350 50 1400l503 491q50 50 124 50 72 0 122-50L1834 877q52-50 52-121t-52-120q-52-50-123.5-50T1587 636l-910 893-380-372q-52-50-123.5-50T50 1157q-50 50-50 121.5z'/%3E%3C/svg%3E");
  -webkit-mask-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 1886 2545.2094' width='12' height='12'%3E%3Cpath d='M0 1278.5Q0 1350 50 1400l503 491q50 50 124 50 72 0 122-50L1834 877q52-50 52-121t-52-120q-52-50-123.5-50T1587 636l-910 893-380-372q-52-50-123.5-50T50 1157q-50 50-50 121.5z'/%3E%3C/svg%3E");
  mask-position: center;
  -webkit-mask-position: center;
  mask-size: cover;
  -webkit-mask-size: cover;
  position: absolute;
  top: calc(50% - 6px);
  width: 12px;
}

input[type='checkbox'].small {
  height: 12px;
  width: 12px;
}
input[type='checkbox'].small:checked:before {
  height: 10px;
  left: calc(50% - 5px);
  top: calc(50% - 5px);
  width: 10px;
}

input:disabled {
  opacity: 0.5;
}

/* To avoid spacing differences between minified and non-minified code, omit
 * whitespace between </select> and the closing .select-wrapper </span> tag.
 * I think that https://github.com/tdewolff/minify/issues/240 is related. */
.select-wrapper {
  display: inline-block;
  margin: 0 4px;
  position: relative;
}
.select-wrapper:after {
  background-color: var(--icon-color);
  content: '';
  height: 12px;
  /* "icon-chevron_down" from MFG Labs */
  mask-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 1675 2545.2094' width='12' height='12'%3E%3Cpath d='M0 880q0-51 37-88t89-37 88 37l624 622 623-622q37-37 89-37t88 37q37 37 37 88.5t-37 88.5l-800 800L37 969Q0 930 0 880z'/%3E%3C/svg%3E");
  -webkit-mask-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 1675 2545.2094' width='12' height='12'%3E%3Cpath d='M0 880q0-51 37-88t89-37 88 37l624 622 623-622q37-37 89-37t88 37q37 37 37 88.5t-37 88.5l-800 800L37 969Q0 930 0 880z'/%3E%3C/svg%3E");
  mask-position: center;
  -webkit-mask-position: center;
  mask-size: cover;
  -webkit-mask-size: cover;
  pointer-events: none;
  position: absolute;
  right: 8px;
  top: calc(50% - 5px);
  width: 12px;
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
