# compute\_music\_gain

The `compute_music_gain` executable uses the
[mp3gain](http://mp3gain.sourceforge.net/) program to compute gain adjustments
and peak amplitudes for dumped songs.

The [update\_music](../update_music) executable computes the same data when
importing songs. This program is only useful for backfilling missing data.

```
Usage compute_music_gain: [flag]...
Computes gain adjustments for songs.
Reads "dump_music" song objects from stdin and writes updated objects to stdout.

  -music-dir string
        Directory containing song files (default "$HOME/music")
```
