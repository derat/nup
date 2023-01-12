// Copyright 2020 Daniel Erat.
// All rights reserved.

import { $, commonStyles, handleFetchError, smallCoverSize } from './common.js';
import { getConfig, Pref, Theme } from './config.js';
import type { PlayView } from './play-view.js';
import type { SearchView } from './search-view.js';

document.adoptedStyleSheets = [commonStyles];

// Import web components so they'll be included in the bundle.
// If we weren't bundling, it'd be faster to load these from index.html.
import './audio-wrapper.js';
import './fullscreen-overlay.js';
import './play-view.js';
import './search-view.js';
import './song-table.js';
import './tag-suggester.js';

const config = getConfig();
const playView = document.querySelector('play-view') as PlayView;
const searchView = document.querySelector('search-view') as SearchView;

// Watch for theme changes.
// The initial state is handled by code inlined in index.html's body.
const darkMediaQuery = '(prefers-color-scheme: dark)';
const updateTheme = () => {
  let dark = false;
  switch (config.get(Pref.THEME)) {
    case Theme.AUTO:
      dark = window.matchMedia(darkMediaQuery).matches;
      break;
    case Theme.LIGHT:
      break;
    case Theme.DARK:
      dark = true;
      break;
  }
  if (dark) document.documentElement.setAttribute('data-theme', 'dark');
  else document.documentElement.removeAttribute('data-theme');
};
config.addCallback((k: string, _) => k === Pref.THEME && updateTheme());
window.matchMedia(darkMediaQuery).addListener((e) => updateTheme());

// Tags known by the server.
let serverTags: string[] = [];

// Returns a promise that will be resolved once tags are fetched.
const fetchServerTags = () =>
  fetch('tags', { method: 'GET' })
    .then((res) => handleFetchError(res))
    .then((res) => res.json())
    .then((tags: string[]) => {
      console.log(`Fetched ${tags.length} tag(s)`);
      serverTags = playView.tags = searchView.tags = tags;
    })
    .catch((err) => {
      console.error(`Failed fetching tags: ${err}`);
    });
fetchServerTags();

// Use the cover art as the favicon.
playView.addEventListener('cover', ((e: CustomEvent) => {
  const favicon = $('favicon') as HTMLLinkElement;
  const setSize = (s: string) => favicon.sizes.replace(favicon.sizes[0], s);
  if (e.detail.url) {
    favicon.href = e.detail.url;
    // The server can fall back to JPEG here if it doesn't have a WebP image
    // at the requested size, but I'm guessing that the browser will sniff the
    // type anyway.
    favicon.type = 'image/webp';
    setSize(`${smallCoverSize}x${smallCoverSize}`);
  } else {
    favicon.href = 'favicon-v1.ico';
    favicon.type = 'image/png';
    setSize('48x48');
  }
}) as EventListenerOrEventListenerObject);

// Wire up components.
playView.addEventListener('field', ((e: CustomEvent) => {
  searchView.resetFields(e.detail.artist, e.detail.album, e.detail.albumId);
}) as EventListenerOrEventListenerObject);
playView.addEventListener('newtags', ((e: CustomEvent) => {
  serverTags = serverTags.concat(e.detail.tags);
  playView.tags = searchView.tags = serverTags;
}) as EventListenerOrEventListenerObject);
searchView.addEventListener('enqueue', ((e: CustomEvent) => {
  playView.enqueueSongs(
    e.detail.songs,
    e.detail.clearFirst,
    e.detail.afterCurrent
  );
}) as EventListenerOrEventListenerObject);

// Used by web tests.
(document as any).test = {
  reset: () => {
    playView.resetForTest();
    searchView.resetForTest();
    // Make a hacky attempt to close any modal dialogs.
    [...document.querySelectorAll('dialog')].forEach((d) => d.close());
  },
  setPlayDelayMs: (ms: number) => playView.setPlayDelayMsForTest(ms),
  updateTags: async () => await fetchServerTags(),
  dragElement: (
    src: HTMLElement,
    dest: HTMLElement,
    offsetX: number,
    offsetY: number
  ) => {
    const dataTransfer = { setDragImage: () => {} };
    let dropEffect = 'none';
    Object.defineProperty(dataTransfer, 'dropEffect', {
      get: () => dropEffect,
      set: (v) => (dropEffect = v),
    });

    const makeEvent = (type: string, clientX: number, clientY: number) => {
      const ev = new DragEvent(type, {
        bubbles: true,
        cancelable: true,
        composed: true, // trigger listeners outside of shadow root
      });
      // https://stackoverflow.com/a/39066443
      Object.defineProperty(ev, 'dataTransfer', { value: dataTransfer });
      Object.defineProperty(ev, 'clientX', { value: clientX });
      Object.defineProperty(ev, 'clientY', { value: clientY });
      return ev;
    };

    const srcRect = src.getBoundingClientRect();
    const srcX = srcRect.x + srcRect.width / 2;
    const srcY = srcRect.y + srcRect.height / 2;
    const destRect = dest.getBoundingClientRect();
    const destX = destRect.x + destRect.width / 2 + (offsetX ?? 0);
    const destY = destRect.y + destRect.height / 2 + (offsetY ?? 0);

    src.dispatchEvent(makeEvent('dragstart', srcX, srcY));
    dest.dispatchEvent(makeEvent('dragenter', destX, destY));
    dest.dispatchEvent(makeEvent('dragover', destX, destY));
    dest.dispatchEvent(makeEvent('dragend', destX, destY));
  },
};
