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
	covers           manage album art
	debug            print information about a song file
	dump             dump songs from the server
	flags            describe all known top-level flags
	help             describe subcommands and their syntax
	projectid        print GCP project ID
	query            run song queries against the server
	scan             scan songs for updated metadata
	storage          update song storage classes
	update           send song updates to the server

  -config string
    	Path to config file (default "/home/danerat/.nup/config.json")
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
    	  cover-size-800  Cover images are at least 800x800
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

  -delete-instances
    	Delete running instances after setting config
  -service string
    	Service name for -delete-instances (default "default")
  -set string
    	Path of updated JSON config file to save to Datastore
```

[Datastore]: https://cloud.google.com/datastore

## `covers` command

The `covers` command manipulates local cover images.

With the `-download` flag, it reads JSON-marshaled [Song] objects written by the
`dump` command and downloads the corresponding album artwork from the [Cover Art
Archive].

Google Images is also convenient for finding album artwork. A custom search for
high-resolution square images can be added to Chrome by going to
`chrome://settings/searchEngines?search=search+engines` and entering the
following information:

*   Search engine: `Google Image Search Album Art`
*   Keyword: `album`
*   URL: `https://www.google.com/search?as_st=y&tbm=isch&hl=en&as_q=%s&as_epq=&as_oq=&as_eq=&cr=&as_sitesearch=&safe=images&tbs=isz:l,iar:s`

[Cover Art Archive]: https://coverartarchive.org/

With the `-generate-webp` flag, it generates smaller [WebP] versions at various
sizes of all of the JPEG images in `-cover-dir`. The server's `/cover` endpoint
returns a WebP image if available when passed the `webp=1` parameter. WebP
images should be generated before syncing the local cover directory to Cloud
Storage.

[WebP]: https://developers.google.com/speed/webp

```
covers [flags]:
	Works with album art images in a directory.
	With -download, downloads album art from coverartarchive.org.
	With -generate-webp, generates WebP versions of existing JPEG images.

  -cover-dir string
    	Directory containing cover images
  -download
    	Download covers for dumped songs read from stdin or positional song files to -cover-dir
  -download-size int
    	Image size to download (250, 500, or 1200) (default 1200)
  -generate-webp
    	Generate WebP versions of covers in -cover-dir
  -max-downloads int
    	Maximum number of songs to inspect for -download (default -1)
  -max-requests int
    	Maximum number of parallel HTTP requests for -download (default 2)
```

## `debug` command

The `debug` command prints information about a song file specified in the
command line.

```
debug [flags] <song-path>...:
	Print information about one or more song files.

  -id3
    	Print all ID3v2 text frames
  -mpeg
    	Read MPEG frames and print size/duration info
```

## `dump` command

The `dump` command downloads all song metadata and user data from the [App
Engine server] and writes JSON-marshaled [Song] objects to stdout.

```
dump [flags]:
	Dump JSON-marshaled song data from the server to stdout.

  -play-batch-size int
    	Size for each batch of entities (default 800)
  -song-batch-size int
    	Size for each batch of entities (default 400)
```

## `projectid` command

The `projectid` command prints the GCP [project ID].

[project ID]: https://cloud.google.com/resource-manager/docs/creating-managing-projects#before_you_begin

```
projectid:
	Print the Google Cloud Platform project ID (as derived from the
	config's serverURL field).
```

## `query` command

The `query` command performs a query using the server and writes the resulting
JSON-marshaled [Song] objects to stdout.

```
query [flags]:
	Query the server and and print JSON-marshaled songs to stdout.

  -filename string
    	Song filename (relative to music dir) to query for
  -path string
    	Song path (resolved to music dir) to query for
  -pretty
    	Pretty-print JSON objects
  -print-id
    	Print song IDs instead of full JSON objects
  -single
    	Fail if exactly one song is not returned
```

## `scan` command

The `scan` command scans the music directory and queries [MusicBrainz] for
updated metadata.

```
scan [flags] <song.mp3>...:
	Scan songs for updated metadata using MusicBrainz.
```

[MusicBrainz]: https://musicbrainz.org/

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
  -rating-cutoff int
    	Minimum song rating for standard storage class (default 4)
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

The `-delete-song` flag can be used to delete specific songs from the server
(e.g. after deleting them from the music dir).

```
update [flags]:
	Send song updates to the server.

  -compare-dump-file string
    	Path to JSON file with songs to compare updates against
  -delete-after-merge
    	Delete source song if -merge-songs is true
  -delete-song int
    	Delete song with given ID
  -dry-run
    	Only print what would be updated
  -dumped-gains-file string
    	Path to dump file from which songs' gains will be read (instead of being computed)
  -force-glob string
    	Glob pattern relative to music dir for files to scan and update even if they haven't changed
  -import-json-file string
    	Path to JSON file with songs to import
  -import-user-data
    	When importing from JSON, replace user data (ratings, tags, plays, etc.) (default true)
  -limit int
    	Limit the number of songs to update (for testing)
  -merge-songs string
    	Merge one song's user data into another song, with IDs as "src:dst"
  -print-cover-id string
    	Print cover ID for specified song file
  -reindex-songs
    	Ask server to reindex all songs' search-related fields (not typically neaded)
  -require-covers
    	Die if cover images aren't found for any songs that have album IDs
  -song-paths-file string
    	Path to file with one relative path per line for songs to force updating
  -test-gain-info string
    	Hardcoded gain info as "track:album:amp" (for testing)
  -use-filenames
    	Identify songs by filename rather than audio data hash (useful when modifying files)
```

### Merging songs

Suppose you have an existing file `old/song.mp3` that's already been rated,
tagged, and played. You just added a new file `new/song.mp3` that contains a
better (e.g. higher-bitrate) version of the same song from a different album.
You want to delete the old file and merge its rating, tags, and play history
into the new file.

1.  Run `nup update` to create a new database object for the new file.
2.  Use `nup dump` to produce a local text file containing JSON representations
    of all songs and find the old and new songs' `songId` properties in it.
    Alternatively, find the songs' IDs using the "Debug" menu item in the web
    interface, or run `nup query -print-id -single -path <PATH>`.
3.  Run `nup update -merge-songs=<OLDID>:<NEWID> -delete-after-merge` to merge
    the old song's user data into the new song and delete the old song from the
    server.
4.  Delete `old/song.mp3` or remove it from your local music directory.

Alternatively, you can just overwrite the old file with the new one and use `nup
update -use-filenames`.
