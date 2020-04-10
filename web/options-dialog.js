// Copyright 2015 Daniel Erat.
// All rights reserved.

import {
  $,
  createElement,
  createShadow,
  createTemplate,
  KeyCodes,
} from './common.js';

const template = createTemplate(`
<style>
  @import 'dialog.css';
  #volumeRange {
    vertical-align: middle;
  }
  #volumeSpan {
    display: inline-block;
    width: 3em;
  }
</style>
<div class="title">Options</div>
<hr class="title" />
<p>
  <label for="volumeRange">Volume:</label>
  <input id="volumeRange" type="range" min="0.0" max="1.0" step="0.1" />
  <span id="volumeSpan"></span>
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

    const volume = this.config_.getVolume();

    this.container_ = this.manager_.createDialog();
    this.shadow_ = this.container_.attachShadow({mode: 'open'});
    this.shadow_.appendChild(template.content.cloneNode(true));

    this.volumeRange_ = $('volumeRange', this.shadow_);
    this.volumeRange_.value = volume;
    this.volumeRange_.addEventListener(
      'input',
      () => {
        const volume = this.volumeRange_.value;
        this.config_.setVolume(volume);
        this.updateVolumeSpan_(volume);
      },
      false,
    );

    this.volumeSpan_ = $('volumeSpan', this.shadow_);
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
