// Copyright 2015 Daniel Erat.
// All rights reserved.

export default class Config {
  VOLUME_KEY = 'volume';

  constructor() {
    this.listeners = [];

    // Initialize defaults.
    this.volume = 0.7;

    this.load();
  }

  addListener(listener) {
    this.listeners.push(listener);
  }

  getVolume() {
    return this.volume;
  }

  setVolume(volume) {
    this.volume = volume;
    for (let i = 0; i < this.listeners.length; i++) {
      this.listeners[i].onVolumeChange(this.volume);
    }
  }

  load() {
    if (!window.localStorage) return;
    const volume = localStorage[this.VOLUME_KEY];
    if (volume != null) this.volume = volume;
  }

  save() {
    if (!window.localStorage) return;
    localStorage[this.VOLUME_KEY] = this.volume;
  }
}
