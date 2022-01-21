// Copyright 2015 Daniel Erat.
// All rights reserved.

import { $, createShadow, createTemplate } from './common.js';
import Config from './config.js';

const template = createTemplate(`
<style>
  @import 'common.css';
  @import 'dialog.css';

  hr.title {
    margin-bottom: var(--margin);
  }
  .row {
    align-items: center;
    display: flex;
    margin-bottom: var(--margin);
  }
  .label-col {
    display: inline-block;
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
</style>

<div class="title">Options</div>
<hr class="title" />

<div class="row">
  <label for="theme-select">
    <span class="label-col">Theme</span>
    <div class="select-wrapper">
      <select id="theme-select">
        <option value="0">Auto</option>
        <option value="1">Light</option>
        <option value="2">Dark</option>
      </select>
    </div>
  </label>
</div>

<div class="row">
  <label for="gain-type-select">
    <span class="label-col">Gain</span>
    <div class="select-wrapper">
      <select id="gain-type-select">
        <option value="0">Album</option>
        <option value="1">Track</option>
        <option value="2">None</option>
      </select>
    </div>
  </label>
</div>

<div class="row">
  <label for="pre-amp-range">
    <span class="label-col">Pre-amp</span>
    <input id="pre-amp-range" type="range" min="-10" max="10" step="1" />
    <span id="pre-amp-span"></span>
  </label>
</div>

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

    this.themeSelect_ = $('theme-select', this.shadow_);
    this.themeSelect_.value = this.config_.get(Config.THEME);
    this.themeSelect_.addEventListener('change', () =>
      this.config_.set(Config.THEME, this.themeSelect_.value)
    );

    this.gainTypeSelect_ = $('gain-type-select', this.shadow_);
    this.gainTypeSelect_.value = this.config_.get(Config.GAIN_TYPE);
    this.gainTypeSelect_.addEventListener('change', () =>
      this.config_.set(Config.GAIN_TYPE, this.gainTypeSelect_.value)
    );

    const preAmp = this.config_.get(Config.PRE_AMP);
    this.preAmpRange_ = $('pre-amp-range', this.shadow_);
    this.preAmpRange_.value = preAmp;
    this.preAmpRange_.addEventListener('input', () =>
      this.updatePreAmpSpan_(this.preAmpRange_.value)
    );
    this.preAmpRange_.addEventListener('change', () =>
      this.config_.set(Config.PRE_AMP, this.preAmpRange_.value)
    );

    this.preAmpSpan_ = $('pre-amp-span', this.shadow_);
    this.updatePreAmpSpan_(preAmp);

    $('ok-button', this.shadow_).addEventListener('click', () =>
      this.manager_.closeChild(this.container_)
    );

    this.container_.addEventListener('close', () => {
      this.config_.save();
      if (this.closeCallback_) this.closeCallback_();
    });
  }

  updatePreAmpSpan_(preAmp) {
    const prefix = preAmp > 0 ? '+' : '';
    this.preAmpSpan_.innerText = `${prefix}${preAmp} dB`;
  }
}
