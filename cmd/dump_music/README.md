# dump\_music

The `dump_music` executable downloads all song metadata and user data from the
[App Engine server](../../server) and writes JSON-marshaled
[Song objects](../../server/db/song.go) to stdout.

```
Usage dump_music: [flag]...
Downloads song metadata and user data from the server and writes JSON-marshaled
songs to stdout.

  -config string
        Path to config file
  -covers
        Include cover filenames
  -play-batch-size int
        Size for each batch of entities (default 800)
  -song-batch-size int
        Size for each batch of entities (default 400)
```
