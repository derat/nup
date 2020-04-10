// Copyright 2015 Daniel Erat.
// All rights reserved.

import {$, createElement, createShadow, KeyCodes} from './common.js';

export default class OptionsDialog {
  constructor(config, manager, closeCallback) {
    this.config_ = config;
    this.manager_ = manager;
    this.closeCallback_ = closeCallback;

    const volume = this.config_.getVolume();

    this.container_ = this.manager_.createDialog();
    this.shadow_ = createShadow(this.container_, 'options-dialog.css');
    createElement('div', 'title', this.shadow_, 'Options');
    createElement('hr', 'title', this.shadow_);

    const p = createElement('p', undefined, this.shadow_);
    createElement('label', undefined, p, 'Volume:').for = 'volumeRange';
    this.volumeRange_ = createElement('input', 'volume', p);
    this.volumeRange_.type = 'range';
    this.volumeRange_.min = '0.0';
    this.volumeRange_.max = '1.0';
    this.volumeRange_.step = '0.1';
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
    this.volumeSpan_ = createElement('span', 'volume', p, '100%');
    this.updateVolumeSpan_(volume);

    const cont = createElement('div', 'button-container', this.shadow_);
    createElement('button', undefined, cont, 'OK').addEventListener(
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
