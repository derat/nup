// Copyright 2015 Daniel Erat.
// All rights reserved.

import { $, createTemplate } from './common.js';
import Config, { getConfig } from './config.js';
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
    <span class="select-wrapper">
      <select id="theme-select">
        <option value="0">Auto</option>
        <option value="1">Light</option>
        <option value="2">Dark</option>
      </select></span
    >
  </label>
</div>

<div class="row">
  <label for="gain-type-select">
    <span class="label-col">Gain</span>
    <span class="select-wrapper">
      <select id="gain-type-select">
        <option value="3">Auto</option>
        <option value="0">Album</option>
        <option value="1">Track</option>
        <option value="2">None</option>
      </select></span
    >
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

// Displays a modal dialog for setting options.
export function showOptionsDialog() {
  const config = getConfig();
  const dialog = createDialog(template, 'options');
  const shadow = dialog.firstElementChild.shadowRoot;
  dialog.addEventListener('close', () => config.save());

  const themeSelect = $('theme-select', shadow) as HTMLSelectElement;
  themeSelect.value = config.get(Config.THEME).toString();
  themeSelect.addEventListener('change', () =>
    config.set(Config.THEME, themeSelect.value)
  );

  const gainTypeSelect = $('gain-type-select', shadow) as HTMLSelectElement;
  gainTypeSelect.value = config.get(Config.GAIN_TYPE).toString();
  gainTypeSelect.addEventListener('change', () =>
    config.set(Config.GAIN_TYPE, gainTypeSelect.value)
  );

  const preAmpSpan = $('pre-amp-span', shadow);
  const updatePreAmpSpan = (v: number) =>
    (preAmpSpan.innerText = `${v > 0 ? '+' : ''}${v} dB`);

  const preAmpValue = config.get(Config.PRE_AMP);
  updatePreAmpSpan(preAmpValue);

  const preAmpRange = $('pre-amp-range', shadow) as HTMLInputElement;
  preAmpRange.value = preAmpValue.toString();
  preAmpRange.addEventListener('input', () =>
    updatePreAmpSpan(parseFloat(preAmpRange.value))
  );
  preAmpRange.addEventListener('change', () =>
    config.set(Config.PRE_AMP, preAmpRange.value)
  );

  $('ok-button', shadow).addEventListener('click', () => dialog.close());
}
