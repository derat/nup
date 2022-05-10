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
    <button class="ok-button" autofocus>OK</button>
  </div>
</form>
`);

// Number of open dialogs.
let numDialogs = 0;

// Creates and shows a modal dialog filled with the supplied template.
// If |className| is supplied, it is added to dialog's shadow root.
// The <dialog> element is returned, and the shadow root can be accessed
// via |dialog.firstChild.shadowRoot|.
export function createDialog(template, className) {
  const dialog = createElement('dialog', 'dialog', document.body);
  dialog.addEventListener('close', () => {
    document.body.removeChild(dialog);
    numDialogs--;
  });

  // It seems like it isn't possible to attach a shadow root directly to
  // <dialog>, so add a wrapper element first.
  const wrapper = createElement('span', className, dialog);
  createShadow(wrapper, template);

  // TODO: Figure out how to get form buttons to be consistently focused
  // automatically. Is the FOUC hack breaking autofocus?
  dialog.showModal();

  numDialogs++;

  return dialog;
}

// Creates and shows a modal dialog with the supplied title and text.
// The dialog is not returned.
export function showMessageDialog(titleText, messageText) {
  const dialog = createDialog(msgTemplate, null);
  const shadow = dialog.firstChild.shadowRoot;
  $('title', shadow).innerText = titleText;
  $('message', shadow).innerText = messageText;
}

// Returns true if a dialog is currently shown.
export const isDialogShown = () => numDialogs > 0;
