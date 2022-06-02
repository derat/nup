// Copyright 2010 Daniel Erat.
// All rights reserved.

import type { AudioWrapper } from './audio-wrapper.js';
import {
  $,
  commonStyles,
  createShadow,
  createTemplate,
  emptyImg,
  formatDuration,
  getCoverUrl,
  getCurrentTimeSec,
  getDumpSongUrl,
  getRatingString,
  getSongUrl,
  moveItem,
  preloadImage,
  smallCoverSize,
  updateTitleAttributeForTruncation,
} from './common.js';
import Config, { getConfig } from './config.js';
import { isDialogShown } from './dialog.js';
import { createMenu, isMenuShown } from './menu.js';
import { showOptionsDialog } from './options-dialog.js';
import type { PresentationLayer } from './presentation-layer.js';
import { showSongInfo } from './song-info.js';
import type { SongTable } from './song-table.js';
import { showStats } from './stats.js';
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
    font-size: 22px;
    padding: 0 var(--margin);
    position: absolute;
    right: 0;
    top: 0;
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
  #rating-overlay {
    color: #fff;
    font-family: var(--icon-font-family);
    font-size: 12px;
    left: calc(var(--margin) + 2px);
    letter-spacing: 2px;
    pointer-events: none;
    position: absolute;
    text-shadow: 0 0 8px #000;
    top: calc(var(--margin) + 55px);
    user-select: none;
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
    font-family: var(--icon-font-family);
    font-size: 10px;
    width: 44px;
  }
  #controls > *:not(:first-child) {
    margin-left: var(--button-spacing);
  }
</style>

<presentation-layer></presentation-layer>

<div id="menu-button">⋯</div>

<audio-wrapper></audio-wrapper>

<div id="song-info">
  <div id="cover-div">
    <img id="cover-img" />
    <div id="rating-overlay"></div>
  </div>
  <div id="details">
    <div id="artist"></div>
    <div id="title"></div>
    <div id="album"></div>
    <div id="time"></div>
  </div>
</div>

<div id="controls">
  <button id="prev" disabled title="Previous song (Alt+P)">⏮</button>
  <button id="play-pause" disabled title="Pause (Space)">⏸</button>
  <button id="next" disabled title="Next song (Alt+N)">⏭</button>
</div>

<song-table id="playlist"></song-table>
`);

// <music-player> plays and displays information about songs. It also maintains
// and displays a playlist. Songs can be enqueued by calling enqueueSongs().
//
// When new tags are created, a 'newtags' CustomEvent with a 'detail.tags'
// property containing a string array of the new tags is emitted.
//
// When an artist or album field in the playlist is clicked, a 'field'
// CustomEvent is emitted. See <song-table> for more details.
//
// When the <presentation-layer>'s visibility changes, a 'present' CustomEvent
// is emitted with a 'detail.visible' boolean property. The document's scrollbar
// should be hidden while the layer is visible.
//
// When the current cover art changes due to a song change, a 'cover'
// CustomEvent is emitted with a 'detail.url' string property corresponding to a
// URL to a |smallCoverSize| WebP image. This property is null if no cover art
// is available.
customElements.define(
  'music-player',
  class MusicPlayer extends HTMLElement {
    static SEEK_SEC_ = 10; // seconds to skip when seeking forward or back
    static NOTIFICATION_SEC_ = 3; // duration to show song-change notification
    static PLAY_DELAY_MS_ = 500; // delay before playing when cycling track
    static PRELOAD_SEC_ = 20; // seconds before end of song to load next song
    static TIME_UPDATE_SLOP_MS_ = 10; // time to wait past second boundary

    config_ = getConfig();
    updater_: Updater | null = null; // initialized in connectedCallback()
    songs_: Song[] = []; // songs in the order in which they should be played
    tags_: string[] = []; // available tags loaded from server
    currentIndex_ = -1; // index into |songs| of current track
    startTime_: number | null = null; // seconds since epoch for current track
    reportedCurrentTrack_ = false; // already reported current as played?
    reachedEndOfSongs_ = false; // did we hit end of last song?
    updateDialog_: UpdateDialog | null = null;
    notification_: Notification | null = null; // for song changes
    closeNotificationTimeoutId_: number | null = null; // closeNotification_()
    playDelayMs_ = MusicPlayer.PLAY_DELAY_MS_;
    playTimeoutId_: number | null = null; // for playInternal_()
    lastTimeUpdatePos_ = 0; // audio position in last onTimeUpdate_()
    timeUpdateTimeoutId_: number | null = null; // onTimeUpdate_()
    shuffled_ = false; // playlist contains shuffled songs

    shadow_ = createShadow(this, template);
    presentationLayer_ = this.shadow_.querySelector(
      'presentation-layer'
    ) as PresentationLayer;
    audio_ = this.shadow_.querySelector('audio-wrapper') as AudioWrapper;
    playlistTable_ = $('playlist', this.shadow_) as SongTable;

    coverDiv_ = $('cover-div', this.shadow_);
    coverImage_ = $('cover-img', this.shadow_) as HTMLImageElement;
    ratingOverlayDiv_ = $('rating-overlay', this.shadow_);
    artistDiv_ = $('artist', this.shadow_);
    titleDiv_ = $('title', this.shadow_);
    albumDiv_ = $('album', this.shadow_);
    timeDiv_ = $('time', this.shadow_);
    prevButton_ = $('prev', this.shadow_) as HTMLButtonElement;
    nextButton_ = $('next', this.shadow_) as HTMLButtonElement;
    playPauseButton_ = $('play-pause', this.shadow_) as HTMLButtonElement;

    constructor() {
      super();

      this.shadow_.adoptedStyleSheets = [commonStyles];

      this.presentationLayer_.addEventListener('next', () => {
        this.cycleTrack_(1, false /* delayPlay */);
      });
      this.presentationLayer_.addEventListener('hide', () => {
        this.setPresentationLayerVisible_(false);
      });

      // We're leaking this callback, but it doesn't matter in practice since
      // music-player never gets removed from the DOM.
      this.config_.addCallback((name, value) => {
        if (name === Config.GAIN_TYPE || name === Config.PRE_AMP) {
          this.updateGain_();
        }
      });

      const menuButton = $('menu-button', this.shadow_);
      menuButton.addEventListener('click', () => {
        const rect = menuButton.getBoundingClientRect();
        createMenu(
          rect.right + 12, // compensate for right padding
          rect.bottom,
          [
            {
              id: 'present',
              text: 'Presentation',
              cb: () => this.setPresentationLayerVisible_(true),
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
              cb: showStats,
            },
            {
              id: 'info',
              text: 'Song info…',
              cb: () => {
                const song = this.currentSong_;
                if (song) showSongInfo(song);
              },
              hotkey: 'Alt+I',
            },
            {
              id: 'debug',
              text: 'Debug…',
              cb: () => {
                const song = this.currentSong_;
                if (song) window.open(getDumpSongUrl(song.songId), '_blank');
              },
              hotkey: 'Alt+D',
            },
          ],
          true /* alignRight */
        );
      });

      this.audio_.addEventListener('ended', this.onEnded_);
      this.audio_.addEventListener('pause', this.onPause_);
      this.audio_.addEventListener('play', this.onPlay_);
      this.audio_.addEventListener('timeupdate', this.onTimeUpdate_);
      this.audio_.addEventListener('error', this.onError_);

      this.coverImage_.addEventListener('click', () =>
        this.showUpdateDialog_()
      );
      this.coverImage_.addEventListener('load', () =>
        this.updateMediaSessionMetadata_(true /* imageLoaded */)
      );

      this.prevButton_.addEventListener('click', () =>
        this.cycleTrack_(-1, true /* delayPlay */)
      );
      this.nextButton_.addEventListener('click', () =>
        this.cycleTrack_(1, true /* delayPlay */)
      );
      this.playPauseButton_.addEventListener('click', () =>
        this.togglePause_()
      );

      this.playlistTable_.addEventListener('field', (e: CustomEvent) => {
        this.dispatchEvent(new CustomEvent('field', { detail: e.detail }));
      });
      this.playlistTable_.addEventListener('reorder', (e: CustomEvent) => {
        this.currentIndex_ = moveItem(
          this.songs_,
          e.detail.fromIndex,
          e.detail.toIndex,
          this.currentIndex_
        );
        this.updatePresentationLayerSongs_();
        this.updateButtonState_();
        // TODO: Preload the next song if needed.
      });
      this.playlistTable_.addEventListener('menu', (e: CustomEvent) => {
        const idx = e.detail.index;
        const orig = e.detail.orig;
        orig.preventDefault();

        const menu = createMenu(orig.pageX, orig.pageY, [
          {
            id: 'play',
            text: 'Play',
            cb: () => this.selectTrack_(idx),
          },
          {
            id: 'remove',
            text: 'Remove',
            cb: () => this.removeSongs_(idx, 1),
          },
          {
            id: 'truncate',
            text: 'Truncate',
            cb: () => this.removeSongs_(idx, this.songs_.length - idx),
          },
          { text: '-' },
          {
            id: 'info',
            text: 'Info…',
            cb: () => showSongInfo(this.songs_[idx]),
          },
          {
            id: 'debug',
            text: 'Debug…',
            cb: () => window.open(getDumpSongUrl(e.detail.songId), '_blank'),
          },
        ]);

        // Highlight the row while the menu is open.
        this.playlistTable_.setRowMenuShown(idx, true);
        menu.addEventListener('close', () => {
          this.playlistTable_.setRowMenuShown(idx, false);
        });
      });

      this.updateSongDisplay_();
    }

    connectedCallback() {
      this.updater_ = new Updater();

      if ('mediaSession' in navigator) {
        const ms = navigator.mediaSession;
        ms.setActionHandler('play', () => this.play_(false /* delay */));
        ms.setActionHandler('pause', () => this.pause_());
        ms.setActionHandler('seekbackward', () =>
          this.seek_(-MusicPlayer.SEEK_SEC_)
        );
        ms.setActionHandler('seekforward', () =>
          this.seek_(MusicPlayer.SEEK_SEC_)
        );
        ms.setActionHandler('previoustrack', () =>
          this.cycleTrack_(-1, true /* delayPlay */)
        );
        ms.setActionHandler('nexttrack', () =>
          this.cycleTrack_(1, true /* delayPlay */)
        );
      }

      document.addEventListener(
        'visibilitychange',
        this.onDocumentVisibilityChange_
      );
      document.body.addEventListener('keydown', this.onKeyDown_);
      window.addEventListener('beforeunload', this.onBeforeUnload_);
    }

    disconnectedCallback() {
      this.updater_.destroy();
      this.updater_ = null;

      this.cancelCloseNotificationTimeout_();
      this.cancelPlayTimeout_();
      this.cancelTimeUpdateTimeout_();

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
        this.onDocumentVisibilityChange_
      );
      document.body.removeEventListener('keydown', this.onKeyDown_);
      window.removeEventListener('beforeunload', this.onBeforeUnload_);
    }

    set tags(tags: string[]) {
      this.tags_ = tags;
    }

    onDocumentVisibilityChange_ = () => {
      // We hold off on updating the displayed time while the document is
      // hidden, so update it as soon as the document is shown.
      if (!document.hidden) this.onTimeUpdate_();
    };

    onKeyDown_ = (e: KeyboardEvent) => {
      if (isDialogShown() || isMenuShown()) return;

      if (
        (() => {
          if (e.altKey && e.key === 'd') {
            const song = this.currentSong_;
            if (song) window.open(getDumpSongUrl(song.songId), '_blank');
            this.setPresentationLayerVisible_(false);
            return true;
          } else if (e.altKey && e.key === 'i') {
            const song = this.currentSong_;
            if (song) showSongInfo(song);
            this.setPresentationLayerVisible_(false);
            return true;
          } else if (e.altKey && e.key === 'n') {
            this.cycleTrack_(1, true /* delay */);
            return true;
          } else if (e.altKey && e.key === 'o') {
            showOptionsDialog();
            this.setPresentationLayerVisible_(false);
            return true;
          } else if (e.altKey && e.key === 'p') {
            this.cycleTrack_(-1, true /* delay */);
            return true;
          } else if (e.altKey && e.key === 'r') {
            this.showUpdateDialog_();
            this.updateDialog_?.focusRating();
            this.setPresentationLayerVisible_(false);
            return true;
          } else if (e.altKey && e.key === 't') {
            this.showUpdateDialog_();
            this.updateDialog_?.focusTags();
            this.setPresentationLayerVisible_(false);
            return true;
          } else if (e.altKey && e.key === 'v') {
            this.setPresentationLayerVisible_(!this.presentationLayer_.visible);
            return true;
          } else if (e.key === ' ') {
            this.togglePause_();
            return true;
          } else if (e.key === 'Escape' && this.presentationLayer_.visible) {
            this.setPresentationLayerVisible_(false);
            return true;
          } else if (e.key === 'ArrowLeft') {
            this.seek_(-MusicPlayer.SEEK_SEC_);
            return true;
          } else if (e.key === 'ArrowRight') {
            this.seek_(MusicPlayer.SEEK_SEC_);
            return true;
          }
          return false;
        })()
      ) {
        e.preventDefault();
        e.stopPropagation();
      }
    };

    onBeforeUnload_ = () => {
      this.closeNotification_();
    };

    get currentSong_() {
      return this.songs_[this.currentIndex_] ?? null;
    }
    get nextSong_() {
      return this.songs_[this.currentIndex_ + 1] ?? null;
    }

    resetForTesting() {
      if (this.songs_.length) this.removeSongs_(0, this.songs_.length);
      this.updateDialog_?.close(false /* save */);
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
      if (clearFirst) this.removeSongs_(0, this.songs_.length);

      let index = afterCurrent
        ? Math.min(this.currentIndex_ + 1, this.songs_.length)
        : this.songs_.length;
      songs.forEach((s) => this.songs_.splice(index++, 0, s));

      if (shuffled && songs.length) this.shuffled_ = true;

      this.playlistTable_.setSongs(this.songs_);

      if (this.currentIndex_ === -1) {
        this.selectTrack_(0);
      } else if (this.reachedEndOfSongs_) {
        this.cycleTrack_(1);
      } else {
        this.updateButtonState_();
        this.updatePresentationLayerSongs_();
      }
    }

    // Removes |len| songs starting at index |start| from the playlist.
    removeSongs_(start: number, len: number) {
      if (start < 0 || len <= 0 || start + len > this.songs_.length) return;

      this.songs_.splice(start, len);
      this.playlistTable_.setSongs(this.songs_);

      if (!this.songs_.length) this.shuffled_ = false;

      // If we're keeping the current song, things are pretty simple.
      const end = start + len - 1;
      if (start > this.currentIndex_ || end < this.currentIndex_) {
        // If we're removing songs before the current one, we need to update the
        // index and highlighting.
        if (end < this.currentIndex_) {
          this.playlistTable_.setRowActive(this.currentIndex_, false);
          this.currentIndex_ -= len;
          this.playlistTable_.setRowActive(this.currentIndex_, true);
        }
        this.updateButtonState_();
        this.updatePresentationLayerSongs_();
        return;
      }

      // Stop playing the (just-removed) current song and choose a new one.
      this.audio_.src = null;
      this.playlistTable_.setRowActive(this.currentIndex_, false);
      this.currentIndex_ = -1;

      // If there are songs after the last-removed one, switch to the first of
      // them.
      if (this.songs_.length > start) {
        this.selectTrack_(start);
        return;
      }

      // Otherwise, we truncated the playlist, i.e. we deleted all songs from
      // the currently-playing one to the end. Jump to the last song.
      // TODO: Pausing is hokey. It'd probably be better to act as if we'd
      // actually reached the end of the last song, but that'd probably require
      // waiting for its duration to be loaded so we can seek.
      this.selectTrack_(this.songs_.length);
      this.pause_();
    }

    // Plays the song at |offset| in the playlist relative to the current song.
    // If |delayPlay| is true, waits a bit before actually playing the audio
    // (in case the user might be about to select a different track).
    cycleTrack_(offset: number, delayPlay = false) {
      this.selectTrack_(this.currentIndex_ + offset, delayPlay);
    }

    // Plays the song at |index| in the playlist.
    // If |delayPlay| is true, waits a bit before actually playing the audio
    // (in case the user might be about to select a different track).
    selectTrack_(index: number, delayPlay = false) {
      if (!this.songs_.length) {
        this.currentIndex_ = -1;
        this.updateSongDisplay_();
        this.updateButtonState_();
        this.updatePresentationLayerSongs_();
        this.reachedEndOfSongs_ = false;
        return;
      }

      if (index < 0) index = 0;
      else if (index >= this.songs_.length) index = this.songs_.length - 1;

      if (index === this.currentIndex_) return;

      this.playlistTable_.setRowActive(this.currentIndex_, false);
      this.playlistTable_.setRowActive(index, true);
      this.playlistTable_.scrollToRow(index);
      this.currentIndex_ = index;
      this.audio_.src = null;

      this.updateSongDisplay_();
      this.updatePresentationLayerSongs_();
      this.updateButtonState_();
      this.play_(delayPlay);

      if (!document.hasFocus()) this.showNotification_();
    }

    updateButtonState_() {
      this.prevButton_.disabled = this.currentIndex_ <= 0;
      this.nextButton_.disabled =
        this.currentIndex_ < 0 || this.currentIndex_ >= this.songs_.length - 1;
      this.playPauseButton_.disabled = this.currentIndex_ < 0;
    }

    updateSongDisplay_() {
      const song = this.currentSong_;
      document.title = song ? `${song.artist} - ${song.title}` : 'Player';

      this.artistDiv_.innerText = song ? song.artist : '';
      this.titleDiv_.innerText = song ? song.title : '';
      this.albumDiv_.innerText = song ? song.album : '';
      this.timeDiv_.innerText = '';

      updateTitleAttributeForTruncation(
        this.artistDiv_,
        song ? song.artist : ''
      );
      updateTitleAttributeForTruncation(this.titleDiv_, song ? song.title : '');
      updateTitleAttributeForTruncation(this.albumDiv_, song ? song.album : '');

      if (song && song.coverFilename) {
        const url = getCoverUrl(song.coverFilename, smallCoverSize);
        this.coverImage_.src = url;
        this.coverDiv_.classList.remove('empty');
        this.dispatchEvent(new CustomEvent('cover', { detail: { url } }));
      } else {
        this.coverImage_.src = emptyImg;
        this.coverDiv_.classList.add('empty');
        this.dispatchEvent(new CustomEvent('cover', { detail: { url: null } }));
      }

      // Cache the scaled cover images for the next song and the one after it.
      // This prevents ugly laggy updates here and in <presentation-layer>.
      // Note that this will probably only work for non-admin users due to an
      // App Engine "feature": https://github.com/derat/nup/issues/1
      const precacheCover = (s?: Song) => {
        if (!s?.coverFilename) return;
        preloadImage(getCoverUrl(s.coverFilename, smallCoverSize));
      };
      precacheCover(this.songs_[this.currentIndex_ + 1]);
      precacheCover(this.songs_[this.currentIndex_ + 2]);

      this.updateCoverTitleAttribute_();
      this.updateRatingOverlay_();
      // Metadata will be updated again after |coverImage_| is loaded.
      this.updateMediaSessionMetadata_(false /* imageLoaded */);
    }

    updateCoverTitleAttribute_() {
      const song = this.currentSong_;
      if (!song) {
        this.coverImage_.title = '';
        return;
      }

      this.coverImage_.title =
        getRatingString(song.rating, '★', '☆', 'Unrated', 'Rating: ') +
        '\n' +
        (song.tags.length
          ? 'Tags: ' + song.tags.sort().join(' ')
          : '(Alt+R or Alt+T to edit)');
    }

    updateRatingOverlay_() {
      const song = this.currentSong_;
      this.ratingOverlayDiv_.innerText = song
        ? getRatingString(song.rating, '★', '', '', '')
        : '';
    }

    updateMediaSessionMetadata_(imageLoaded: boolean) {
      if (!('mediaSession' in navigator)) return;

      const song = this.currentSong_;
      if (!song) {
        navigator.mediaSession.metadata = null;
        return;
      }

      const artwork: MediaImage[] = [];
      if (imageLoaded) {
        const img = this.coverImage_;
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

    updatePresentationLayerSongs_() {
      this.presentationLayer_.updateSongs(
        this.currentSong_,
        this.songs_[this.currentIndex_ + 1] ?? null
      );
    }

    showNotification_() {
      if (!('Notification' in window)) return;

      if (Notification.permission !== 'granted') {
        if (Notification.permission !== 'denied') {
          Notification.requestPermission();
        }
        return;
      }

      this.closeNotification_();
      this.cancelCloseNotificationTimeout_();

      const song = this.currentSong_;
      if (!song) return;

      const options: NotificationOptions = {
        body: `${song.title}\n${song.album}\n${formatDuration(song.length)}`,
      };
      if (song.coverFilename) {
        options.icon = getCoverUrl(song.coverFilename, smallCoverSize);
      }
      this.notification_ = new Notification(`${song.artist}`, options);
      this.closeNotificationTimeoutId_ = window.setTimeout(() => {
        this.closeNotificationTimeoutId_ = null;
        this.closeNotification_();
      }, MusicPlayer.NOTIFICATION_SEC_ * 1000);
    }

    closeNotification_() {
      this.notification_?.close();
      this.notification_ = null;
    }

    cancelCloseNotificationTimeout_() {
      if (this.closeNotificationTimeoutId_ === null) return;
      window.clearTimeout(this.closeNotificationTimeoutId_);
      this.closeNotificationTimeoutId_ = null;
    }

    // Starts playback. If |currentSong_| isn't being played, switches to it
    // even if we were already playing. Also restarts playback if we were
    // stopped at the end of the last song in the playlist.
    //
    // If |delay| is true, waits a bit before loading media and playing;
    // otherwise starts playing immediately.
    play_(delay: boolean) {
      if (!this.currentSong_) return;

      this.cancelPlayTimeout_();

      if (delay) {
        console.log(`Playing in ${this.playDelayMs_} ms`);
        this.playTimeoutId_ = window.setTimeout(() => {
          this.playTimeoutId_ = undefined;
          this.playInternal_();
        }, this.playDelayMs_);
      } else {
        this.playInternal_();
      }
    }

    cancelPlayTimeout_() {
      if (this.playTimeoutId_ === null) return;
      window.clearTimeout(this.playTimeoutId_);
      this.playTimeoutId_ = null;
    }

    // Internal method called by play_().
    playInternal_() {
      const song = this.currentSong_;
      if (!song) return;

      // Get an absolute URL since that's what we'll get from the <audio>
      // element: https://stackoverflow.com/a/44547904
      const url = getSongUrl(song.filename);
      if (this.audio_.src !== url || this.reachedEndOfSongs_) {
        console.log(`Starting ${song.songId} (${url})`);
        this.audio_.src = url;
        this.audio_.currentTime = 0;

        this.startTime_ = getCurrentTimeSec();
        this.reportedCurrentTrack_ = false;
        this.reachedEndOfSongs_ = false;
        this.lastTimeUpdatePos_ = 0;
        this.updateGain_();
      }

      console.log('Playing');
      this.audio_.play().catch((e) => {
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
    pause_() {
      console.log('Pausing');
      this.audio_.pause();
    }

    togglePause_() {
      this.audio_.paused ? this.play_(false /* delay */) : this.pause_();
    }

    seek_(seconds: number) {
      if (!this.audio_.seekable) return;

      const newTime = Math.max(this.audio_.currentTime + seconds, 0);
      if (newTime < this.audio_.duration) this.audio_.currentTime = newTime;
    }

    onEnded_ = () => {
      this.cancelTimeUpdateTimeout_();
      if (this.currentIndex_ >= this.songs_.length - 1) {
        this.reachedEndOfSongs_ = true;
      } else {
        this.cycleTrack_(1, false /* delay */);
      }
    };

    onPause_ = () => {
      this.cancelTimeUpdateTimeout_();
      this.playPauseButton_.innerText = '▶';
      this.playPauseButton_.title = 'Play (Space)';
    };

    onPlay_ = () => {
      this.playPauseButton_.innerText = '⏸';
      this.playPauseButton_.title = 'Pause (Space)';
    };

    onTimeUpdate_ = () => {
      const song = this.currentSong_;
      if (song === null) return;

      const pos = this.audio_.currentTime;
      const played = this.audio_.playtime;
      const dur = song.length;

      if (!this.reportedCurrentTrack_ && (played >= 240 || played > dur / 2)) {
        this.updater_.reportPlay(song.songId, this.startTime_);
        this.reportedCurrentTrack_ = true;
      }

      // Only update the displayed time while the document is visible.
      if (!document.hidden) {
        const str = dur
          ? `${formatDuration(pos)} / ${formatDuration(dur)}`
          : '';
        if (this.timeDiv_.innerText !== str) this.timeDiv_.innerText = str;

        this.presentationLayer_.updatePosition(pos);
      }

      // Preload the next song once we're nearing the end of this one.
      if (
        pos >= dur - MusicPlayer.PRELOAD_SEC_ &&
        this.nextSong_ &&
        !this.audio_.preloadSrc
      ) {
        const url = getSongUrl(this.nextSong_.filename);
        console.log(`Preloading ${this.nextSong_.songId} (${url})`);
        this.audio_.preloadSrc = url;
      }

      // Schedule a fake update for just after when we expect the playback
      // position to cross the next second boundary. Only do this when we're
      // actually making progress, though.
      if (
        !this.audio_.paused &&
        pos > this.lastTimeUpdatePos_ &&
        this.timeUpdateTimeoutId_ === null
      ) {
        const nextMs = 1000 * (Math.floor(pos + 1) - pos);
        this.timeUpdateTimeoutId_ = window.setTimeout(() => {
          this.timeUpdateTimeoutId_ = null;
          this.onTimeUpdate_();
        }, nextMs + MusicPlayer.TIME_UPDATE_SLOP_MS_);
      }

      this.lastTimeUpdatePos_ = pos;
    };

    onError_ = () => {
      this.cycleTrack_(1, false /* delay */);
    };

    cancelTimeUpdateTimeout_() {
      if (this.timeUpdateTimeoutId_ === null) return;
      window.clearTimeout(this.timeUpdateTimeoutId_);
      this.timeUpdateTimeoutId_ = null;
    }

    showUpdateDialog_() {
      const song = this.currentSong_;
      if (this.updateDialog_ || !song) return;
      this.updateDialog_ = new UpdateDialog(
        song,
        this.tags_,
        (rating, tags) => {
          this.updateDialog_ = null;

          if (rating === null && tags === null) return;

          this.updater_.rateAndTag(song.songId, rating, tags);

          if (rating !== null) {
            song.rating = rating;
            this.updateRatingOverlay_();
          }
          if (tags !== null) {
            song.tags = tags;
            const created = tags.filter((t) => !this.tags_.includes(t));
            if (created.length > 0) {
              this.dispatchEvent(
                new CustomEvent('newtags', { detail: { tags: created } })
              );
            }
          }
          this.updateCoverTitleAttribute_();
        }
      );
    }

    // Shows or hides the presentation layer.
    setPresentationLayerVisible_(visible: boolean) {
      if (this.presentationLayer_.visible === visible) return;
      this.presentationLayer_.visible = visible;
      this.dispatchEvent(new CustomEvent('present', { detail: { visible } }));
    }

    // Adjusts |audio|'s gain appropriately for the current song and settings.
    // This implements the approach described at
    // https://wiki.hydrogenaud.io/index.php?title=ReplayGain_specification.
    updateGain_() {
      let adj = this.config_.get(Config.PRE_AMP); // decibels

      const song = this.currentSong_;
      if (song) {
        let gainType = this.config_.get(Config.GAIN_TYPE);
        if (gainType === Config.GAIN_AUTO) {
          gainType = this.shuffled_ ? Config.GAIN_TRACK : Config.GAIN_ALBUM;
        }

        if (gainType === Config.GAIN_ALBUM) {
          adj += song.albumGain ?? 0;
        } else if (gainType === Config.GAIN_TRACK) {
          adj += song.trackGain ?? 0;
        }
      }

      let scale = 10 ** (adj / 20);

      // TODO: Add an option to prevent clipping instead of always doing this?
      if (song?.peakAmp) scale = Math.min(scale, 1 / song.peakAmp);

      console.log(`Scaling amplitude by ${scale.toFixed(3)}`);
      this.audio_.gain = scale;
    }
  }
);
