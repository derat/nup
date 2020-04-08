// Copyright 2015 Daniel Erat.
// All rights reserved.

class OptionsDialog {
  constructor(config, container) {
    this.config = config;
    this.container = container;
    this.container.insertAdjacentHTML(
      'afterbegin',
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
        '</div>',
    );

    this.closeCallback = null;

    this.volumeRange = $('volumeRange');
    this.volumeSpan = $('volumeSpan');

    const volume = this.config.getVolume();
    this.volumeRange.value = volume;
    this.updateVolumeSpan(volume);

    this.volumeRange.addEventListener(
      'input',
      this.handleVolumeRangeInput.bind(this),
      false,
    );
    $('optionsOkButton').addEventListener(
      'click',
      this.handleOkButtonClick.bind(this),
      false,
    );
    this.keyListener = this.handleBodyKeyDown.bind(this);
    document.body.addEventListener('keydown', this.keyListener, false);
  }

  getContainer() {
    return this.container;
  }

  setCloseCallback(cb) {
    this.closeCallback = cb;
  }

  updateVolumeSpan(volume) {
    this.volumeSpan.innerText = parseInt(volume * 100) + '%';
  }

  handleVolumeRangeInput(e) {
    const volume = this.volumeRange.value;
    this.config.setVolume(volume);
    this.updateVolumeSpan(volume);
  }

  handleOkButtonClick(e) {
    this.close();
  }

  handleBodyKeyDown(e) {
    if (this.processAccelerator(e)) {
      e.preventDefault();
      e.stopPropagation();
    }
  }

  processAccelerator(e) {
    if (e.keyCode == KeyCodes.ESCAPE) {
      this.close();
      return true;
    }

    return false;
  }

  close() {
    document.body.removeEventListener('keydown', this.keyListener, false);
    this.config.save();
    if (this.closeCallback) this.closeCallback();
  }
}
