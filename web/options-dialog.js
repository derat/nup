// Copyright 2015 Daniel Erat.
// All rights reserved.

import {$, createShadow, createTemplate, KeyCodes} from './common.js';

const template = createTemplate(`
<style>
  @import 'dialog.css';
  #volume-range {
    vertical-align: middle;
  }
  #volume-span {
    display: inline-block;
    width: 3em;
  }
</style>
<div class="title">Options</div>
<hr class="title" />
<p>
  <label for="volume-range">Volume:</label>
  <input id="volume-range" type="range" min="0.0" max="1.0" step="0.1" />
  <span id="volume-span"></span>
</p>
<div class="button-container">
  <button id="ok-button">OK</button>
</div>
`);

export default class OptionsDialog {
  constructor(config, manager, closeCallback) {
    this.config_ = config;
    this.manager_ = manager;
    this.closeCallback_ = closeCallback;

    const volume = this.config_.get(this.config_.VOLUME);

    this.container_ = this.manager_.createDialog();
    this.shadow_ = createShadow(this.container_, template);

    this.volumeRange_ = $('volume-range', this.shadow_);
    this.volumeRange_.value = volume;
    this.volumeRange_.addEventListener(
      'input',
      () => {
        const volume = this.volumeRange_.value;
        this.config_.set(this.config_.VOLUME, volume);
        this.updateVolumeSpan_(volume);
      },
      false,
    );

    this.volumeSpan_ = $('volume-span', this.shadow_);
    this.updateVolumeSpan_(volume);

    $('ok-button', this.shadow_).addEventListener(
      'click',
      () => this.close(),
      false,
    );

    this.keyListener_ = e => {
      if (e.keyCode == KeyCodes.ESCAPE) {
        e.preventDefault();
        e.stopPropagation();
        this.close();
      }
    };
    document.body.addEventListener('keydown', this.keyListener_, false);
  }

  updateVolumeSpan_(volume) {
    this.volumeSpan_.innerText = parseInt(volume * 100) + '%';
  }

  close() {
    document.body.removeEventListener('keydown', this.keyListener_, false);
    this.config_.save();
    this.manager_.closeDialog(this.container_);
    if (this.closeCallback_) this.closeCallback_();
  }
}
