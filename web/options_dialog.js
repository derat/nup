// Copyright 2015 Daniel Erat.
// All rights reserved.

function OptionsDialog(config, container) {
  this.config = config;
  this.container = container;
  this.container.insertAdjacentHTML('afterbegin',
      '<div id="optionsDiv">' +
      '  <div class="title">Options</div>' +
      '  <hr class="title">' +
      '  <p id="volumeRow">' +
      '    <label for="volumeRange">Volume:</label>' +
      '    <input id="volumeRange" type="range" min="0.0" max="1.0" step="0.1">' +
      '    <span id="volumeSpan">100%</span>' +
      '  </p>' +
      '  <div class="buttonContainer">' +
      '   <input id="optionsOkButton" type="button" value="OK">' +
      '  </div>' +
      '</div>');

  this.closeCallback = null;

  this.volumeRange = $('volumeRange');
  this.volumeSpan = $('volumeSpan');

  var volume = this.config.getVolume();
  this.volumeRange.value = volume;
  this.updateVolumeSpan(volume);

  this.volumeRange.addEventListener('input', this.handleVolumeRangeInput.bind(this), false);
  $('optionsOkButton').addEventListener('click', this.handleOkButtonClick.bind(this), false);
  this.keyListener = this.handleBodyKeyDown.bind(this);
  document.body.addEventListener('keydown', this.keyListener, false);
};

OptionsDialog.prototype.getContainer = function() {
  return this.container;
};

OptionsDialog.prototype.setCloseCallback = function(cb) {
  this.closeCallback = cb;
};

OptionsDialog.prototype.updateVolumeSpan = function(volume) {
  this.volumeSpan.innerText = parseInt(volume * 100) + '%';
};

OptionsDialog.prototype.handleVolumeRangeInput = function(e) {
  var volume = this.volumeRange.value;
  this.config.setVolume(volume);
  this.updateVolumeSpan(volume);
};

OptionsDialog.prototype.handleOkButtonClick = function(e) {
  this.close();
};

OptionsDialog.prototype.handleBodyKeyDown = function(e) {
  if (this.processAccelerator(e)) {
    e.preventDefault();
    e.stopPropagation();
  }
};

OptionsDialog.prototype.processAccelerator = function(e) {
  if (e.keyCode == KeyCodes.ESCAPE) {
    this.close();
    return true;
  }

  return false;
};

OptionsDialog.prototype.close = function() {
  document.body.removeEventListener('keydown', this.keyListener, false);
  this.config.save();
  if (this.closeCallback)
    this.closeCallback();
};
