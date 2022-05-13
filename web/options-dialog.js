// Copyright 2015 Daniel Erat.
// All rights reserved.

import { $, createShadow, createTemplate } from './common.js';
import Config from './config.js';
import { createDialog } from './dialog.js';

const template = createTemplate(`
<style>
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
        <option value="3">Auto</option>
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

<form method="dialog">
  <div class="button-container">
    <button id="ok-button">OK</button>
  </div>
</form>
`);

// Displays a modal dialog for setting options via |config|.
export function showOptionsDialog(config) {
  const dialog = createDialog(template, 'options');
  const shadow = dialog.firstChild.shadowRoot;
  dialog.addEventListener('close', () => config.save());

  const themeSelect = $('theme-select', shadow);
  themeSelect.value = config.get(Config.THEME);
  themeSelect.addEventListener('change', () =>
    config.set(Config.THEME, themeSelect.value)
  );

  const gainTypeSelect = $('gain-type-select', shadow);
  gainTypeSelect.value = config.get(Config.GAIN_TYPE);
  gainTypeSelect.addEventListener('change', () =>
    config.set(Config.GAIN_TYPE, gainTypeSelect.value)
  );

  const preAmpSpan = $('pre-amp-span', shadow);
  const updatePreAmpSpan = (v) =>
    (preAmpSpan.innerText = `${v > 0 ? '+' : ''}${v} dB`);

  const preAmpValue = config.get(Config.PRE_AMP);
  updatePreAmpSpan(preAmpValue);

  const preAmpRange = $('pre-amp-range', shadow);
  preAmpRange.value = preAmpValue;
  preAmpRange.addEventListener('input', () =>
    updatePreAmpSpan(preAmpRange.value)
  );
  preAmpRange.addEventListener('change', () =>
    config.set(Config.PRE_AMP, preAmpRange.value)
  );
}
