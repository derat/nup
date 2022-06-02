// Copyright 2022 Daniel Erat.
// All rights reserved.

declare interface CSSStyleSheet {
  replaceSync(text: string): void;
}

declare interface DocumentOrShadowRoot {
  adoptedStyleSheets: CSSStyleSheet[];
}
