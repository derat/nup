// Copyright 2011 Daniel Erat <dan@erat.org>
// All rights reserved.

function DialogManager() {
  this.listeners = [];
  var lightbox = document.createElement('div');
  this.lightbox = lightbox;
  lightbox.className = 'dialogLightbox';
  lightbox.addEventListener('click', this.handleLightboxClick_.bind(this), false);
  document.body.insertBefore(lightbox, document.body.firstChild);

  var outerContainer = document.createElement('div');
  outerContainer.className = 'dialogOuterContainer';
  document.body.insertBefore(outerContainer, lightbox.nextSibling);

  this.innerContainer = createElement('div', 'dialogInnerContainer', outerContainer);
}

DialogManager.prototype.getNumDialogs = function() {
  return this.innerContainer.childNodes.length;
};

// |listener| is invoked with a 'visible' boolean argument when we go from
// no dialogs to one dialog or vice versa.
DialogManager.prototype.registerVisibilityChangeListener = function(listener) {
  this.listeners.push(listener);
};

DialogManager.prototype.createDialog = function() {
  var dialog = createElement('span', 'dialog', this.innerContainer);
  if (this.getNumDialogs() == 1) {
    this.lightbox.style.display = 'block';
    this.listeners.forEach(function(v) { v(true); }, this);
  }
  return dialog;
};

DialogManager.prototype.closeDialog = function(dialog) {
  this.innerContainer.removeChild(dialog);
  if (this.getNumDialogs() == 0) {
    this.lightbox.style.display = 'none';
    this.listeners.forEach(function(v) { v(false); }, this);
  }
};

DialogManager.prototype.createMessageDialog = function(titleText, messageText) {
  var dialog = this.createDialog();
  dialog.addEventListener('keydown', this.handleMessageDialogKeyDown_.bind(this, dialog), false);

  addClassName(dialog, 'messageDialog');
  createElement('div', 'title', dialog, titleText);
  createElement('hr', 'title', dialog);
  createElement('div', 'message', dialog, messageText);
  var container = createElement('div', 'buttonContainer', dialog);
  var button = createElement('input', '', container);
  button.type = 'button';
  button.value = 'OK';
  button.addEventListener('click', this.handleMessageDialogButtonClick_.bind(this, dialog), false);
  button.focus();
};

DialogManager.prototype.handleMessageDialogButtonClick_ = function(dialog, e) {
  this.closeDialog(dialog);
};

DialogManager.prototype.handleMessageDialogKeyDown_ = function(dialog, e) {
  if (e.keyCode == 27) {  // escape
    e.preventDefault();
    this.closeDialog(dialog);
  }
};

DialogManager.prototype.handleLightboxClick_ = function(e) {
  e.stopPropagation();
};
