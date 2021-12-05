# check\_music

The `check_music` executable checks for issues in songs dumped by
[dump\_music](../dump_music) or in cover images.

```
Usage check_music: [flag]...
Check for issues in songs and cover images.
Reads "dump_music -covers" song objects from stdin.

  -check string
        Comma-separated list of checks to perform:
          album-id        Songs have MusicBrainz album IDs
          cover-size-400  Cover images are at least 400x400
          imported        All songs have been imported
          song-cover      Songs have cover files
         (default "album-id,imported,song-cover")
  -cover-dir string
        Directory containing cover art (".covers" within -music-dir if unset)
  -music-dir string
        Directory containing song files (default "$HOME/music")
```
