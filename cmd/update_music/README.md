# update\_music

The `update_music` executable updates the [App Engine server](../../server)'s
song data.

By default, `update_music` scans the music directory from the config file and
sends metadata for all song files that have been added or modified since the
previous run. The `-force-glob` and `-song-paths-file` flags can be used to read
specific song files instead of scanning all files for changes.

The `-import-json-file` flag can be used to instead read JSON-marshaled
[Song objects](../../server/db/song.go) from a file. Note that any existing user
data (ratings, tags, and playback history) will be replaced by default, although
this behavior can be disabled by passing `-import-user-data=false`.

The `-delete-song-id` flag can be used to delete specific songs from the server
(e.g. after deleting them from the music dir).

```
Usage update_music: [flag]...
Sends song updates to the server.

  -config string
        Path to config file
  -delete-song-id int
        Delete song with given ID
  -dry-run
        Only print what would be updated
  -dumped-gains-file string
        Path to a dump_music file from which gains will be read (instead of being computed)
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

## Merging songs

Suppose you have an existing file `old/song.mp3` that's already been rated,
tagged, and played. You just added a new file `new/song.mp3` that contains a
better (e.g. higher-bitrate) version of the same song from a different album.
You want to delete the old file and merge its rating, tags, and play history
into the new file.

1.  Run `update_music` to create a new database object for the new file.
2.  Use [dump_music](../dump_music) to produce a local text file containing JSON
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
