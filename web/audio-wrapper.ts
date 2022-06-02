// Copyright 2022 Daniel Erat.
// All rights reserved.

import {
  clamp,
  createShadow,
  createTemplate,
  getCurrentTimeSec,
} from './common.js';

const template = createTemplate(`
<audio type="audio/mpeg" preload="auto">
  Your browser doesn't support the audio element.
</audio>
`);

// <audio-wrapper> wraps the <audio> element to hide some of its complexity.
//
// It transparently forwards a subset of <audio>'s properties, events, and
// methods (but not HTML attributes), with the following changes:
//
// - |gain| can be set to adjust the audio's gain. Valid values must be greater
//   than 0, but can also exceed 1 to amplify the signal (unlike |volume|).
// - |playtime| contains the total playtime of |src| so far in seconds.
// - |preloadSrc| can be set to asynchronously prepare a file for playback.
// - |src| can be set to a falsy value to pause the <audio> element and remove
//   its |src| attribute.
// - pause() ramps the gain down before pausing to avoid audible pops.
// - After errors, playback is retried several times before an 'error' event is
//   emitted. The <audio> element is paused while offline and automatically
//   resumed if the network connection comes back soon afterward.
//
// Just to document it somewhere, the way that Chrome requests media files over
// HTTP seems pretty annoying and wasteful, resulting in the server reading and
// sending the same bytes multiple times.
//
// For example, I saw Chrome 100.0.4896.93 do the following when requesting a
// 4434715-byte file:
// - Request "bytes=0-", get a 206, and drop the connection.
// - Request "bytes=3276800-" 55 seconds later to get the rest.
//
// For a 8435194-byte file:
// - Request "bytes=0-" but drop the connection.
// - Request "bytes=3276800-3806618" 55 seconds later and get a 304 response.
// - Request "bytes=3806619-8435193" 130 milliseconds later to get the rest.
//
// I think I've occasionally seen Chrome get the whole file in a single request,
// but it's rare. https://support.google.com/chrome/thread/25510119 complains
// about this too. I didn't find any Chromium issues discussing this when I
// looked in April 2022.
customElements.define(
  'audio-wrapper',
  class AudioWrapper extends HTMLElement {
    static GAIN_CHANGE_SEC_ = 0.03; // duration for audio gain changes
    static MAX_RETRIES_ = 2; // number of consecutive playback errors to retry
    static PAUSE_GAIN_ = 0.001; // target audio gain when pausing
    static RESUME_WHEN_ONLINE_SEC_ = 30; // maximum delay for auto-resume when online

    audioCtx_: AudioContext;
    gainNode_: GainNode;
    gain_: number;
    audioSrc_: MediaElementAudioSourceNode | null;

    shadow_: ShadowRoot;
    audio_: HTMLAudioElement;
    preloadAudio_: HTMLAudioElement | null;

    lastUpdateTime_: number | null; // time at last 'timeupdate' or 'play' event
    lastUpdatePos_: number; // position at last 'timeupdate' event
    playtime_: number; // total playtime of |src| in seconds
    pauseTimeoutId_: number | null; // calls audio_.pause() after dropping gain
    pausedForOfflineTime_: number | null; // seconds since epoch when auto-paused
    numErrors_: number; // consecutive playback errors

    constructor() {
      super();

      this.audioCtx_ = new AudioContext();
      this.gainNode_ = this.audioCtx_.createGain();
      this.gainNode_.connect(this.audioCtx_.destination);
      this.gain_ = 1;
      this.audioSrc_ = null;

      this.shadow_ = createShadow(this, template);
      this.audio_ = this.shadow_.querySelector('audio');
      this.configureAudio_();
      this.preloadAudio_ = null;

      this.lastUpdateTime_ = null;
      this.lastUpdatePos_ = 0;
      this.playtime_ = 0;
      this.pauseTimeoutId_ = null;
      this.pausedForOfflineTime_ = null;
      this.numErrors_ = 0;
    }

    connectedCallback() {
      window.addEventListener('online', this.onOnline_);
    }

    disconnectedCallback() {
      window.removeEventListener('online', this.onOnline_);

      if (this.pauseTimeoutId_) {
        window.clearTimeout(this.pauseTimeoutId_);
        this.pauseTimeoutId_ = null;
      }
    }

    // Adds event handlers to |audio_| and recreates |audioSrc_| to route
    // |audio_|'s output through |gainNode_|.
    configureAudio_() {
      this.audio_.addEventListener('ended', this.onEnded_);
      this.audio_.addEventListener('error', this.onError_);
      this.audio_.addEventListener('pause', this.onPause_);
      this.audio_.addEventListener('play', this.onPlay_);
      this.audio_.addEventListener('timeupdate', this.onTimeUpdate_);

      this.audioSrc_ = this.audioCtx_.createMediaElementSource(this.audio_);
      this.audioSrc_.connect(this.gainNode_);
    }

    // Deconfigures |audio_| and replaces it with |audio|.
    replaceAudio_(audio: HTMLAudioElement) {
      this.audio_.removeAttribute('src');
      this.audio_.removeEventListener('ended', this.onEnded_);
      this.audio_.removeEventListener('error', this.onError_);
      this.audio_.removeEventListener('pause', this.onPause_);
      this.audio_.removeEventListener('play', this.onPlay_);
      this.audio_.removeEventListener('timeupdate', this.onTimeUpdate_);

      this.audioSrc_?.disconnect();
      this.audioSrc_ = null;

      this.audio_.parentNode.replaceChild(audio, this.audio_);
      this.audio_ = audio;
      this.configureAudio_();
    }

    onOnline_ = () => {
      // Automatically resume playing if we previously paused due to going
      // offline: https://github.com/derat/nup/issues/17
      if (this.pausedForOfflineTime_ !== null) {
        console.log('Back online');
        const elapsed = getCurrentTimeSec() - this.pausedForOfflineTime_;
        const resume = elapsed <= AudioWrapper.RESUME_WHEN_ONLINE_SEC_;
        this.pausedForOfflineTime_ = null;
        this.reloadAudio_();
        if (resume) this.audio_.play();
      }
    };

    onEnded_ = (e: Event) => {
      this.resendAudioEvent_(e);
    };

    onError_ = (e: Event) => {
      if (e.target !== this.audio_) return;

      this.numErrors_++;

      const error = (e.target as HTMLAudioElement).error;
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
          } else if (this.numErrors_ <= AudioWrapper.MAX_RETRIES_) {
            console.log(`Retrying from position ${this.lastUpdatePos_}`);
            this.reloadAudio_();
            this.audio_.play();
          } else {
            console.log(`Giving up after ${this.numErrors_} errors`);
            this.resendAudioEvent_(e);
          }
          break;
      }
    };

    onPause_ = (e: Event) => {
      this.lastUpdateTime_ = null;
      this.resendAudioEvent_(e);
    };

    onPlay_ = (e: Event) => {
      this.lastUpdateTime_ = getCurrentTimeSec();
      this.resendAudioEvent_(e);
    };

    onTimeUpdate_ = (e: Event) => {
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
        this.playtime_ += clamp(timeDiff, 0, posDiff);
      }

      this.lastUpdateTime_ = now;
      this.lastUpdatePos_ = pos;
      this.numErrors_ = 0;

      this.resendAudioEvent_(e);
    };

    // Dispatches a new event based on |e|.
    resendAudioEvent_(e: Event) {
      const ne = new Event(e.type);
      Object.defineProperty(ne, 'target', { value: e.target });
      this.dispatchEvent(ne);
    }

    // Reinitializes |audio_|. This is sometimes needed to get it back into a
    // playable state after a network error -- otherwise, play() triggers a 'The
    // element has no supported sources.' error.
    reloadAudio_() {
      this.audio_.load();
      this.audio_.currentTime = this.lastUpdatePos_;
    }

    // Sets |gainNode_|'s gain to |v|.
    setAudioGain_(v: number) {
      // Per https://developer.mozilla.org/en-US/docs/Web/API/GainNode:
      // "If modified, the new gain is instantly applied, causing unaesthetic
      // 'clicks' in the resulting audio. To prevent this from happening, never
      // change the value directly but use the exponential interpolation methods
      // on the AudioParam interface."
      //
      // Note also that the ramp confusingly uses the time of the "last event"
      // as its starting point, so we need to explicitly set the gain again just
      // before starting the ramp to avoid still having an abrupt transition:
      // https://stackoverflow.com/a/34480323
      // https://stackoverflow.com/a/61924161
      // etc.
      const g = this.gainNode_.gain;
      const t = this.audioCtx_.currentTime;
      g.setValueAtTime(g.value, t);
      g.exponentialRampToValueAtTime(v, t + AudioWrapper.GAIN_CHANGE_SEC_);
    }

    // Cancels |pauseTimeoutId_| if non-null.
    cancelPauseTimeout_() {
      if (this.pauseTimeoutId_ === null) return;
      window.clearTimeout(this.pauseTimeoutId_);
      this.pauseTimeoutId_ = null;
    }

    get src() {
      return this.audio_.src;
    }
    set src(src: string) {
      // Deal with "The AudioContext was not allowed to start. It must be
      // resumed (or created) after a user gesture on the page.":
      // https://developers.google.com/web/updates/2017/09/autoplay-policy-changes#webaudio
      const ctx = this.gainNode_.context as AudioContext;
      if (ctx.state === 'suspended') ctx.resume();

      if (!src) {
        this.audio_.pause();
        this.audio_.removeAttribute('src');
      } else if (this.preloadSrc === src && !this.preloadAudio_.error) {
        this.replaceAudio_(this.preloadAudio_);
        this.preloadAudio_ = null;
      } else {
        this.audio_.src = src;
      }

      this.currentTime = 0;
      this.lastUpdateTime_ = null;
      this.lastUpdatePos_ = 0;
      this.playtime_ = 0;
      this.pausedForOfflineTime_ = null;
      this.numErrors_ = 0;

      this.cancelPauseTimeout_();
    }

    // Sigh: https://github.com/prettier/prettier/issues/5287
    /* prettier-ignore */ get currentTime() { return this.audio_.currentTime; }
    /* prettier-ignore */ set currentTime(t: number) { this.audio_.currentTime = t; }
    /* prettier-ignore */ get duration() { return this.audio_.duration; }
    /* prettier-ignore */ get paused() { return this.audio_.paused; }
    /* prettier-ignore */ get seekable() { return this.audio_.seekable; }
    /* prettier-ignore */ get playtime() { return this.playtime_; }

    play() {
      this.cancelPauseTimeout_();
      this.setAudioGain_(this.gain_); // restore pre-pause gain
      return this.audio_.play();
    }

    pause() {
      if (this.pauseTimeoutId_ !== null) return;

      // Avoid pops caused by abruptly stopping playback:
      // https://github.com/derat/nup/issues/34
      this.setAudioGain_(AudioWrapper.PAUSE_GAIN_);
      this.pauseTimeoutId_ = window.setTimeout(() => {
        this.pauseTimeoutId_ = null;
        this.audio_.pause();
      }, AudioWrapper.GAIN_CHANGE_SEC_ * 1000);
    }

    load() {
      this.audio_.load();
    }

    // prettier-ignore
    get gain() { return this.gain_; }
    set gain(v: number) {
      this.setAudioGain_(v);
      this.gain_ = v;
    }

    // TODO: Maybe try to implement proper gapless playback?
    // http://dalecurtis.github.io/llama-demo/index.html provides some guidance
    // about doing this using MSE, although it seems pretty complicated and the
    // current approach here seems to work reasonably well most of the time.
    get preloadSrc() {
      return this.preloadAudio_ ? this.preloadAudio_.src : null;
    }
    set preloadSrc(src: string) {
      if (this.preloadAudio_?.src === src) return;
      // This is split over multiple lines solely to prevent Prettier from
      // doing some of the most hideous formatting that I've ever seen.
      const el = template.content.firstElementChild.cloneNode(true);
      this.preloadAudio_ = el as HTMLAudioElement;
      this.preloadAudio_.src = src;
    }
  }
);
