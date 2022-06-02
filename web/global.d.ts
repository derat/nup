// Copyright 2022 Daniel Erat.
// All rights reserved.

// Corresponds to Song in server/db/song.go.
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

// These seem to be present in tsc 4.7.2 but not in
// bullseye's node-typescript 4.1.3-1 package.
// https://github.com/Microsoft/TypeScript/issues/28502
// https://github.com/microsoft/TypeScript-DOM-lib-generator/blob/main/baselines/dom.generated.d.ts
interface ResizeObserver {
  observe(target: Element): void;
}
declare var ResizeObserver: {
  prototype: ResizeObserver;
  new (callback: ResizeObserverCallback): ResizeObserver;
};
interface ResizeObserverCallback {
  (entries: ResizeObserverEntry[], observer: ResizeObserver): void;
}
interface ResizeObserverEntry {}

// These seem to be present in tsc 4.7.2 but not in
// bullseye's node-typescript 4.1.3-1 package.
// https://github.com/Microsoft/TypeScript/issues/19473
// https://github.com/microsoft/TypeScript-DOM-lib-generator/blob/main/baselines/dom.generated.d.ts
interface Navigator {
  readonly mediaSession: MediaSession;
}
interface MediaSession {
  metadata: MediaMetadata | null;
  setActionHandler(
    // @ts-ignore: "type MediaSessionAction" conflicts if already defined.
    action: MediaSessionAction,
    handler: MediaSessionActionHandler | null
  ): void;
}
interface MediaMetadata {
  album: string;
  artist: string;
  artwork: ReadonlyArray<MediaImage>;
  title: string;
}
declare var MediaMetadata: {
  prototype: MediaMetadata;
  new (init?: MediaMetadataInit): MediaMetadata;
};
interface MediaMetadataInit {
  album?: string;
  artist?: string;
  artwork?: MediaImage[];
  title?: string;
}
interface MediaImage {
  sizes?: string;
  src: string;
  type?: string;
}
interface MediaSessionActionHandler {
  (details: MediaSessionActionDetails): void;
}
interface MediaSessionActionDetails {
  // @ts-ignore: "type MediaSessionAction" conflicts if already defined.
  action: MediaSessionAction;
  fastSeek?: boolean | null;
  seekOffset?: number | null;
  seekTime?: number | null;
}
