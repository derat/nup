# set\_music\_storage\_classes

The `set_music_storage_classes` executable reads JSON-marshaled
[Song objects](../../server/db/song.go) written by
[dump_music](../dump_music) and updates each song file's
[storage class](https://cloud.google.com/storage/docs/storage-classes) in
[Google Cloud Storage](https://cloud.google.com/storage) based on its rating.

Check the current
[Cloud Storage pricing](https://cloud.google.com/storage/pricing), but for
single-user use, it's probably most economical to just set the song bucket's
default storage class to Coldline and use that for all songs.

```
Usage set_music_storage_classes: [flag]...
Updates song files' storage classes in Google Cloud Storage.
Unmarshals "dump_music" song objects from stdin.

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
