# nup

App Engine app for streaming a music collection.

## Deploying the app

From the base directory, run:

```sh
gcloud app deploy --project=<PROJECT_ID>
```

## Development and testing

First, from the base directory, run:

```sh
dev_appserver.py --datastore_consistency_policy=consistent .
```

This starts a local development App Engine instance listening at
`http://localhost:8080`.

*   End-to-end Go tests that exercise the App Engine server can be run from the
    `test/` directory via `go test`.
*   Selenium tests that exercise both the web client and the server can be run
    from the `test/web/` directory via `./tests.py`.

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
    update_music -config $HOME/.nup/update_music.json delete-song-id <ID>
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
