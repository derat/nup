// Copyright 2015 Daniel Erat.
// All rights reserved.

function initConfig() {
  document.config = new Config();
}

function Config() {
  this.listeners = [];

  // Initialize defaults.
  this.volume = 0.7;

  this.load();
}

Config.VOLUME_KEY = 'volume';

Config.prototype.addListener = function(listener) {
  this.listeners.push(listener);
};

Config.prototype.getVolume = function() {
  return this.volume;
};

Config.prototype.setVolume = function(volume) {
  this.volume = volume;
  for (var i = 0; i < this.listeners.length; i++) {
    this.listeners[i].onVolumeChange(this.volume);
  }
};

Config.prototype.load = function() {
  if (!window.localStorage) return;

  var volume = localStorage[Config.VOLUME_KEY];
  if (volume != null) this.volume = volume;
};

Config.prototype.save = function() {
  if (!window.localStorage) return;

  localStorage[Config.VOLUME_KEY] = this.volume;
};
