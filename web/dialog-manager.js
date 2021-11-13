// Copyright 2011 Daniel Erat.
// All rights reserved.

import { $, createElement, createShadow, createTemplate } from './common.js';

const template = createTemplate(`
<style>
  @import 'common.css';

  :host {
    pointer-events: none;
  }
  .lightbox {
    background-color: #000;
    display: none;
    height: 100%;
    opacity: 0;
    pointer-events: auto;
    position: fixed;
    width: 100%;
    z-index: 10;
  }
  .lightbox.shown {
    display: block;
  }
  .lightbox.dimming {
    opacity: 0.1;
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
    background-color: var(--bg-color);
    border: solid 1px var(--frame-border-color);
    box-shadow: 0 2px 6px 2px rgba(0, 0, 0, 0.1);
    display: inline-block;
    padding: var(--margin);
    pointer-events: auto;
    text-align: left;
  }
  .message-dialog {
    width: 400px;
  }
  .menu {
    background-color: var(--bg-color);
    border: solid 1px var(--frame-border-color);
    box-shadow: 0 1px 2px 1px rgba(0, 0, 0, 0.2);
    pointer-events: auto;
    position: absolute;
    text-align: left;
  }
  .menu .item {
    cursor: default;
    padding: 6px 12px;
  }
  .menu .item:hover {
    background-color: var(--menu-hover-color);
  }
  .menu .item:first-child {
    padding-top: 8px;
  }
  .menu .item:last-child {
    padding-bottom: 8px;
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

// <dialog-manager> manages dialog windows and context menus. It dims the rest
// of the page behind dialogs and blocks mouse events.
customElements.define(
  'dialog-manager',
  class extends HTMLElement {
    constructor() {
      super();

      this.shadow_ = createShadow(this, template);
      this.lightbox_ = $('lightbox', this.shadow_);
      this.lightbox_.addEventListener(
        'click',
        (e) => {
          // Close all menus if the lightbox receives a click.
          this.menus_.forEach((m) => this.closeChild(m));
          e.stopPropagation();
        },
        false
      );
      this.container_ = $('container', this.shadow_);

      // Close all menus if the Escape key is pressed.
      document.body.addEventListener('keydown', (e) => {
        if (e.key == 'Escape') this.menus_.forEach((m) => this.closeChild(m));
      });
    }

    get numChildren() {
      return this.container_.children.length;
    }

    get menus_() {
      return Array.from(this.container_.children).filter((c) =>
        c.classList.contains('menu')
      );
    }

    // Creates, displays, and returns an empty dialog window container. The
    // caller should add content within the returned container and is
    // responsible for closing the dialog by passing it to closeChild().
    createDialog() {
      const dialog = createElement('span', 'dialog', this.container_);
      this.updateLightbox_();
      return dialog;
    }

    // Creates and displays a dialog window already containing the supplied
    // title and text.
    createMessageDialog(titleText, messageText) {
      const dialog = this.createDialog();
      dialog.addEventListener('keydown', (e) => {
        if (e.key == 'Escape') {
          e.preventDefault();
          this.closeChild(dialog);
        }
      });

      dialog.classList.add('message-dialog');
      const shadow = dialog.attachShadow({ mode: 'open' });
      shadow.appendChild(messageDialogTemplate.content.cloneNode(true));
      $('title', shadow).innerText = titleText;
      $('message', shadow).innerText = messageText;
      const button = $('ok-button', shadow);
      button.addEventListener('click', () => this.closeChild(dialog), false);
      button.focus();
    }

    // Creates and displays a simple context menu at the specified location.
    // |items| is an array of objects with 'text' properties containing the menu
    // item text and 'cb' properties containing the corresponding callback.
    createMenu(x, y, items) {
      const menu = createElement('span', 'menu', this.container_);
      menu.addEventListener('click', (e) => {
        this.closeChild(menu);
      });
      this.updateLightbox_();

      for (const item of items) {
        const el = createElement('div', 'item', menu, item.text);
        el.id = item.id;
        el.addEventListener('click', (e) => item.cb());
      }

      // Keep the menu onscreen.
      menu.style.left =
        x + menu.clientWidth <= window.innerWidth
          ? `${x}px`
          : `${x - menu.clientWidth}px`;
      menu.style.top =
        y + menu.clientHeight <= window.innerHeight
          ? `${y}px`
          : `${y - menu.clientHeight}px`;

      return menu;
    }

    // Closes |child| (either a dialog or a menu).
    // A 'close' event is dispatched to |child|.
    closeChild(child) {
      child.dispatchEvent(new Event('close'));
      this.container_.removeChild(child);
      this.updateLightbox_();
    }

    // Updates the lightbox's visibility and opacity.
    updateLightbox_() {
      if (this.numChildren > 0) {
        this.lightbox_.classList.add('shown');
        if (this.numChildren > this.menus_.length) {
          this.lightbox_.classList.add('dimming');
        } else {
          this.lightbox_.classList.remove('dimming');
        }
      } else {
        this.lightbox_.classList.remove('shown');
      }
    }
  }
);
