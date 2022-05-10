// Copyright 2022 Daniel Erat.
// All rights reserved.

import { $, createElement, createShadow, createTemplate } from './common.js';

const msgTemplate = createTemplate(`
<style>
  @import 'dialog.css';
  :host {
    width: 400px;
  }
  #message {
    line-height: 18px;
    margin-top: 10px;
  }
</style>
<div id="title" class="title"></div>
<hr class="title" />
<div id="message"></div>
<form method="dialog">
  <div class="button-container">
    <button class="ok-button">OK</button>
  </div>
</form>
`);

// Creates and shows a modal dialog filled with the supplied template.
// If |className| is supplied, it is added to dialog's shadow root.
// The <dialog> element is returned, and the shadow root is saved to a
// |shadow| property on it.
export function createDialog(template, className) {
  const dialog = createElement('dialog', 'dialog', document.body);
  dialog.addEventListener('close', () => document.body.removeChild(dialog));

  // It seems like it isn't possible to attach a shadow root directly to
  // <dialog>, so add a wrapper element first.
  const wrapper = createElement('span', className, dialog);
  dialog.shadow = createShadow(wrapper, template);

  // TODO: Figure out how to get form buttons to be consistently focused
  // automatically. Is the FOUC hack breaking autofocus?
  dialog.showModal();

  return dialog;
}

// Creates and shows a modal dialog with the supplied title and text.
// The dialog is not returned.
export function showMessageDialog(titleText, messageText) {
  const dialog = createDialog(msgTemplate, null);
  $('title', dialog.shadow).innerText = titleText;
  $('message', dialog.shadow).innerText = messageText;
}
