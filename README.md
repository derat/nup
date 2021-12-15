# nup

[![Build Status](https://storage.googleapis.com/derat-build-badges/3a131bc1-9d79-498e-b911-dbe5ee293039.svg)](https://storage.googleapis.com/derat-build-badges/3a131bc1-9d79-498e-b911-dbe5ee293039.html)

App Engine app for streaming a music collection.

## Overview

<p float="left">
  <img src="https://user-images.githubusercontent.com/40385/142275393-040006bd-448b-4638-9a3e-c03e191fc5d1.png"
       width="49%" alt="light mode screenshot">
  <img src="https://user-images.githubusercontent.com/40385/142275425-dbe5453f-ad36-4204-afc2-1a99044c0d66.png"
       width="49%" alt="dark mode screenshot">
</p>

This repository contains a [Google App Engine] app for interacting with a
personal music collection, along with a web client and various command-line
tools for managing the data.

The basic idea is that you mirror your music collection (and the corresponding
album artwork) to [Google Cloud Storage] and then run the [update_music]
command against the local copy to save metadata to a [Datastore] database.
User-generated information like ratings, tags, and playback history is also
saved in Datastore. The App Engine app performs queries against Datastore and
serves songs and album art from Cloud Storage.

[update_music]: ./cmd/update_music

An [Android client](https://github.com/derat/nup-android/) is also available.

This project is probably only of potential interest to people who both [buy all
of their music](https://www.erat.org/buying_music.html) and are comfortable
setting up a [Google Cloud] project and compiling and running command-line
programs, which seems like a very small set. If it sounds appealing to you and
you'd like to see more detailed instructions, though, please let me know!

[Google App Engine]: https://cloud.google.com/appengine
[Google Cloud Storage]: https://cloud.google.com/storage
[Datastore]: https://cloud.google.com/datastore
[Google Cloud]: https://cloud.google.com/

## History

In 2001 or 2002, I wrote [dmc], a silly C application that used the [FMOD]
library to play my MP3 collection. It used OpenGL to render a UI and some simple
visualizations. I ran it on a small ([Mini-ITX]? I don't remember) computer
plugged into my TV.

[dmc]: https://www.erat.org/programming.html#older
[FMOD]: https://www.fmod.com/
[Mini-ITX]: https://en.wikipedia.org/wiki/Mini-ITX

Sometime around 2005 or 2006, I decided that I wanted to be able to rate and tag
the songs in my music collection and track my playback history so I could listen
to stuff that I liked but hadn't heard recently, or play non-distracting
instrumental music while reading or programming. I was using [MPD] to play
locally-stored MP3 files at the time, so I wrote some [Ruby] scripts to search
for and enqueue songs and display information about the current song onscreen. I
also wrote a [Ruby audioscrobbler library] for sending playback reports to the
service that later became [Last.fm].

[MPD]: https://www.musicpd.org/
[Ruby]: https://www.ruby-lang.org/
[Ruby audioscrobbler library]: https://www.erat.org/programming.html#audioscrobbler
[Last.fm]: https://www.last.fm/

In 2010, I decided that it was silly to need to have my desktop computer turned
on whenever I wanted to listen to music, so I wrote a daemon in Ruby to serve
music and album art and support searching/tagging/rating/etc. over HTTP. Song
information was stored in a [SQLite] database. I added a web interface and wrote
[an Android client] that supported offline playback, and ran the server on a
little always-on SoC Linux device. This was before the Raspberry Pi was
released, and all I remember about the device was that upgrades were terrifying
because it didn't put out enough power to be able to reliably boot off its
external HDD.

[SQLite]: https://www.sqlite.org/
[an Android client]: https://github.com/derat/nup-android

In 2014, I decided that it'd be nice to be less dependent on my home network
connection, so I rewrote the server in [Go] as a [Google App Engine] app that'd
serve music and covers from [Google Cloud Storage]. That's what this repository
contains.

[Go]: https://golang.org/

It's 2021 now and I haven't felt the urge to rewrite all this code again.

The name "nup" doesn't mean anything; it just didn't seem to be used by any
major projects. (I tried to think of a backronym for it but didn't come up with
anything obvious other than the 'p' standing for "player".)

## Configuration

At the very least, you'll need to do the following:

*   Create a [Google Cloud] project.
*   Enable the Cloud Storage and App Engine APIs.
*   Create Cloud Storage buckets for your songs and album art.
*   Use the [gsutil] tool to sync songs and album art to Cloud Storage.
*   Configure and deploy the App Engine app.
*   Compile the [update_music] tool, create a small config file for it, and use
    it to send song metadata to the App Engine app so it can be saved to
    Datastore.

As mentioned above, please let me know if you're feeling adventurous and would
like to see detailed instructions for these steps.

[gsutil]: https://cloud.google.com/storage/docs/gsutil

Before deploying the App Engine app, create an `config.json` file in the same
directory as `app.yaml` corresponding to the `config` struct in
[server/config.go](./server/config.go):

```json
{
  "projectId": "my-project",
  "googleUsers": [
    "example@gmail.com",
    "example.2@gmail.com",
  ],
  "basicAuthUsers": [
    {
      "username": "myuser",
      "password": "mypass"
    }
  ],
  "songBucket": "my-songs",
  "coverBucket": "my-covers"
}
```

The `projectId` property contains the GCP project ID. It isn't used by the
server; it's just defined here to simplify deployment commands since the
`gcloud` program doesn't support specifying a per-directory default project ID.

All other fields are documented in the Go file linked above.

## Deploying

(The following command depends on the [jq] program being present and `projectId`
being set in `config.json` as described above.)

To deploy the App Engine app and delete old, non-serving versions, run the
`deploy.sh` script.

Note that App Engine often continues serving stale versions of static files for
10 minutes or more after deploying. I think that [this has been broken for a
long time](https://issuetracker.google.com/issues/35890923). [This Stack
Overflow question](https://stackoverflow.com/q/2783082/6882947) has more
discussion.

[jq]: https://stedolan.github.io/jq/

## Development and testing

The [example/](./example) directory contains code for starting a local App
Engine server with example data for development.

All tests can be executed by running `go test ./...` from the root of the
repository.

*   Unit tests live alongside the code that they exercise.
*   End-to-end tests that exercise the App Engine server and the [dump_music]
    and [update_music] commands are in the [test/e2e/](./test/e2e) directory.
*   [Selenium] tests that exercise both the web interface (in Chrome) and the
    server are in the [test/web/](./test/web) directory. By default, Chrome runs
    headlessly using [Xvfb].

[dump_music]: ./cmd/dump_music
[Selenium]: https://www.selenium.dev/
[Xvfb]: https://en.wikipedia.org/wiki/Xvfb
