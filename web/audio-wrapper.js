// Copyright 2022 Daniel Erat.
// All rights reserved.

import { createShadow, createTemplate, getCurrentTimeSec } from './common.js';

const template = createTemplate(`
<audio type="audio/mpeg" preload="auto">
  Your browser doesn't support the audio element.
</audio>
`);

// <audio-wrapper> wraps the <audio> element to hide some of its complexity.
//
// It transparently forwards a subset of <audio>'s properties, events and
// methods (but not HTML attributes), with the following changes:
//
// - The |preloadSrc| property can be set to asynchronously prepare the next
//   file for playback.
// - The |volume| property can be set to values above 1 to amplify audio.
// - If the |src| property is set to a falsey value, the <audio> is paused and
//   its |src| attribute is removed.
customElements.define(
  'audio-wrapper',
  class extends HTMLElement {
    static GAIN_CHANGE_SEC_ = 0.1; // duration for audio gain changes
    static RESUME_WHEN_ONLINE_SEC_ = 30; // maximum delay for auto-resume when online

    constructor() {
      super();

      this.audioCtx_ = new AudioContext();
      this.gainNode_ = this.audioCtx_.createGain();
      this.gainNode_.connect(this.audioCtx_.destination);
      this.gain_ = 1;

      this.shadow_ = createShadow(this, template);
      this.audio_ = this.shadow_.querySelector('audio');
      this.configureAudio_();
      this.preloadAudio_ = null;

      this.lastUpdateSrc_ = null;
      this.lastUpdatePos_ = null;
      this.pausedForOfflineTime_ = null; // seconds since epoch when auto-paused
      this.numErrors_ = 0; // consecutive playback errors

      // Automatically resume playing if we previously paused due to going
      // offline: https://github.com/derat/nup/issues/17
      window.addEventListener('online', (e) => this.onOnline_(e));
    }

    // Adds event handlers to |audio_| and routes it through |gainNode_|.
    configureAudio_() {
      for (const t of ['ended', 'pause', 'play']) {
        this.audio_.addEventListener(t, (e) => this.resendAudioEvent_(e));
      }

      this.audio_.addEventListener('error', (e) => this.onError_(e));
      this.audio_.addEventListener('timeupdate', (e) => this.onTimeUpdate_(e));

      this.audioSrc_ = this.audioCtx_.createMediaElementSource(this.audio_);
      this.audioSrc_.connect(this.gainNode_);
    }

    onOnline_(e) {
      if (this.pausedForOfflineTime_ === null) return;

      const elapsed = getCurrentTimeSec() - this.pausedForOfflineTime_;
      const resume = elapsed <= this.constructor.RESUME_WHEN_ONLINE_SEC_;

      console.log('Back online');
      this.pausedForOfflineTime_ = null;
      this.reloadAudio_();
      if (resume) this.audio_.play();
    }

    onError_(e) {
      if (e.target !== this.audio_) return;

      this.numErrors_++;

      const error = e.target.error;
      console.log(`Got playback error ${error.code} (${error.message})`);
      switch (error.code) {
        case error.MEDIA_ERR_ABORTED: // 1
          break;
        case error.MEDIA_ERR_NETWORK: // 2
        case error.MEDIA_ERR_DECODE: // 3
        case error.MEDIA_ERR_SRC_NOT_SUPPORTED: // 4
          if (!navigator.onLine) {
            console.log('Currently offline; pausing');
            this.audio_.pause();
            this.pausedForOfflineTime_ = getCurrentTimeSec();
          } else if (this.numErrors_ <= this.constructor.MAX_RETRIES_) {
            console.log(`Retrying from position ${this.lastUpdatePos_}`);
            this.reloadAudio_();
            this.audio_.play();
          } else {
            console.log(`Giving up after ${this.numErrors_} errors`);
            this.resendAudioEvent_(e);
          }
          break;
      }
    }

    onTimeUpdate_(e) {
      if (e.target !== this.audio_) return;

      const src = this.audio_.src;
      const pos = this.audio_.currentTime;
      if (src === this.lastUpdateSrc_ && pos === this.lastUpdatePos_) return;

      this.lastUpdateSrc_ = src;
      this.lastUpdatePos_ = pos;
      this.numErrors_ = 0;

      this.resendAudioEvent_(e);
    }

    resendAudioEvent_(e) {
      const ne = new Event(e.type);
      Object.defineProperty(ne, 'target', { get: () => e.target });
      this.dispatchEvent(ne);
    }

    // Reinitializes |audio_|. This is sometimes needed to get it back into a
    // playable state after a network error -- otherwise, play() triggers a 'The
    // element has no supported sources.' error.
    reloadAudio_() {
      this.audio_.load();
      this.audio_.currentTime = this.lastUpdatePos_;
    }

    get src() {
      return this.audio_.src;
    }
    set src(src) {
      this.numErrors_ = 0;

      if (src === this.audio_.src) return;

      // Deal with "The AudioContext was not allowed to start. It must be
      // resumed (or created) after a user gesture on the page.":
      // https://developers.google.com/web/updates/2017/09/autoplay-policy-changes#webaudio
      const ctx = this.gainNode_.context;
      if (ctx.state === 'suspended') ctx.resume();

      if (!src) {
        this.audio_.pause();
        this.audio_.removeAttribute('src');
      } else if (this.preloadSrc === src && !this.preloadAudio_.error) {
        this.audioSrc_.disconnect(this.gainNode_);
        this.audio_.removeAttribute('src');
        this.audio_.parentNode.replaceChild(this.preloadAudio_, this.audio_);
        this.audio_ = this.preloadAudio_;
        this.configureAudio_(); // resets |audioSrc_|
      } else {
        this.audio_.src = src;
      }
      this.preloadAudio_ = null;
    }

    get currentTime() {
      return this.audio_.currentTime;
    }
    set currentTime(t) {
      this.audio_.currentTime = t;
    }

    get duration() {
      return this.audio_.duration;
    }
    get paused() {
      return this.audio_.paused;
    }
    get seekable() {
      return this.audio_.seekable;
    }

    play() {
      return this.audio_.play();
    }
    pause() {
      this.audio_.pause();
    }
    load() {
      this.audio_.load();
    }

    get volume() {
      return this.gain_;
    }
    set volume(v) {
      // Per https://developer.mozilla.org/en-US/docs/Web/API/GainNode:
      // "If modified, the new gain is instantly applied, causing unaesthetic
      // 'clicks' in the resulting audio. To prevent this from happening, never
      // change the value directly but use the exponential interpolation methods
      // on the AudioParam interface."
      this.gainNode_.gain.exponentialRampToValueAtTime(
        v, // TODO: Prevent 0 from being passed.
        this.audioCtx_.currentTime + this.constructor.GAIN_CHANGE_SEC_
      );
      this.gain_ = v;
    }

    get preloadSrc() {
      return this.preloadAudio_ ? this.preloadAudio_.src : null;
    }
    set preloadSrc(src) {
      if (this.preloadAudio_ && this.preloadAudio_.src === src) return;
      this.preloadAudio_ = this.audio_.cloneNode(true);
      this.preloadAudio_.src = src;
    }
  }
);
