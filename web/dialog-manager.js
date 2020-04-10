// Copyright 2011 Daniel Erat.
// All rights reserved.

import {createElement, createShadow} from './common.js';

class DialogManager extends HTMLElement {
  constructor() {
    super();

    // TODO: Is there a better way to do this?
    this.style.cssText = `
      left: 0;
      height: 100%;
      pointer-events: none;
      position: fixed;
      top: 0;
      width: 100%;
      z-index: 10;
    `;

    this.shadow_ = createShadow(this, 'dialog-manager.css');
    this.lightbox_ = createElement('div', 'lightbox', this.shadow_);
    this.lightbox_.addEventListener('click', e => e.stopPropagation(), false);
    const outer = createElement('div', 'outer-container', this.shadow_);
    this.container_ = createElement('div', 'inner-container', outer);
  }

  getNumDialogs() {
    return this.container_.children.length;
  }

  createDialog() {
    const dialog = createElement('span', 'dialog', this.container_);
    if (this.getNumDialogs() == 1) this.lightbox_.classList.add('shown');
    return dialog;
  }

  closeDialog(dialog) {
    this.container_.removeChild(dialog);
    if (!this.getNumDialogs()) this.lightbox_.classList.remove('shown');
  }

  createMessageDialog(titleText, messageText) {
    const dialog = this.createDialog();
    dialog.addEventListener(
      'keydown',
      e => {
        if (e.keyCode == 27) {
          // escape
          e.preventDefault();
          this.closeDialog(dialog);
        }
      },
      false,
    );

    dialog.classList.add('message-dialog');
    const shadow = createShadow(dialog, 'message-dialog.css');
    createElement('div', 'title', shadow, titleText);
    createElement('hr', 'title', shadow);
    createElement('div', 'message', shadow, messageText);
    const cont = createElement('div', 'button-container', shadow);
    const button = createElement('button', undefined, cont, 'OK');
    button.addEventListener('click', () => this.closeDialog(dialog), false);
    button.focus();
  }
}

customElements.define('dialog-manager', DialogManager);
