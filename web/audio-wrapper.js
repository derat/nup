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
// - |gain| can be set to adjust the audio's gain. Valid values must be greater
//   than 0, but can also exceed 1 to amplify the signal (unlike |volume|).
// - |playtime| contains the total playtime of |src| so far in seconds.
// - |preloadSrc| can be set to asynchronously prepare a file for playback.
// - |src| can be set to a falsey value to pause the <audio> element and remove
//   its |src| attribute.
// - After errors, playback is retried several times before an 'error' event is
//   emitted. The <audio> element is paused while offline and automatically
//   resumed if the network connection comes back soon afterward.
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

      this.lastUpdateTime_ = null; // time at last 'timeupdate' or 'play' event
      this.lastUpdatePos_ = 0; // position at last 'timeupdate' event
      this.playtime_ = 0; // total playtime of |src| in seconds
      this.pausedForOfflineTime_ = null; // seconds since epoch when auto-paused
      this.numErrors_ = 0; // consecutive playback errors

      window.addEventListener('online', (e) => this.onOnline_(e));
    }

    // Adds event handlers to |audio_| and routes it through |gainNode_|.
    configureAudio_() {
      this.audio_.addEventListener('ended', (e) => this.resendAudioEvent_(e));
      this.audio_.addEventListener('error', (e) => this.onError_(e));
      this.audio_.addEventListener('pause', (e) => this.onPause_(e));
      this.audio_.addEventListener('play', (e) => this.onPlay_(e));
      this.audio_.addEventListener('timeupdate', (e) => this.onTimeUpdate_(e));

      this.audioSrc_ = this.audioCtx_.createMediaElementSource(this.audio_);
      this.audioSrc_.connect(this.gainNode_);
    }

    onOnline_(e) {
      // Automatically resume playing if we previously paused due to going
      // offline: https://github.com/derat/nup/issues/17
      if (this.pausedForOfflineTime_ !== null) {
        console.log('Back online');
        const elapsed = getCurrentTimeSec() - this.pausedForOfflineTime_;
        const resume = elapsed <= this.constructor.RESUME_WHEN_ONLINE_SEC_;
        this.pausedForOfflineTime_ = null;
        this.reloadAudio_();
        if (resume) this.audio_.play();
      }
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
            console.log('Offline; pausing');
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

    onPause_(e) {
      this.lastUpdateTime_ = null;
      this.resendAudioEvent_(e);
    }

    onPlay_(e) {
      this.lastUpdateTime_ = getCurrentTimeSec();
      this.resendAudioEvent_(e);
    }

    onTimeUpdate_(e) {
      if (e.target !== this.audio_) return;

      const pos = this.audio_.currentTime;
      if (pos === this.lastUpdatePos_) return;

      const now = getCurrentTimeSec();
      if (this.lastUpdateTime_ !== null) {
        // Playback can hang if the network is flaky, so make sure that we don't
        // incorrectly increment the playtime by the wall time if the position
        // didn't move as much: https://github.com/derat/nup/issues/20
        const timeDiff = now - this.lastUpdateTime_;
        const posDiff = pos - this.lastUpdatePos_;
        this.playtime_ += Math.max(Math.min(timeDiff, posDiff), 0);
      }

      this.lastUpdateTime_ = now;
      this.lastUpdatePos_ = pos;
      this.numErrors_ = 0;

      this.resendAudioEvent_(e);
    }

    // Dispatches a new event based on |e|.
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

      this.lastUpdateTime_ = null;
      this.lastUpdatePos_ = 0;
      this.playtime_ = 0;
      this.pausedForOfflineTime_ = null;
      this.numErrors_ = 0;

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

    get gain() {
      return this.gain_;
    }
    set gain(v) {
      // Per https://developer.mozilla.org/en-US/docs/Web/API/GainNode:
      // "If modified, the new gain is instantly applied, causing unaesthetic
      // 'clicks' in the resulting audio. To prevent this from happening, never
      // change the value directly but use the exponential interpolation methods
      // on the AudioParam interface."
      this.gainNode_.gain.exponentialRampToValueAtTime(
        v,
        this.audioCtx_.currentTime + this.constructor.GAIN_CHANGE_SEC_
      );
      this.gain_ = v;
    }

    get playtime() {
      return this.playtime_;
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
