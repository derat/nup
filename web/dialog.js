// Copyright 2022 Daniel Erat.
// All rights reserved.

import {
  $,
  commonStyles,
  createElement,
  createShadow,
  createTemplate,
} from './common.js';

const dialogStyle = createTemplate(`
<style>
  :host {
    display: inline-block; /* let width be set */
    text-align: left;
  }
  div.title {
    color: var(--dialog-title-color);
    font-size: 18px;
    font-weight: bold;
    overflow: hidden;
    text-overflow: ellipsis;
    user-select: none;
    white-space: nowrap;
  }
  hr.title {
    background-color: var(--border-color);
    border: 0;
    height: 1px;
    margin: 4px 0;
  }
  div.button-container {
    margin-top: 4px;
    text-align: right;
  }
</style>
`);

const msgTemplate = createTemplate(`
<style>
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
    <button id="ok-button" autofocus>OK</button>
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
  const shadow = createShadow(wrapper, dialogStyle);
  shadow.adoptedStyleSheets = [commonStyles];
  shadow.appendChild(template.content.cloneNode(true));

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
  $('ok-button', shadow).addEventListener('click', () => dialog.close());
}

// Returns true if a dialog is currently shown.
export const isDialogShown = () => numDialogs > 0;
