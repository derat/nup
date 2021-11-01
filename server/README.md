# nup Google App Engine server

## HTTP endpoints

### / (GET)

Returns the index page.

### /clear (POST, dev-only)

Deletes all song and play objects from Datastore. Used by tests.

### /config (POST, dev-only)

Unmarshals a JSON-encoded [ServerConfig] struct from the request body and
applies it. Used by tests.

### /cover (GET)

Returns an album cover art image in JPEG format.

*   `filename` - Image path from [Song]'s `CoverFilename` field.
*   `size` (optional) - Integer cover dimensions, e.g. `400` to request that the
    image be scaled (and possibly cropped) to 400x400.

### /delete\_song (POST)

Deletes a song from Datastore.

*   `songId` - Integer ID from [Song]'s `SongID` field.

### /dump\_song (GET)

Returns a JSON-marshaled [Song] object. [Play]s are included.

*   `songId` - Integer ID from [Song]'s `SongID` field.

### /export (GET)

Returns a series of JSON-marshaled [Song] or [Play] objects, followed by an
optional JSON string containing a cursor for the next batch if not all objects
were returned.

*   `cursor` (optional) - Cursor to continue an earlier request.
*   `max` (optional) - Integer maximum number of items to return.
*   `type` - Type of entity to export (`song` or `play`).

Several parameters are only relevant for the `song` type:

*   `deleted` (optional) - If `1`, return deleted songs rather than non-deleted
    songs.
*   `minLastModifiedNsec` (optional) - Integer nanoseconds since Unix epoch of
    songs' last-modified timestamps. Used for incremental updates.
*   `omit` (optional) - Comma-separated list of [Song] fields to clear.
    Available fields are `coverFilename`, `plays`, and `sha1`.

### /flush\_cache (POST, dev-only)

Flushes data cached in Memcache (and possibly also in Datastore). Used by tests.

*   `onlyMemcache` (optional) - If `1`, don't flush the Datastore cache.

### /import (POST)

Imports a series (not an array) of JSON-marshaled [Song] and [Play] objects
into Datastore.

*   `replaceUserData` (optional) - If `1`, replace the songs' existing user data
    in Datastore (ratings, tags, play history) with user data from the supplied
    songs. Otherwise, the existing data is preserved.
*   `updateDelayNsec` (optional) - Integer value containing nanoseconds to wait
    before writing to Datastore. Used by tests.

### /now (GET)

Returns the server's current time as integer nanoseconds since the Unix epoch.

### /played (POST)

Records a single play of a song in Datastore. Also saves the reporter's IP
address.

*   `songId` - Integer ID from [Song]'s `SongID` field.
*   `startTime` - Float seconds since the Unix epoch specifying when playback
    of the song started.

### /query (GET)

Queries Datastore and returns a JSON-marshaled array of [Song]s.

*   `album` (optional) - String album name.
*   `albumId` (optional) - String album ID from `MusicBrainz Album Id` field,
    e.g. `124f4108-fec8-4663-b69c-19b37ff1703c`.
*   `artist` (optional) - String artist name.
*   `cacheOnly` (optional) - If `1`, only return cached data. Used by tests.
*   `keywords` (optional) - Space-separated keywords to match against artists,
    titles, and albums.
*   `firstTrack` (optional) - If `1`, only returns songs that are the first
    tracks of first discs.
*   `minFirstPlayed` (optional) - Float seconds since the Unix epoch specifying
    the minimum time at which songs were first played (used to select
    recently-added music).
*   `minRating` (optional) - Float minimum song rating in the range `(0.0,
    1.0]`.
*   `maxLastPlayed` (optional) - Float seconds since the Unix epoch specifying
    the maximmum time at which songs were last played (used to select music that
    hasn't been played recently).
*   `maxPlays` (optional) - Integer maximum number of plays.
*   `shuffle` (optional) - If `1`, shuffle the order of returned songs.
*   `unrated` (optional) - If `1`, return only songs that have no rating.
*   `tags` (optional) - Space-separated tags, e.g. `electronic -vocals`. Tags
    preceded by `-` must not be present. All other tags must be present.
*   `title` (optional) - String song title.

### /rate\_and\_tag (POST)

Updates a song's rating and/or tags in Datastore.

*   `rating` (optional) - Float rating for the song in the range `[0.0, 1.0]`,
    or `-1` to clear the song's rating. See [Song]'s `Rating` field.
*   `songId` - Integer ID from [Song]'s `SongID` field.
*   `tags` (optional) - Space-separated tags for the song. See [Song]'s `Tags`
    field.
*   `updateDelayNsec` (optional) - Integer value containing nanoseconds to wait
    before writing to Datastore. Used by tests.

### /song (GET)

Returns a song's MP3 data.

*   `filename` - MP3 path from [Song]'s `Filename` field.

### /tags (GET)

Returns a JSON-marshaled array of strings containing known tags.

*   `requireCache` (optional) - If `1`, only return cached data. Used by tests.

[ServerConfig]: ./types/config.go
[Play]: ./types/song.go
[Song]: ./types/song.go