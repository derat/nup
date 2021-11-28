# nup

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
album artwork) to [Google Cloud Storage] and then run the `update_music` command
against the local copy to save metadata to a [Datastore] database.
User-generated information like ratings, tags, and playback history is also
saved in Datastore. The App Engine app performs queries against Datastore and
serves songs and album art from Cloud Storage.

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
*   Compile the `update_music` tool, create a small config file for it, and use
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

First, from the base directory, run the `./dev.sh` script. This starts a local
development App Engine instance listening at `http://localhost:8080`.

*   End-to-end Go tests that exercise the App Engine server and the `dump_music`
    and `update_music` commands can be run from the `test/e2e/` directory via
    `go test`.
*   Selenium tests that exercise both the web client and the server can be run
    from the `test/web/` directory via `./tests.py`. The `run_tests_in_xvfb.sh`
    script can be used to run test headlessly using [Xvfb].
*   For development, you can import example data and start a file server by
    running `./import_to_devserver.sh` in the `test/example/` directory. Use
    `test@example.com` to log in.

Go unit tests can be executed by running the following from the root of this
repository:

```sh
go test ./...
```

[Xvfb]: https://en.wikipedia.org/wiki/Xvfb

## Merging songs

Suppose you have an existing file `old/song.mp3` that's already been rated,
tagged, and played. You just added a new file `new/song.mp3` that contains a
better (e.g. higher-bitrate) version of the same song from a different album.
You want to delete the old file and merge its rating, tags, and play history
into the new file.

1.  Run the `update_music` command to create a new database object for the new
    file.
2.  Use the `dump_music` command to produce a local text file containing JSON
    representations of all songs. Let's call it `music.txt`. This will contain
    objects for both the old and new files.
3.  Optionally, find the old file's line in `music.txt` and copy it into a new
    `delete.txt` file. You can keep this as a backup in case something goes
    wrong.
4.  Find the new file's line in `music.txt` and copy it into a new `update.txt`
    file.
5.  Replace the `rating`, `plays`, and `tags` properties in `update.txt` with
    the old file's values.
6.  Run a command like the following to update the new file's database object:
    ```sh
    update_music -config $HOME/.nup/update_music.json \
      -import-user-data -import-json-file update.txt
    ```
    You might want to run this with `-dry-run` first to check `update.txt`'s
    syntax and double-check what will be done.
7.  Run a command like the following to delete the old file's database object:
    ```sh
    update_music -config $HOME/.nup/update_music.json -delete-song-id <ID>
    ```
    `ID` corresponds to the numeric value of the old file's `songId` property.
8.  Delete `old/song.mp3` or remove it from your local music directory.


You can put multiple updated objects in `update.txt`, with each on its own line.
If you want to delete multiple objects from a `delete.txt` file, use a command
like the following:
```sh
sed -nre 's/.*"songId":"([0-9]+)".*/\1/p' <delete.txt | while read id; do
  update_music -config $HOME/.nup/update_music.json -delete-song-id $id
done
```
