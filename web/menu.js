// Copyright 2022 Daniel Erat.
// All rights reserved.

import { createElement } from './common.js';

// Creates and displays a simple context menu at the specified location.
// Returns a <dialog> element.
//
// |items| is an array of objects with the following properties:
// - text   - menu item text, or '-' to insert separator instead
// - cb     - callback to run when clicked
// - id     - optional ID for element (used in tests)
// - hotkey - optional text describing menu's accelerator
export function createMenu(x, y, items, alignRight) {
  const menu = createElement('dialog', 'menu', document.body);
  menu.addEventListener('close', () => document.body.removeChild(menu));
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

  const hotkeys = items.some((it) => it.hotkey);
  for (const item of items) {
    if (item.text === '-') {
      createElement('hr', null, menu, null);
    } else {
      const el = createElement('div', 'item', menu, item.text);
      if (item.id) el.id = item.id;
      if (hotkeys) createElement('span', 'hotkey', el, item.hotkey || '');
      el.addEventListener('click', (e) => {
        e.stopPropagation();
        menu.close();
        item.cb();
      });
    }
  }

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

  menu.showModal();
  return menu;
}
