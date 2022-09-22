// Copyright 2010 Daniel Erat.
// All rights reserved.

import type { AudioWrapper } from './audio-wrapper.js';
import {
  $,
  clamp,
  commonStyles,
  createShadow,
  createTemplate,
  emptyImg,
  formatDuration,
  getCoverUrl,
  getDumpSongUrl,
  getRatingString,
  getSongUrl,
  moveItem,
  preloadImage,
  setIcon,
  smallCoverSize,
  spinnerIcon,
  starIcon,
  updateTitleAttributeForTruncation,
} from './common.js';
import { getConfig, GainType, Pref } from './config.js';
import { isDialogShown } from './dialog.js';
import type { FullscreenOverlay } from './fullscreen-overlay.js';
import { createMenu, isMenuShown } from './menu.js';
import { showOptionsDialog } from './options-dialog.js';
import { showSongInfoDialog } from './song-info-dialog.js';
import type { SongTable } from './song-table.js';
import { showStatsDialog } from './stats-dialog.js';
import UpdateDialog from './update-dialog.js';
import Updater from './updater.js';

const template = createTemplate(`
<style>
  :host {
    display: flex;
    flex-direction: column;
    overflow: hidden;
    position: relative; /* needed for menu-button */
  }

  #menu-button {
    cursor: pointer;
    font-size: 32px;
    padding: var(--margin);
    padding-bottom: calc(var(--margin) / 2);
    position: absolute;
    right: 0;
    top: -24px;
    user-select: none;
  }

  #song-info {
    display: flex;
    margin: var(--margin);
  }

  #cover-div {
    align-items: center;
    display: flex;
    justify-content: center;
    margin-right: var(--margin);
  }
  #cover-div.empty {
    background-color: var(--cover-missing-color);
    outline: solid 1px var(--border-color);
    outline-offset: -1px;
  }
  #cover-img {
    cursor: pointer;
    height: 70px;
    object-fit: cover;
    user-select: none;
    width: 70px;
  }
  #cover-div.empty #cover-img {
    /* Make fully transparent rather than using visibility: hidden so the
     * image will still be clickable even if the cover is missing. */
    opacity: 0;
  }

  #spinner {
    display: none;
    fill: #fff;
    filter: drop-shadow(0 0 4px #000);
    height: 14px;
    left: 62px;
    opacity: 0.8;
    position: absolute;
    top: 15px;
    width: 14px;
  }
  #spinner.visible {
    display: block;
  }

  #rating-overlay {
    display: flex;
    filter: drop-shadow(0 0 4px #000);
    left: calc(var(--margin) + 1px);
    pointer-events: none;
    position: absolute;
    top: calc(var(--margin) + 54px);
    user-select: none;
  }
  #rating-overlay svg {
    fill: #fff;
    height: 15px;
    margin-right: -2px;
    width: 15px;
  }

  #details {
    line-height: 1.3;
    white-space: nowrap;
  }
  #artist {
    font-weight: bold;
  }
  #title {
    font-style: italic;
  }
  #time {
    opacity: 0.7;
    /* Add a layout boundary since we update this frequently:
     * http://blog.wilsonpage.co.uk/introducing-layout-boundaries/
     * Oddly, doing this on #details doesn't seem to have the same effect. */
    height: 17px;
    overflow: hidden;
    width: 100px;
  }
  #controls {
    margin: var(--margin);
    margin-top: 0;
    user-select: none;
    white-space: nowrap;
  }
  #controls button {
    width: 44px;
  }
  #controls > *:not(:first-child) {
    margin-left: var(--button-spacing);
  }
  #play-pause svg:first-child {
    display: none;
  }
  #play-pause.playing svg:first-child {
    display: inline;
  }
  #play-pause.playing svg:last-child {
    display: none;
  }
</style>

<fullscreen-overlay></fullscreen-overlay>

<div id="menu-button">…</div>

<audio-wrapper></audio-wrapper>

<div id="song-info">
  <div id="cover-div">
    <img id="cover-img" alt="" />
    <svg id="spinner"></svg>
    <div id="rating-overlay"></div>
  </div>
  <div id="details">
    <div id="artist"></div>
    <div id="title"></div>
    <div id="album"></div>
    <div id="time"></div>
  </div>
</div>

<!-- prettier-ignore -->
<div id="controls">
  <button id="prev" disabled title="Previous song (Alt+P)">
    <!-- "icon-step_backward" from MFG Labs -->
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1696.9295 2545.2094" width="14" height="14">
      <path d="M0 1906V606q0-50 35.5-85.5T121 485h239q50 0 85 35.5t35 85.5v557l1057-655q60-39 102.5-14t42.5 100v1323q0 76-42.5 101t-102.5-15L480 1349v557q0 50-35.5 85.5T360 2027H121q-49 0-85-36t-36-85z"/>
    </svg>
  </button>
  <button id="play-pause" disabled title="Pause (Space)">
    <!-- "icon-pause" from MFG Labs -->
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1632.1216 2545.2094" width="14" height="14">
      <path d="M0 1963q0 55 38 94t93 39h260q55 0 93-39t38-94V547q0-55-38-93t-93-38H131q-55 0-93 38T0 547v1416zm983 0q0 55 38.5 94t92.5 39h261q54 0 92.5-39t38.5-94V547q0-55-38.5-93t-92.5-38h-261q-54 0-92.5 38T983 547v1416z"/>
    </svg>
    <!-- "icon-play" from MFG Labs -->
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1350 2545.2094" width="14" height="14">
      <path d="M0 1950V562q0-79 45.5-105.5T153 472l1156 715q41 29 41 69 0 18-10 35.5t-20 25.5l-11 8-1156 716q-62 41-107.5 14.5T0 1950z"/>
    </svg>
  </button>
  <button id="next" disabled title="Next song (Alt+N)">
    <!-- "icon-step_forward" from MFG Labs -->
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1795 2545.2094" width="14" height="14">
      <path d="M0 1921V531q0-84 43-94.5T174 473l1100 695V531q0-43 36.5-80t79.5-37h232q81 0 127 35t46 82v1390q0 47-46 82t-127 35h-232q-43 0-79.5-37t-36.5-80v-579L174 2038q-88 40-131 7.5T0 1921z"/>
    </svg>
  </button>
</div>

<song-table id="playlist"></song-table>
`);

const SEEK_SEC = 10; // seconds to skip when seeking forward or back
const NOTIFICATION_SEC = 3; // duration to show song-change notification
const PLAY_DELAY_MS = 500; // delay before playing when cycling track
const PRELOAD_SEC = 20; // seconds before end of song to load next song
const UPDATE_POSITION_SLOP_MS = 10; // time to wait past second boundary

// <play-view> plays and displays information about songs. It also maintains
// and displays a playlist. Songs can be enqueued by calling enqueueSongs().
//
// When new tags are created, a 'newtags' CustomEvent with a 'detail.tags'
// property containing a string array of the new tags is emitted.
//
// When an artist or album field in the playlist is clicked, a 'field'
// CustomEvent is emitted. See <song-table> for more details.
//
// When the current cover art changes due to a song change, a 'cover'
// CustomEvent is emitted with a 'detail.url' string property corresponding to a
// URL to a |smallCoverSize| WebP image. This property is null if no cover art
// is available.
export class PlayView extends HTMLElement {
  #config = getConfig();
  #updater: Updater | null = null; // initialized in connectedCallback()
  #songs: Song[] = []; // playlist
  #tags: string[] = []; // available tags loaded from server
  #currentIndex = -1; // index into #songs of current track
  #startTime: Date | null = null; // time at which current track started playing
  #reportedCurrentTrack = false; // already reported current as played?
  #reachedEndOfSongs = false; // did we hit end of last song?
  #updateDialog: UpdateDialog | null = null; // edit rating and tags
  #notification: Notification | null = null; // displays song changes
  #closeNotificationTimeoutId: number | null = null;
  #playDelayMs = PLAY_DELAY_MS;
  #playTimeoutId: number | null = null; // for #playInternal()
  #lastUpdatePosition = 0; // audio position in last #updatePosition()
  #updatePositionTimeoutId: number | null = null;
  #shuffled = false; // playlist contains shuffled songs

  #shadow = createShadow(this, template);
  #overlay = this.#shadow.querySelector(
    'fullscreen-overlay'
  ) as FullscreenOverlay;
  #audio = this.#shadow.querySelector('audio-wrapper') as AudioWrapper;
  #playlistTable = $('playlist', this.#shadow) as SongTable;

  #coverDiv = $('cover-div', this.#shadow);
  #coverImage = $('cover-img', this.#shadow) as HTMLImageElement;
  #spinner = $('spinner', this.#shadow);
  #ratingOverlay = $('rating-overlay', this.#shadow);
  #artistDiv = $('artist', this.#shadow);
  #titleDiv = $('title', this.#shadow);
  #albumDiv = $('album', this.#shadow);
  #timeDiv = $('time', this.#shadow);
  #prevButton = $('prev', this.#shadow) as HTMLButtonElement;
  #nextButton = $('next', this.#shadow) as HTMLButtonElement;
  #playPauseButton = $('play-pause', this.#shadow) as HTMLButtonElement;

  constructor() {
    super();

    this.#shadow.adoptedStyleSheets = [commonStyles];

    this.#overlay.addEventListener('next', () => this.#cycleTrack(1));

    // We're leaking this callback, but it doesn't matter in practice since
    // play-view never gets removed from the DOM.
    this.#config.addCallback((name, value) => {
      if ([Pref.GAIN_TYPE, Pref.PRE_AMP].includes(name)) {
        this.#updateGain();
      }
    });

    this.#spinner = setIcon(this.#spinner, spinnerIcon);

    const menuButton = $('menu-button', this.#shadow);
    menuButton.addEventListener('click', () => {
      const rect = menuButton.getBoundingClientRect();
      createMenu(
        rect.right + 12, // compensate for right padding
        rect.bottom,
        [
          {
            id: 'fullscreen',
            text: 'Fullscreen',
            cb: () => (this.#overlay.visible = true),
            hotkey: 'Alt+V',
          },
          {
            id: 'options',
            text: 'Options…',
            cb: showOptionsDialog,
            hotkey: 'Alt+O',
          },
          {
            id: 'stats',
            text: 'Stats…',
            cb: showStatsDialog,
            hotkey: 'Alt+S',
          },
          {
            id: 'info',
            text: 'Song info…',
            cb: () => {
              const song = this.#currentSong;
              if (song) showSongInfoDialog(song, true /* isCurrent */);
            },
            hotkey: 'Alt+I',
          },
          {
            id: 'debug',
            text: 'Debug…',
            cb: () => {
              const song = this.#currentSong;
              if (song) window.open(getDumpSongUrl(song.songId), '_blank');
            },
            hotkey: 'Alt+D',
          },
        ],
        true /* alignRight */
      );
    });

    this.#audio.addEventListener('ended', this.#onEnded);
    this.#audio.addEventListener('pause', this.#onPause);
    this.#audio.addEventListener('play', this.#onPlay);
    this.#audio.addEventListener('playing', this.#onPlaying);
    this.#audio.addEventListener('timeupdate', this.#onTimeUpdate);
    this.#audio.addEventListener('error', this.#onError);

    this.#coverImage.addEventListener('click', () => this.#showUpdateDialog());
    this.#coverImage.addEventListener('load', () =>
      this.#updateMediaSessionMetadata(true /* imageLoaded */)
    );

    this.#prevButton.addEventListener('click', () =>
      this.#cycleTrack(-1, true /* delayPlay */)
    );
    this.#nextButton.addEventListener('click', () =>
      this.#cycleTrack(1, true /* delayPlay */)
    );
    this.#playPauseButton.addEventListener('click', () => this.#togglePause());

    this.#playlistTable.addEventListener('field', ((e: CustomEvent) => {
      this.dispatchEvent(new CustomEvent('field', { detail: e.detail }));
    }) as EventListenerOrEventListenerObject);
    this.#playlistTable.addEventListener('reorder', ((e: CustomEvent) => {
      this.#currentIndex = moveItem(
        this.#songs,
        e.detail.fromIndex,
        e.detail.toIndex,
        this.#currentIndex
      )!;
      this.#updateOverlaySongs();
      this.#updateButtonState();
      // TODO: Preload the next song if needed.
    }) as EventListenerOrEventListenerObject);
    this.#playlistTable.addEventListener('menu', ((e: CustomEvent) => {
      const idx = e.detail.index;
      const orig = e.detail.orig;
      orig.preventDefault();

      const menu = createMenu(orig.pageX, orig.pageY, [
        {
          id: 'play',
          text: 'Play',
          cb: () => this.#selectTrack(idx),
        },
        {
          id: 'remove',
          text: 'Remove',
          cb: () => this.#removeSongs(idx, 1),
        },
        {
          id: 'truncate',
          text: 'Truncate',
          cb: () => this.#removeSongs(idx, this.#songs.length - idx),
        },
        { text: '-' },
        {
          id: 'info',
          text: 'Info…',
          cb: () => showSongInfoDialog(this.#songs[idx]),
        },
        {
          id: 'debug',
          text: 'Debug…',
          cb: () => window.open(getDumpSongUrl(e.detail.songId), '_blank'),
        },
      ]);

      // Highlight the row while the menu is open.
      this.#playlistTable.setRowMenuShown(idx, true);
      menu.addEventListener('close', () => {
        this.#playlistTable.setRowMenuShown(idx, false);
      });
    }) as EventListenerOrEventListenerObject);

    this.#updateSongDisplay();
  }

  connectedCallback() {
    this.#updater = new Updater();

    if ('mediaSession' in navigator) {
      const ms = navigator.mediaSession;
      ms.setActionHandler('play', () => this.#play());
      ms.setActionHandler('pause', () => this.#pause());
      ms.setActionHandler('seekbackward', () => this.#seek(-SEEK_SEC));
      ms.setActionHandler('seekforward', () => this.#seek(SEEK_SEC));
      ms.setActionHandler('previoustrack', () =>
        this.#cycleTrack(-1, true /* delayPlay */)
      );
      ms.setActionHandler('nexttrack', () =>
        this.#cycleTrack(1, true /* delayPlay */)
      );
    }

    document.addEventListener(
      'visibilitychange',
      this.#onDocumentVisibilityChange
    );
    document.body.addEventListener('keydown', this.#onKeyDown);
    window.addEventListener('beforeunload', this.#onBeforeUnload);
  }

  disconnectedCallback() {
    this.#updater?.destroy();
    this.#updater = null;

    this.#cancelCloseNotificationTimeout();
    this.#cancelPlayTimeout();
    this.#cancelUpdatePositionTimeout();

    if ('mediaSession' in navigator) {
      const ms = navigator.mediaSession;
      ms.setActionHandler('play', null);
      ms.setActionHandler('pause', null);
      ms.setActionHandler('seekbackward', null);
      ms.setActionHandler('seekforward', null);
      ms.setActionHandler('previoustrack', null);
      ms.setActionHandler('nexttrack', null);
    }

    document.removeEventListener(
      'visibilitychange',
      this.#onDocumentVisibilityChange
    );
    document.body.removeEventListener('keydown', this.#onKeyDown);
    window.removeEventListener('beforeunload', this.#onBeforeUnload);
  }

  set tags(tags: string[]) {
    this.#tags = tags;
  }

  #onDocumentVisibilityChange = () => {
    // We hold off on updating the displayed time while the document is
    // hidden, so update it as soon as the document is shown.
    if (!document.hidden) this.#updatePosition();
  };

  #onKeyDown = (e: KeyboardEvent) => {
    if (isDialogShown() || isMenuShown()) return;

    if (
      (() => {
        if (e.altKey && e.key === 'd') {
          const song = this.#currentSong;
          if (song) window.open(getDumpSongUrl(song.songId), '_blank');
          this.#overlay.visible = false;
          return true;
        } else if (e.altKey && e.key === 'i') {
          const song = this.#currentSong;
          if (song) showSongInfoDialog(song, true /* isCurrent */);
          this.#overlay.visible = false;
          return true;
        } else if (e.altKey && e.key === 'n') {
          this.#cycleTrack(1, true /* delayPlay */);
          return true;
        } else if (e.altKey && e.key === 'o') {
          showOptionsDialog();
          this.#overlay.visible = false;
          return true;
        } else if (e.altKey && e.key === 'p') {
          this.#cycleTrack(-1, true /* delayPlay */);
          return true;
        } else if (e.altKey && e.key === 'r') {
          this.#showUpdateDialog();
          this.#updateDialog?.focusRating();
          this.#overlay.visible = false;
          return true;
        } else if (e.altKey && e.key === 's') {
          showStatsDialog();
          this.#overlay.visible = false;
        } else if (e.altKey && e.key === 't') {
          this.#showUpdateDialog();
          this.#updateDialog?.focusTags();
          this.#overlay.visible = false;
          return true;
        } else if (e.altKey && e.key === 'v') {
          this.#overlay.visible = !this.#overlay.visible;
          return true;
        } else if (e.key === ' ') {
          this.#togglePause();
          return true;
        } else if (e.key === 'Escape' && this.#overlay.visible) {
          this.#overlay.visible = false;
          return true;
        } else if (e.key === 'ArrowLeft') {
          this.#seek(-SEEK_SEC);
          return true;
        } else if (e.key === 'ArrowRight') {
          this.#seek(SEEK_SEC);
          return true;
        }
        return false;
      })()
    ) {
      e.preventDefault();
      e.stopPropagation();
    }
  };

  #onBeforeUnload = () => {
    this.#closeNotification();
  };

  get #currentSong() {
    return this.#songs[this.#currentIndex] ?? null;
  }
  get #nextSong() {
    return this.#songs[this.#currentIndex + 1] ?? null;
  }

  resetForTest() {
    if (this.#songs.length) this.#removeSongs(0, this.#songs.length);
    this.#updateDialog?.close(false /* save */);
  }
  setPlayDelayMsForTest(ms: number) {
    this.#playDelayMs = ms;
  }

  // Adds |songs| to the playlist.
  // If |clearFirst| is true, the existing playlist is cleared first.
  // If |afterCurrent| is true, |songs| are inserted immediately after the
  // current song. Otherwise, they are appended to the end of the playlist.
  // |shuffled| is used for the 'auto' gain adjustment setting.
  enqueueSongs(
    songs: Song[],
    clearFirst: boolean,
    afterCurrent: boolean,
    shuffled: boolean
  ) {
    if (clearFirst) this.#removeSongs(0, this.#songs.length);

    let index = afterCurrent
      ? Math.min(this.#currentIndex + 1, this.#songs.length)
      : this.#songs.length;
    songs.forEach((s) => this.#songs.splice(index++, 0, s));

    if (shuffled && songs.length) this.#shuffled = true;

    this.#playlistTable.setSongs(this.#songs);

    if (this.#currentIndex === -1) {
      this.#selectTrack(0);
    } else if (this.#reachedEndOfSongs) {
      this.#cycleTrack(1);
    } else {
      this.#updateButtonState();
      this.#updateOverlaySongs();
    }
  }

  // Removes |len| songs starting at index |start| from the playlist.
  #removeSongs(start: number, len: number) {
    if (start < 0 || len <= 0 || start + len > this.#songs.length) return;

    this.#songs.splice(start, len);
    this.#playlistTable.setSongs(this.#songs);

    if (!this.#songs.length) this.#shuffled = false;

    // If we're keeping the current song, things are pretty simple.
    const end = start + len - 1;
    if (start > this.#currentIndex || end < this.#currentIndex) {
      // If we're removing songs before the current one, we need to update the
      // index and highlighting.
      if (end < this.#currentIndex) {
        this.#playlistTable.setRowActive(this.#currentIndex, false);
        this.#currentIndex -= len;
        this.#playlistTable.setRowActive(this.#currentIndex, true);
      }
      this.#updateButtonState();
      this.#updateOverlaySongs();
      return;
    }

    // Stop playing the (just-removed) current song and choose a new one.
    this.#audio.src = null;
    this.#playlistTable.setRowActive(this.#currentIndex, false);
    this.#currentIndex = -1;

    // If there are songs after the last-removed one, switch to the first of
    // them.
    if (this.#songs.length > start) {
      this.#selectTrack(start);
      return;
    }

    // Otherwise, we truncated the playlist, i.e. we deleted all songs from
    // the currently-playing one to the end. Jump to the last song.
    // TODO: Pausing is hokey. It'd probably be better to act as if we'd
    // actually reached the end of the last song, but that'd probably require
    // waiting for its duration to be loaded so we can seek.
    this.#selectTrack(this.#songs.length);
    this.#pause();
  }

  // Plays the song at |offset| in the playlist relative to the current song.
  // If |delayPlay| is true, waits a bit before actually playing the audio
  // (in case the user is going to cycle the track again).
  #cycleTrack(offset: number, delayPlay = false) {
    this.#selectTrack(this.#currentIndex + offset, delayPlay);
  }

  // Plays the song at |index| in the playlist.
  // If |delayPlay| is true, waits a bit before actually playing the audio
  // (in case the user might be about to select a different track).
  #selectTrack(index: number, delayPlay = false) {
    if (!this.#songs.length) {
      this.#currentIndex = -1;
      this.#updateSongDisplay();
      this.#updateButtonState();
      this.#updateOverlaySongs();
      this.#reachedEndOfSongs = false;
      return;
    }

    index = clamp(index, 0, this.#songs.length - 1);
    if (index === this.#currentIndex) return;

    this.#playlistTable.setRowActive(this.#currentIndex, false);
    this.#playlistTable.setRowActive(index, true);
    this.#playlistTable.scrollToRow(index);
    this.#currentIndex = index;
    this.#audio.src = null;

    this.#updateSongDisplay();
    this.#updateOverlaySongs();
    this.#updateButtonState();
    this.#play(delayPlay);

    if (document.hidden) this.#showNotification();
  }

  #updateButtonState() {
    this.#prevButton.disabled = this.#currentIndex <= 0;
    this.#nextButton.disabled =
      this.#currentIndex < 0 || this.#currentIndex >= this.#songs.length - 1;
    this.#playPauseButton.disabled = this.#currentIndex < 0;
  }

  #updateSongDisplay() {
    const song = this.#currentSong;
    document.title = song ? `${song.artist} - ${song.title}` : 'nup';

    this.#artistDiv.innerText = song ? song.artist : '';
    this.#titleDiv.innerText = song ? song.title : '';
    this.#albumDiv.innerText = song ? song.album : '';
    this.#timeDiv.innerText = '';

    updateTitleAttributeForTruncation(this.#artistDiv, song ? song.artist : '');
    updateTitleAttributeForTruncation(this.#titleDiv, song ? song.title : '');
    updateTitleAttributeForTruncation(this.#albumDiv, song ? song.album : '');

    if (song && song.coverFilename) {
      const url = getCoverUrl(song.coverFilename, smallCoverSize);
      this.#coverImage.src = url;
      this.#coverDiv.classList.remove('empty');
      this.dispatchEvent(new CustomEvent('cover', { detail: { url } }));
    } else {
      this.#coverImage.src = emptyImg;
      this.#coverDiv.classList.add('empty');
      this.dispatchEvent(new CustomEvent('cover', { detail: { url: null } }));
    }

    // Cache the scaled cover images for the next song and the one after it.
    // This prevents ugly laggy updates here and in <fullscreen-overlay>.
    // Note that this will probably only work for non-admin users due to an
    // App Engine "feature": https://github.com/derat/nup/issues/1
    const precacheCover = (s?: Song) => {
      if (!s?.coverFilename) return;
      preloadImage(getCoverUrl(s.coverFilename, smallCoverSize));
    };
    precacheCover(this.#songs[this.#currentIndex + 1]);
    precacheCover(this.#songs[this.#currentIndex + 2]);

    this.#updateCoverTitleAttribute();
    this.#updateRatingOverlay();
    // Metadata will be updated again after |#coverImage| is loaded.
    this.#updateMediaSessionMetadata(false /* imageLoaded */);
  }

  #updateCoverTitleAttribute() {
    const song = this.#currentSong;
    if (!song) {
      this.#coverImage.title = '';
      return;
    }

    this.#coverImage.title =
      (song.rating ? 'Rating: ' : '') +
      getRatingString(song.rating) +
      '\n' +
      (song.tags.length
        ? 'Tags: ' + song.tags.sort().join(' ')
        : '(Alt+R or Alt+T to edit)');
  }

  #updateRatingOverlay() {
    const stars = this.#currentSong?.rating ?? 0;
    const overlay = this.#ratingOverlay;
    while (overlay.children.length > stars) {
      overlay.removeChild(overlay.lastChild!);
    }
    while (this.#ratingOverlay.children.length < stars) {
      overlay.appendChild(starIcon.content.firstElementChild!.cloneNode(true));
    }
  }

  #updateMediaSessionMetadata(imageLoaded: boolean) {
    if (!('mediaSession' in navigator)) return;

    const song = this.#currentSong;
    if (!song) {
      navigator.mediaSession.metadata = null;
      return;
    }

    const artwork: MediaImage[] = [];
    if (imageLoaded) {
      const img = this.#coverImage;
      artwork.push({
        src: img.src,
        sizes: `${img.naturalWidth}x${img.naturalHeight}`,
        type: 'image/webp',
      });
    }
    navigator.mediaSession.metadata = new MediaMetadata({
      title: song.title,
      artist: song.artist,
      album: song.album,
      artwork,
    });
  }

  #updateOverlaySongs() {
    this.#overlay.updateSongs(
      this.#currentSong,
      this.#songs[this.#currentIndex + 1] ?? null
    );
  }

  #showNotification() {
    if (!('Notification' in window)) return;

    if (Notification.permission !== 'granted') {
      if (Notification.permission !== 'denied') {
        Notification.requestPermission();
      }
      return;
    }

    this.#closeNotification();
    this.#cancelCloseNotificationTimeout();

    const song = this.#currentSong;
    if (!song) return;

    const options: NotificationOptions = {
      body: `${song.title}\n${song.album}\n${formatDuration(song.length)}`,
    };
    if (song.coverFilename) {
      options.icon = getCoverUrl(song.coverFilename, smallCoverSize);
    }
    this.#notification = new Notification(`${song.artist}`, options);
    this.#closeNotificationTimeoutId = window.setTimeout(() => {
      this.#closeNotificationTimeoutId = null;
      this.#closeNotification();
    }, NOTIFICATION_SEC * 1000);
  }

  #closeNotification() {
    this.#notification?.close();
    this.#notification = null;
  }

  #cancelCloseNotificationTimeout() {
    if (this.#closeNotificationTimeoutId === null) return;
    window.clearTimeout(this.#closeNotificationTimeoutId);
    this.#closeNotificationTimeoutId = null;
  }

  // Starts playback. If |#currentSong| isn't being played, switches to it
  // even if we were already playing. Also restarts playback if we were
  // stopped at the end of the last song in the playlist.
  //
  // If |delay| is true, waits a bit before loading media and playing;
  // otherwise starts playing immediately.
  #play(delay = false) {
    if (!this.#currentSong) return;

    this.#cancelPlayTimeout();
    this.#showSpinner(); // hidden in #onPlaying and #onError

    if (delay) {
      console.log(`Playing in ${this.#playDelayMs} ms`);
      this.#playTimeoutId = window.setTimeout(() => {
        this.#playTimeoutId = null;
        this.#playInternal();
      }, this.#playDelayMs);
    } else {
      this.#playInternal();
    }
  }

  #cancelPlayTimeout() {
    if (this.#playTimeoutId === null) return;
    window.clearTimeout(this.#playTimeoutId);
    this.#playTimeoutId = null;
  }

  // Internal method called by #play().
  #playInternal() {
    const song = this.#currentSong;
    if (!song) {
      this.#hideSpinner();
      return;
    }

    // Get an absolute URL since that's what we'll get from the <audio>
    // element: https://stackoverflow.com/a/44547904
    const url = getSongUrl(song.filename);
    if (this.#audio.src !== url || this.#reachedEndOfSongs) {
      console.log(`Starting ${song.songId} (${url})`);
      this.#audio.src = url;
      this.#audio.currentTime = 0;

      this.#startTime = new Date();
      this.#reportedCurrentTrack = false;
      this.#reachedEndOfSongs = false;
      this.#lastUpdatePosition = 0;
      this.#updateGain();
    }

    console.log('Playing');
    this.#audio.play().catch((e) => {
      // play() actually returns a promise that is resolved after playback
      // actually starts. If we change the <audio>'s src or call its pause()
      // method while in the preparatory state, it complains. Ignore those
      // errors.
      // https://developers.google.com/web/updates/2017/06/play-request-was-interrupted
      if (
        e.name === 'AbortError' &&
        (e.message.match(/interrupted by a new load request/) ||
          e.message.match(/interrupted by a call to pause/))
      ) {
        return;
      }
      throw e;
    });
  }

  // Pauses playback. Safe to call if already paused or stopped.
  #pause() {
    console.log('Pausing');
    this.#audio.pause();
  }

  #togglePause() {
    this.#audio.paused ? this.#play() : this.#pause();
  }

  #seek(seconds: number) {
    if (!this.#audio.seekable) return;
    const newTime = Math.max(this.#audio.currentTime + seconds, 0);
    if (newTime >= this.#audio.duration) return;
    this.#audio.currentTime = newTime;
    this.#updatePosition();
  }

  #onEnded = () => {
    this.#updatePosition();
    if (this.#currentIndex >= this.#songs.length - 1) {
      this.#reachedEndOfSongs = true;
    } else {
      this.#cycleTrack(1);
    }
  };

  #onPause = () => {
    this.#updatePosition();
    this.#playPauseButton.classList.remove('playing');
    this.#playPauseButton.title = 'Play (Space)';
  };

  #onPlay = () => {
    this.#updatePosition();
    this.#playPauseButton.classList.add('playing');
    this.#playPauseButton.title = 'Pause (Space)';
  };

  #onPlaying = () => {
    this.#hideSpinner();
  };

  #onTimeUpdate = () => {
    // I was hoping I could just call #scheduleUpdatePosition() here, but it
    // causes occasional failures in TestDisplayTimeWhilePlaying where it looks
    // like seconds are being skipped sometimes.
    this.#updatePosition();
  };

  #onError = () => {
    this.#hideSpinner();
    this.#cycleTrack(1);
  };

  #updatePosition() {
    const song = this.#currentSong;
    if (song === null) return;

    const pos = this.#audio.currentTime;
    const played = this.#audio.playtime;
    const dur = song.length;

    if (!this.#reportedCurrentTrack && (played >= 240 || played > dur / 2)) {
      this.#updater?.reportPlay(song.songId, this.#startTime!);
      this.#reportedCurrentTrack = true;
    }

    this.#overlay.updatePosition(pos);

    // Only update the displayed time while the document is visible.
    if (!document.hidden) {
      const str = dur ? `${formatDuration(pos)} / ${formatDuration(dur)}` : '';
      if (this.#timeDiv.innerText !== str) this.#timeDiv.innerText = str;
    }

    // Preload the next song once we're nearing the end of this one.
    if (pos >= dur - PRELOAD_SEC && this.#nextSong && !this.#audio.preloadSrc) {
      const url = getSongUrl(this.#nextSong.filename);
      console.log(`Preloading ${this.#nextSong.songId} (${url})`);
      this.#audio.preloadSrc = url;
    }

    // Only schedule the next update when we're actually making progress.
    if (pos > this.#lastUpdatePosition) this.#scheduleUpdatePosition();
    else this.#cancelUpdatePositionTimeout();
    this.#lastUpdatePosition = pos;
  }

  #scheduleUpdatePosition() {
    this.#cancelUpdatePositionTimeout();

    if (this.#audio.paused) return;

    // Schedule the next update for just after when we expect the playback
    // position to cross the next second boundary.
    const pos = this.#audio.currentTime;
    const nextMs = 1000 * (Math.floor(pos + 1) - pos);
    this.#updatePositionTimeoutId = window.setTimeout(() => {
      this.#updatePositionTimeoutId = null;
      this.#updatePosition();
    }, nextMs + UPDATE_POSITION_SLOP_MS);
  }

  #cancelUpdatePositionTimeout() {
    if (this.#updatePositionTimeoutId === null) return;
    window.clearTimeout(this.#updatePositionTimeoutId);
    this.#updatePositionTimeoutId = null;
  }

  #showSpinner = () => this.#spinner.classList.add('visible');
  #hideSpinner = () => this.#spinner.classList.remove('visible');

  #showUpdateDialog() {
    const song = this.#currentSong;
    if (this.#updateDialog || !song) return;
    this.#updateDialog = new UpdateDialog(song, this.#tags, (rating, tags) => {
      this.#updateDialog = null;

      if (rating === null && tags === null) return;

      this.#updater?.rateAndTag(song.songId, rating, tags);

      if (rating !== null) {
        song.rating = rating;
        this.#updateRatingOverlay();
      }
      if (tags !== null) {
        song.tags = tags;
        const created = tags.filter((t) => !this.#tags.includes(t));
        if (created.length > 0) {
          this.dispatchEvent(
            new CustomEvent('newtags', { detail: { tags: created } })
          );
        }
      }
      this.#updateCoverTitleAttribute();
    });
  }

  // Adjusts |audio|'s gain appropriately for the current song and settings.
  // This implements the approach described at
  // https://wiki.hydrogenaud.io/index.php?title=ReplayGain_specification.
  #updateGain() {
    let adj = this.#config.get(Pref.PRE_AMP); // decibels

    let reason = '';
    const song = this.#currentSong;
    if (song) {
      let gainType = this.#config.get(Pref.GAIN_TYPE);
      if (gainType === GainType.AUTO) {
        gainType = this.#shuffled ? GainType.TRACK : GainType.ALBUM;
      }

      if (gainType === GainType.ALBUM) {
        adj += song.albumGain ?? 0;
        reason = ' for album';
      } else if (gainType === GainType.TRACK) {
        adj += song.trackGain ?? 0;
        reason = ' for track';
      }
    }

    let scale = 10 ** (adj / 20);

    // TODO: Add an option to prevent clipping instead of always doing this?
    if (song?.peakAmp) scale = Math.min(scale, 1 / song.peakAmp);

    console.log(`Scaling amplitude by ${scale.toFixed(3)}${reason}`);
    this.#audio.gain = scale;
  }
}

customElements.define('play-view', PlayView);
