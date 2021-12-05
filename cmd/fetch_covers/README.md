# fetch\_covers

The `fetch_covers` executable reads JSON-marshaled
[Song objects](../../server/db/song.go) written by [dump_music](../dump_music)
and downloads the corresponding album artwork from the
[Cover Art Archive](https://coverartarchive.org/).

Google Images is also convenient for finding album artwork. A custom search for
high-resolution square images can be added to Chrome by going to
`chrome://settings/searchEngines?search=search+engines` and entering the
following information:

*   Search engine: `Google Image Search Album Art`
*   Keyword: `album`
*   URL: `https://www.google.com/search?as_st=y&tbm=isch&hl=en&as_q=%s&as_epq=&as_oq=&as_eq=&cr=&as_sitesearch=&safe=images&tbs=isz:l,iar:s`

```
Usage fetch_covers: [flag]...
Reads dumped song metadata and downloads album art from coverartarchive.org.

  -cover-dir string
        Path to directory where cover images should be written
  -dump-file string
        Path to file containing dumped JSON songs
  -max-requests int
        Maximum number of parallel HTTP requests (default 2)
  -max-songs int
        Maximum number of songs to inspect (default -1)
```
