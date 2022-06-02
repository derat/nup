// Copyright 2022 Daniel Erat.
// All rights reserved.

declare interface Song {
  sha1: string;
  songId: string;
  filename?: string;
  coverFilename?: string;
  artist: string;
  title: string;
  album: string;
  albumArtist?: string;
  albumId?: string;
  track: number;
  disc: number;
  length: number;
  trackGain: number;
  albumGain: number;
  peakAmp: number;
  rating: number;
  tags: string[];
}

declare interface CSSStyleSheet {
  replaceSync(text: string): void;
}

declare interface DocumentOrShadowRoot {
  adoptedStyleSheets: CSSStyleSheet[];
}
