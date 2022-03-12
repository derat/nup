// Copyright 2022 Daniel Erat.
// All rights reserved.

import { createShadow, createTemplate } from './common.js';

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
    }

    // Adds event handlers to |audio_| and routes it through |gainNode_|.
    configureAudio_() {
      for (const t of ['ended', 'pause', 'play', 'timeupdate']) {
        this.audio_.addEventListener(t, (e) =>
          this.dispatchEvent(new CustomEvent(t))
        );
      }
      this.audio_.addEventListener('error', (e) => {
        const ne = new CustomEvent('error');
        ne.target = e.target;
        this.dispatchEvent(ne);
      });

      this.audioSrc_ = this.audioCtx_.createMediaElementSource(this.audio_);
      this.audioSrc_.connect(this.gainNode_);
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
