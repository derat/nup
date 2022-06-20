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

const GAIN_CHANGE_SEC = 0.03; // duration for audio gain changes
const MAX_RETRIES = 2; // number of consecutive playback errors to retry
const PAUSE_GAIN = 0.001; // target audio gain when pausing
const RESUME_WHEN_ONLINE_SEC = 30; // maximum delay for auto-resume when online

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
export class AudioWrapper extends HTMLElement {
  #audioCtx = new AudioContext();
  #gainNode = this.#audioCtx.createGain();
  #audioSrc: MediaElementAudioSourceNode | null = null;
  #gain = 1;

  #shadow = createShadow(this, template);
  #audio = this.#shadow.querySelector('audio') as HTMLAudioElement;
  #preloadAudio: HTMLAudioElement | null = null;

  #lastUpdateTime: number | null = null; // last 'timeupdate' or 'play' event
  #lastUpdatePos = 0; // position at last 'timeupdate' event
  #playtime = 0; // total playtime of |src| in seconds
  #pauseTimeoutId: number | null = null; // #audio.pause() after gain lower
  #pausedForOfflineTime: number | null = null; // time when auto-paused
  #numErrors = 0; // consecutive playback errors

  constructor() {
    super();
    this.#gainNode.connect(this.#audioCtx.destination);
    this.#configureAudio();
  }

  connectedCallback() {
    window.addEventListener('online', this.#onOnline);
  }

  disconnectedCallback() {
    window.removeEventListener('online', this.#onOnline);

    if (this.#pauseTimeoutId) {
      window.clearTimeout(this.#pauseTimeoutId);
      this.#pauseTimeoutId = null;
    }
  }

  // Adds event handlers to #audio and recreates #audioSrc to route
  // #audio's output through #gainNode.
  #configureAudio() {
    this.#audio.addEventListener('ended', this.#onEnded);
    this.#audio.addEventListener('error', this.#onError);
    this.#audio.addEventListener('pause', this.#onPause);
    this.#audio.addEventListener('play', this.#onPlay);
    this.#audio.addEventListener('timeupdate', this.#onTimeUpdate);

    this.#audioSrc = this.#audioCtx.createMediaElementSource(this.#audio);
    this.#audioSrc.connect(this.#gainNode);
  }

  // Deconfigures #audio and replaces it with |audio|.
  #replaceAudio(audio: HTMLAudioElement) {
    this.#audio.removeAttribute('src');
    this.#audio.removeEventListener('ended', this.#onEnded);
    this.#audio.removeEventListener('error', this.#onError);
    this.#audio.removeEventListener('pause', this.#onPause);
    this.#audio.removeEventListener('play', this.#onPlay);
    this.#audio.removeEventListener('timeupdate', this.#onTimeUpdate);

    this.#audioSrc?.disconnect();
    this.#audioSrc = null;

    this.#audio.parentNode!.replaceChild(audio, this.#audio);
    this.#audio = audio;
    this.#configureAudio();
  }

  #onOnline = () => {
    // Automatically resume playing if we previously paused due to going
    // offline: https://github.com/derat/nup/issues/17
    if (this.#pausedForOfflineTime !== null) {
      console.log('Back online');
      const elapsed = getCurrentTimeSec() - this.#pausedForOfflineTime;
      const resume = elapsed <= RESUME_WHEN_ONLINE_SEC;
      this.#pausedForOfflineTime = null;
      this.#reloadAudio();
      if (resume) this.#audio.play();
    }
  };

  #onEnded = (e: Event) => {
    this.#resendAudioEvent(e);
  };

  #onError = (e: Event) => {
    if (e.target !== this.#audio) return;

    this.#numErrors++;

    const error = this.#audio.error!;
    console.log(`Got playback error ${error.code} (${error.message})`);
    switch (error.code) {
      case error.MEDIA_ERR_ABORTED: // 1
        break;
      case error.MEDIA_ERR_NETWORK: // 2
      case error.MEDIA_ERR_DECODE: // 3
      case error.MEDIA_ERR_SRC_NOT_SUPPORTED: // 4
        if (!navigator.onLine) {
          console.log('Offline; pausing');
          this.#audio.pause();
          this.#pausedForOfflineTime = getCurrentTimeSec();
        } else if (this.#numErrors <= MAX_RETRIES) {
          console.log(`Retrying from position ${this.#lastUpdatePos}`);
          this.#reloadAudio();
          this.#audio.play();
        } else {
          console.log(`Giving up after ${this.#numErrors} errors`);
          this.#resendAudioEvent(e);
        }
        break;
    }
  };

  #onPause = (e: Event) => {
    this.#lastUpdateTime = null;
    this.#resendAudioEvent(e);
  };

  #onPlay = (e: Event) => {
    this.#lastUpdateTime = getCurrentTimeSec();
    this.#resendAudioEvent(e);
  };

  #onTimeUpdate = (e: Event) => {
    if (e.target !== this.#audio) return;

    const pos = this.#audio.currentTime;
    if (pos === this.#lastUpdatePos) return;

    const now = getCurrentTimeSec();
    if (this.#lastUpdateTime !== null) {
      // Playback can hang if the network is flaky, so make sure that we don't
      // incorrectly increment the playtime by the wall time if the position
      // didn't move as much: https://github.com/derat/nup/issues/20
      const timeDiff = now - this.#lastUpdateTime;
      const posDiff = pos - this.#lastUpdatePos;
      this.#playtime += clamp(timeDiff, 0, posDiff);
    }

    this.#lastUpdateTime = now;
    this.#lastUpdatePos = pos;
    this.#numErrors = 0;

    this.#resendAudioEvent(e);
  };

  // Dispatches a new event based on |e|.
  #resendAudioEvent(e: Event) {
    const ne = new Event(e.type);
    Object.defineProperty(ne, 'target', { value: e.target });
    this.dispatchEvent(ne);
  }

  // Reinitializes #audio. This is sometimes needed to get it back into a
  // playable state after a network error -- otherwise, play() triggers a 'The
  // element has no supported sources.' error.
  #reloadAudio() {
    this.#audio.load();
    this.#audio.currentTime = this.#lastUpdatePos;
  }

  // Sets #gainNode's gain to |v|.
  #setAudioGain(v: number) {
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
    const g = this.#gainNode.gain;
    const t = this.#audioCtx.currentTime;
    g.setValueAtTime(g.value, t);
    g.exponentialRampToValueAtTime(v, t + GAIN_CHANGE_SEC);
  }

  // Cancels #pauseTimeoutId if non-null.
  #cancelPauseTimeout() {
    if (this.#pauseTimeoutId === null) return;
    window.clearTimeout(this.#pauseTimeoutId);
    this.#pauseTimeoutId = null;
  }

  get src() {
    return this.#audio.src ?? null;
  }
  set src(src: string | null) {
    // Deal with "The AudioContext was not allowed to start. It must be
    // resumed (or created) after a user gesture on the page.":
    // https://developers.google.com/web/updates/2017/09/autoplay-policy-changes#webaudio
    const ctx = this.#gainNode.context as AudioContext;
    if (ctx.state === 'suspended') ctx.resume();

    // Throw out the preload element if it encountered an error.
    if (this.#preloadAudio?.error) {
      const error = this.#preloadAudio.error;
      console.log(
        `Preload error ${error.code} (${error.message}) for ${this.preloadSrc}`
      );
      this.#preloadAudio = null;
    }

    if (!src) {
      this.#audio.pause();
      this.#audio.removeAttribute('src');
    } else if (this.preloadSrc === src) {
      this.#replaceAudio(this.#preloadAudio!);
      this.#preloadAudio = null;
    } else {
      this.#audio.src = src;
    }

    this.currentTime = 0;
    this.#lastUpdateTime = null;
    this.#lastUpdatePos = 0;
    this.#playtime = 0;
    this.#pausedForOfflineTime = null;
    this.#numErrors = 0;

    this.#cancelPauseTimeout();
  }

  // Sigh: https://github.com/prettier/prettier/issues/5287
  /* prettier-ignore */ get currentTime() { return this.#audio.currentTime; }
  /* prettier-ignore */ set currentTime(t: number) { this.#audio.currentTime = t; }
  /* prettier-ignore */ get duration() { return this.#audio.duration; }
  /* prettier-ignore */ get paused() { return this.#audio.paused; }
  /* prettier-ignore */ get seekable() { return this.#audio.seekable; }
  /* prettier-ignore */ get playtime() { return this.#playtime; }

  play() {
    this.#cancelPauseTimeout();
    this.#setAudioGain(this.#gain); // restore pre-pause gain
    return this.#audio.play();
  }

  pause() {
    if (this.#pauseTimeoutId !== null) return;

    // Avoid pops caused by abruptly stopping playback:
    // https://github.com/derat/nup/issues/34
    this.#setAudioGain(PAUSE_GAIN);
    this.#pauseTimeoutId = window.setTimeout(() => {
      this.#pauseTimeoutId = null;
      this.#audio.pause();
    }, GAIN_CHANGE_SEC * 1000);
  }

  load() {
    this.#audio.load();
  }

  // prettier-ignore
  get gain() { return this.#gain; }
  set gain(v: number) {
    this.#setAudioGain(v);
    this.#gain = v;
  }

  // TODO: Maybe try to implement proper gapless playback?
  // http://dalecurtis.github.io/llama-demo/index.html provides some guidance
  // about doing this using MSE, although it seems pretty complicated and the
  // current approach here seems to work reasonably well most of the time.
  get preloadSrc() {
    return this.#preloadAudio?.src ?? null;
  }
  set preloadSrc(src: string | null) {
    if (src === null) {
      this.#preloadAudio = null;
      return;
    }

    if (this.#preloadAudio?.src === src) return;

    // It seems like the <audio> element needs to be attached to the document
    // before setting its 'src' attribute; otherwise the element gets error code
    // 4: "MEDIA_ELEMENT_ERROR: Media load rejected by URL safety check". I
    // suspect that it's caused by the document being missing in this code:
    // https://github.com/chromium/chromium/blob/41b188b8fa60155466e96829e3462f46e1ad6405/third_party/blink/renderer/core/html/media/html_media_element.cc#L1727
    const el = template.content.firstElementChild!.cloneNode(true);
    this.#preloadAudio = el as HTMLAudioElement;
    this.#shadow.appendChild(this.#preloadAudio);
    this.#preloadAudio.src = src;
  }
}

customElements.define('audio-wrapper', AudioWrapper);
