// Copyright 2022 Daniel Erat.
// All rights reserved.

// Corresponds to Song in server/db/song.go.
declare interface Song {
  sha1: string;
  songId: string;
  filename: string;
  coverFilename?: string;
  artist: string;
  title: string;
  album: string;
  albumArtist?: string;
  albumId?: string;
  track: number;
  disc: number;
  date?: string;
  length: number;
  trackGain: number;
  albumGain: number;
  peakAmp: number;
  rating: number;
  tags: string[];
}

// Corresponds to SearchPreset in server/config/config.go.
declare interface SearchPreset {
  name: string;
  tags: string;
  minRating: number;
  unrated: boolean;
  firstPlayed: number;
  lastPlayed: number;
  orderByLastPlayed: boolean;
  maxPlays: number;
  firstTrack: boolean;
  shuffle: boolean;
  play: boolean;
}

// The remainder of this file contains hokey subsets of DOM stuff that's either
// new enough or not standardized enough that tsc doesn't include it yet.

// https://github.com/microsoft/TypeScript-DOM-lib-generator/issues/897
declare interface CSSStyleSheet {
  replaceSync(text: string): void;
}
declare interface DocumentOrShadowRoot {
  adoptedStyleSheets: CSSStyleSheet[];
}
