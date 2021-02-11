// Copyright 2015 Daniel Erat.
// All rights reserved.

import {$, createShadow, createTemplate} from './common.js';
import Config from './config.js';

const template = createTemplate(`
<style>
  @import 'common.css';
  @import 'dialog.css';
  hr.title {
    margin-bottom: 12px;
  }
  .row {
    align-items: center;
    display: flex;
    margin-bottom: 8px;
  }
  .row .label-col {
    width: 6em;
  }
  #pre-amp-range {
    margin-right: 6px;
    vertical-align: middle;
  }
  #pre-amp-span {
    display: inline-block;
    width: 3em;
  }
  .button-container {
    margin-top: 12px;
  }
</style>
<div class="title">Options</div>
<hr class="title">
<label for="gain-type-select" class="row select-wrapper">
  <span class="label-col">Gain</span>
  <select id="gain-type-select">
    <option value="0">Album</option>
    <option value="1">Track</option>
    <option value="2">None</option>
  </select>
</label>
<label for="pre-amp-range" class="row">
  <span class="label-col">Pre-amp</span>
  <input id="pre-amp-range" type="range" min="-10" max="10" step="1">
  <span id="pre-amp-span"></span>
</label>
<div class="button-container">
  <button id="ok-button">OK</button>
</div>
`);

export default class OptionsDialog {
  constructor(config, manager, closeCallback) {
    this.config_ = config;
    this.manager_ = manager;
    this.closeCallback_ = closeCallback;


    this.container_ = this.manager_.createDialog();
    this.shadow_ = createShadow(this.container_, template);

    this.gainTypeSelect_ = $('gain-type-select', this.shadow_);
    this.gainTypeSelect_.value = this.config_.get(Config.GAIN_TYPE);
    this.gainTypeSelect_.addEventListener(
      'change',
      () => this.config_.set(Config.GAIN_TYPE, this.gainTypeSelect_.value),
    );

    const preAmp = this.config_.get(Config.PRE_AMP);
    this.preAmpRange_ = $('pre-amp-range', this.shadow_);
    this.preAmpRange_.value = preAmp;
    this.preAmpRange_.addEventListener(
      'input',
      () => this.updatePreAmpSpan_(this.preAmpRange_.value),
    );
    this.preAmpRange_.addEventListener(
      'change',
      () => this.config_.set(Config.PRE_AMP, this.preAmpRange_.value),
    );

    this.preAmpSpan_ = $('pre-amp-span', this.shadow_);
    this.updatePreAmpSpan_(preAmp);

    $('ok-button', this.shadow_).addEventListener(
      'click',
      () => this.close(),
      false,
    );

    this.keyListener_ = e => {
      if (e.key == 'Escape') {
        e.preventDefault();
        e.stopPropagation();
        this.close();
      }
    };
    document.body.addEventListener('keydown', this.keyListener_, false);
  }

  updatePreAmpSpan_(preAmp) {
    const prefix = preAmp > 0 ? '+' : '';
    this.preAmpSpan_.innerText = `${prefix}${preAmp} dB`;
  }

  close() {
    document.body.removeEventListener('keydown', this.keyListener_, false);
    this.config_.save();
    this.manager_.closeDialog(this.container_);
    if (this.closeCallback_) this.closeCallback_();
  }
}
