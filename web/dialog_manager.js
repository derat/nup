// Copyright 2011 Daniel Erat.
// All rights reserved.

class DialogManager {
  constructor() {
    this.listeners = [];
    const lightbox = document.createElement('div');
    this.lightbox = lightbox;
    lightbox.className = 'dialogLightbox';
    lightbox.addEventListener('click', e => e.stopPropagation(), false);
    document.body.insertBefore(lightbox, document.body.firstChild);

    const outerContainer = document.createElement('div');
    outerContainer.className = 'dialogOuterContainer';
    document.body.insertBefore(outerContainer, lightbox.nextSibling);

    this.innerContainer = createElement(
      'div',
      'dialogInnerContainer',
      outerContainer,
    );
  }

  getNumDialogs() {
    return this.innerContainer.children.length;
  }

  // |listener| is invoked with a 'visible' boolean argument when we go from
  // no dialogs to one dialog or vice versa.
  registerVisibilityChangeListener(listener) {
    this.listeners.push(listener);
  }

  createDialog() {
    const dialog = createElement('span', 'dialog', this.innerContainer);
    if (this.getNumDialogs() == 1) {
      this.lightbox.style.display = 'block';
      this.listeners.forEach(v => v(true), this);
    }
    return dialog;
  }

  closeDialog(dialog) {
    this.innerContainer.removeChild(dialog);
    if (this.getNumDialogs() == 0) {
      this.lightbox.style.display = 'none';
      this.listeners.forEach(v => v(false), this);
    }
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

    dialog.classList.add('messageDialog');
    createElement('div', 'title', dialog, titleText);
    createElement('hr', 'title', dialog);
    createElement('div', 'message', dialog, messageText);
    const container = createElement('div', 'buttonContainer', dialog);
    const button = createElement('input', '', container);
    button.type = 'button';
    button.value = 'OK';
    button.addEventListener('click', () => this.closeDialog(dialog), false);
    button.focus();
  }
}
