// Copyright 2011 Daniel Erat.
// All rights reserved.

import {$, createElement, createTemplate} from './common.js';

const template = createTemplate(`
<style>
.lightbox {
  background-color: black;
  display: none;
  height: 100%;
  opacity: 0.1;
  pointer-events: auto;
  position: fixed;
  width: 100%;
  z-index: 10;
}
.lightbox.shown {
  display: block;
}
.outer-container {
  display: table;
  height: 100%;
  pointer-events: none;
  position: absolute;
  width: 100%;
  z-index: 11;
}
.inner-container {
  display: table-cell;
  text-align: center;
  vertical-align: middle;
}
.dialog {
  background-color: white;
  border: solid 1px #aaa;
  box-shadow: 0 2px 6px 2px rgba(0, 0, 0, 0.1);
  -moz-box-shadow: 0 2px 6px 2px rgba(0, 0, 0, 0.1);
  -webkit-box-shadow: 0 2px 6px 2px rgba(0, 0, 0, 0.1);
  display: inline-block;
  padding: 10px;
  pointer-events: auto;
  text-align: left;
}
.message-dialog {
  width: 400px;
}
</style>
<div id="lightbox" class="lightbox"></div>
<div class="outer-container">
  <div id="container" class="inner-container"></div>
</div>
`);

const messageDialogTemplate = createTemplate(`
<style>
  @import 'dialog.css';
  #message {
    line-height: 18px;
    margin-top: 10px;
  }
</style>
<div id="title" class="title"></div>
<hr class="title" />
<div id="message"></div>
<div class="button-container">
  <button id="ok-button">OK</button>
</div>
`);

customElements.define(
  'dialog-manager',
  class extends HTMLElement {
    constructor() {
      super();

      this.style.pointerEvents = 'none';

      this.shadow_ = this.attachShadow({mode: 'open'});
      this.shadow_.appendChild(template.content.cloneNode(true));
      this.lightbox_ = $('lightbox', this.shadow_);
      this.lightbox_.addEventListener('click', e => e.stopPropagation(), false);
      this.container_ = $('container', this.shadow_);
    }

    get numDialogs() {
      return this.container_.children.length;
    }

    createDialog() {
      const dialog = createElement('span', 'dialog', this.container_);
      if (this.numDialogs == 1) this.lightbox_.classList.add('shown');
      return dialog;
    }

    closeDialog(dialog) {
      this.container_.removeChild(dialog);
      if (!this.numDialogs) this.lightbox_.classList.remove('shown');
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
      const shadow = dialog.attachShadow({mode: 'open'});
      shadow.appendChild(messageDialogTemplate.content.cloneNode(true));
      $('title', shadow).innerText = titleText;
      $('message', shadow).innerText = messageText;
      const button = $('ok-button', shadow);
      button.addEventListener('click', () => this.closeDialog(dialog), false);
      button.focus();
    }
  },
);
