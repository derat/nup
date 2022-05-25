// Copyright 2022 Daniel Erat.
// All rights reserved.

import {
  commonStyles,
  createElement,
  createShadow,
  createTemplate,
} from './common.js';

const menuStyle = createTemplate(`
<style>
  .item {
    cursor: default;
    padding: 6px 12px;
  }
  .item:hover {
    background-color: var(--menu-hover-color);
  }
  .item:first-child {
    padding-top: 8px;
  }
  .item:last-child {
    padding-bottom: 8px;
  }
  .item .hotkey {
    color: var(--text-label-color);
    display: inline-block;
    float: right;
    margin-left: var(--margin);
    text-align: right;
    min-width: 50px;
  }
  hr {
    background-color: var(--border-color);
    border: 0;
    height: 1px;
    margin: 4px 0;
  }
</style>
`);

// Number of open menus.
let numMenus = 0;

// Creates and displays a simple context menu at the specified location.
// Returns a <dialog> element containing a <span> acting as a shadow root.
//
// |items| is an array of objects with the following properties:
// - text   - menu item text, or '-' to insert separator instead
// - cb     - callback to run when clicked
// - id     - optional ID for element (used in tests)
// - hotkey - optional text describing menu's accelerator
export function createMenu(x, y, items, alignRight) {
  const menu = createElement('dialog', 'menu', document.body);
  menu.addEventListener('close', () => {
    document.body.removeChild(menu);
    numMenus--;
  });
  menu.addEventListener('click', (e) => {
    const rect = menu.getBoundingClientRect();
    if (
      e.clientX < rect.left ||
      e.clientX > rect.right ||
      e.clientY < rect.top ||
      e.clientY > rect.bottom
    ) {
      menu.close();
    }
  });

  // It seems like it isn't possible to attach a shadow root directly to
  // <dialog>, so add a wrapper element first.
  const wrapper = createElement('span', null, menu);
  const shadow = createShadow(wrapper, menuStyle);
  shadow.adoptedStyleSheets = [commonStyles];

  const hotkeys = items.some((it) => it.hotkey);
  for (const item of items) {
    if (item.text === '-') {
      createElement('hr', null, shadow);
    } else {
      const el = createElement('div', 'item', shadow, item.text);
      if (item.id) el.id = item.id;
      if (hotkeys) createElement('span', 'hotkey', el, item.hotkey ?? '');
      el.addEventListener('click', (e) => {
        e.stopPropagation();
        menu.close();
        item.cb();
      });
    }
  }

  // For some reason, the menu's clientWidth and clientHeight seem to initially
  // be 0 after switching to <dialog>. Deferring the calculation of the menu's
  // position seems to work around this.
  window.setTimeout(() => {
    if (alignRight) {
      menu.style.right = `${x}px`;
    } else {
      // Keep the menu onscreen.
      menu.style.left =
        x + menu.clientWidth <= window.innerWidth
          ? `${x}px`
          : `${x - menu.clientWidth}px`;
    }
    menu.style.top =
      y + menu.clientHeight <= window.innerHeight
        ? `${y}px`
        : `${y - menu.clientHeight}px`;

    // Only show the menu after it's been positioned.
    menu.classList.add('ready');
  });

  numMenus++;
  menu.showModal();
  return menu;
}

// Returns true if a menu is currently shown.
export const isMenuShown = () => numMenus > 0;
