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
  .menu hr {
    background-color: var(--border-color);
    border: 0;
    height: 1px;
    margin: 4px 0;
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

// <overlay-manager> manages dialog windows and context menus. It dims the rest
// of the page behind dialogs and blocks mouse events.
customElements.define(
  'overlay-manager',
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
      // If there are no menus, close the first dialog (if any).
      document.body.addEventListener('keydown', (e) => {
        if (e.key == 'Escape') {
          if (this.menus_.length) {
            this.menus_.forEach((m) => this.closeChild(m));
            e.stopPropagation();
          } else if (this.numChildren) {
            this.closeChild(this.container_.children.item(0));
            e.stopPropagation();
          }
        }
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

    // Creates, displays, and returns an empty dialog window container,
    // additionally dimming the rest of the screen.
    //
    // The caller should add content within the returned container and can close
    // it by passing it to closeChild(). Escape keypresses will also close the
    // dialog. The caller should listen for a 'close' event on the container if
    // any actions need to be performed when the dialog is closed.
    createDialog() {
      const dialog = createElement('span', 'dialog', this.container_);
      this.updateLightbox_();
      return dialog;
    }

    // Creates and displays a dialog window already containing the supplied
    // title and text.
    createMessageDialog(titleText, messageText) {
      const dialog = this.createDialog();
      dialog.classList.add('message-dialog');

      const shadow = dialog.attachShadow({ mode: 'open' });
      shadow.appendChild(messageDialogTemplate.content.cloneNode(true));
      $('title', shadow).innerText = titleText;
      $('message', shadow).innerText = messageText;

      const button = $('ok-button', shadow);
      button.addEventListener('click', () => this.closeChild(dialog));
      button.focus();
    }

    // Creates and displays a simple context menu at the specified location.
    //
    // |items| is an array of objects with the following properties:
    // - text - menu item text, or '-' to insert separator instead
    // - cb   - callback to run when clicked
    // - id   - optional ID for element (used in tests)
    createMenu(x, y, items) {
      const menu = createElement('span', 'menu', this.container_);
      this.updateLightbox_();

      for (const item of items) {
        if (item.text === '-') {
          createElement('hr', null, menu, null);
        } else {
          const el = createElement('div', 'item', menu, item.text);
          if (item.id) el.id = item.id;
          el.addEventListener('click', (e) => {
            e.stopPropagation();
            this.closeChild(menu);
            item.cb();
          });
        }
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
