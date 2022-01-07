# `nup` executable

This directory contains a `nup` command-line program that interacts with the
[App Engine server]. The program supports various subcommands that are
described below.

[App Engine server]: ../../server

```
Usage: nup <flags> <subcommand> <subcommand args>

Subcommands:
        check            check for issues in songs and cover images
        commands         list all command names
        config           manage server configuration
        covers           download album art
        dump             dump songs from the server
        flags            describe all known top-level flags
        gain             compute gain adjustments for songs
        help             describe subcommands and their syntax
        projectid        print GCP project ID
        storage          update song storage classes
        update           send song updates to the server

  -config string
        Path to config file (default "$HOME/.nup/config.json")
```

## `check` command

The `check` command checks for issues in JSON-marshaled [Song] objects dumped by
the `dump` command or in cover images.

[Song]: ../../server/db/song.go

```
check [flags]:
        Check for issues in dumped songs from stdin.

  -check string
        Comma-separated list of checks to perform:
          album-id        Songs have MusicBrainz album IDs
          cover-size-400  Cover images are at least 400x400
          imported        All songs have been imported
          song-cover      Songs have cover files
         (default "album-id,imported,song-cover")
```

## `config` command

The `config` command prints or updates the server's saved configuration in
[Datastore]. See the [Config](../../server/config/config.go) struct for details.

```
config [flags]:
        Manage the App Engine server's configuration in Datastore.
        By default, prints the existing JSON-marshaled configuration.

  -set string
        Path of updated JSON config file to save to Datastore
```

[Datastore]: https://cloud.google.com/datastore

## `covers` command

The `covers` command reads JSON-marshaled [Song] objects written by the `dump`
command and downloads the corresponding album artwork from the [Cover Art
Archive].

Google Images is also convenient for finding album artwork. A custom search for
high-resolution square images can be added to Chrome by going to
`chrome://settings/searchEngines?search=search+engines` and entering the
following information:

*   Search engine: `Google Image Search Album Art`
*   Keyword: `album`
*   URL: `https://www.google.com/search?as_st=y&tbm=isch&hl=en&as_q=%s&as_epq=&as_oq=&as_eq=&cr=&as_sitesearch=&safe=images&tbs=isz:l,iar:s`

[Cover Art Archive]: https://coverartarchive.org/

```
covers [flags]:
        Download album art from coverartarchive.org for dumped songs from
        stdin. Image files are written to the directory specified via
        -cover-dir.

  -cover-dir string
        Directory to write covers to
  -max-requests int
        Maximum number of parallel HTTP requests (default 2)
  -max-songs int
        Maximum number of songs to inspect (default -1)
```

## `dump` command

The `dump` command downloads all song metadata and user data from the [App
Engine server] and writes JSON-marshaled [Song] objects to stdout.

```
dump [flags]:
        Dump JSON-marshaled song data from the server to stdout.

  -covers
        Include cover filenames
  -play-batch-size int
        Size for each batch of entities (default 800)
  -song-batch-size int
        Size for each batch of entities (default 400)
```

## `gain` command

The `gain` command uses the [mp3gain] program to compute gain adjustments and
peak amplitudes for dumped songs.

The `update` command computes the same data when importing songs. `gain` is
only useful for backfilling missing data.

[mp3gain]: http://mp3gain.sourceforge.net/

```
gain:
        Compute gain adjustments for dumped songs from stdin using mp3gain.
        Writes updated JSON song objects to stdout.
```

## `projectid` command

The `projectid` command prints the GCP [project ID].

[project ID]: https://cloud.google.com/resource-manager/docs/creating-managing-projects#before_you_begin

```
projectid:
        Print the Google Cloud Platform project ID (as derived from the
        config's serverURL field).
```

## `storage` command

The `storage` command reads JSON-marshaled [Song] objects written by the `dump`
command and updates each song file's [storage class] in [Google Cloud Storage]
based on its rating.

Check the current [Cloud Storage pricing], but for single-user use, it's
probably most economical to just set the song bucket's default storage class to
Coldline and use that for all songs.

[Google Cloud Storage]: https://cloud.google.com/storage
[storage class]: https://cloud.google.com/storage/docs/storage-classes
[Cloud Storage pricing]: https://cloud.google.com/storage/pricing

```
storage [flags]:
        Update song files' storage classes in Google Cloud Storage based on
        ratings in dumped songs from stdin.

  -bucket string
        Google Cloud Storage bucket containing songs
  -class string
        Storage class for infrequently-accessed files (default "COLDLINE")
  -max-updates int
        Maximum number of files to update (default -1)
  -rating-cutoff float
        Minimum song rating for standard storage class (default 0.75)
  -workers int
        Maximum concurrent Google Cloud Storage updates (default 10)
```

## `update` command

The `update` command updates the [App Engine server]'s song data.

By default, `update` scans the music directory from the config file and sends
metadata for all song files that have been added or modified since the previous
run. The `-force-glob` and `-song-paths-file` flags can be used to read specific
song files instead of scanning all files for changes.

The `-import-json-file` flag can be used to instead read JSON-marshaled [Song]
objects from a file. Note that any existing user data (ratings, tags, and
playback history) will be replaced by default, although this behavior can be
disabled by passing `-import-user-data=false`.

The `-delete-song-id` flag can be used to delete specific songs from the server
(e.g. after deleting them from the music dir).

```
update [flags]:
        Send song updates to the server.

  -delete-song-id int
        Delete song with given ID
  -dry-run
        Only print what would be updated
  -dumped-gains-file string
        Path to dump file from which songs' gains will be read (instead of being computed)
  -force-glob string
        Glob pattern relative to music dir for files to scan and update even if they haven't changed
  -import-json-file string
        If non-empty, path to JSON file containing a stream of Song objects to import
  -import-user-data
        When importing from JSON, replace user data (ratings, tags, plays, etc.) (default true)
  -limit int
        If positive, limits the number of songs to update (for testing)
  -require-covers
        Die if cover images aren't found for any songs that have album IDs
  -song-paths-file string
        Path to a file containing one relative path per line for songs to force updating
  -test-gain-info string
        Hardcoded gain info as "track:album:amp" (for testing)
```

### Merging songs

Suppose you have an existing file `old/song.mp3` that's already been rated,
tagged, and played. You just added a new file `new/song.mp3` that contains a
better (e.g. higher-bitrate) version of the same song from a different album.
You want to delete the old file and merge its rating, tags, and play history
into the new file.

1.  Run `nup update` to create a new database object for the new file.
2.  Use `nup dump` to produce a local text file containing JSON representations
    of all songs. Let's call it `music.txt`. This will contain objects for both
    the old and new files.
3.  Optionally, find the old file's line in `music.txt` and copy it into a new
    `delete.txt` file. You can keep this as a backup in case something goes
    wrong.
4.  Find the new file's line in `music.txt` and copy it into a new `update.txt`
    file.
5.  Replace the `rating`, `plays`, and `tags` properties in `update.txt` with
    the old file's values.
6.  Run a command like the following to update the new file's database object:
    ```sh
    nup update -import-user-data -import-json-file update.txt
    ```
    You might want to run this with `-dry-run` first to check `update.txt`'s
    syntax and double-check what will be done.
7.  Run a command like the following to delete the old file's database object:
    ```sh
    nup update -delete-song-id <ID>
    ```
    `ID` corresponds to the numeric value of the old file's `songId` property.
8.  Delete `old/song.mp3` or remove it from your local music directory.

You can put multiple updated objects in `update.txt`, with each on its own line.
If you want to delete multiple objects from a `delete.txt` file, use a command
like the following:
```sh
sed -nre 's/.*"songId":"([0-9]+)".*/\1/p' <delete.txt | while read id; do
  nup update -delete-song-id $id
done
```
