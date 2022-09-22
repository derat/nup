# nup Google App Engine server

## HTTP endpoints

### / (GET)

Returns the index page.

### /clear (POST, dev-only)

Deletes all song and play objects from Datastore. Used by tests.

### /config (POST, dev-only)

Modifies server behavior. Used by tests.

*   `forceUpdateFailures` (optional) - If `1`, report failures for all user data
    updates (ratings, tags, plays).

### /cover (GET)

Returns an album cover art image in JPEG format.

*   `filename` - Image path from [Song]'s `CoverFilename` field.
*   `size` (optional) - Integer cover dimensions, e.g. `400` to request that the
    image be scaled (and possibly cropped) to 400x400.
*   `webp` (optional) - If `1`, return a prescaled WebP version of the image if
    available. If unavailable, return JPEG.

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
*   `useFilenames` (optional) - If `1`, identify songs by filenames rather than
    hashes of their audio data. This is useful after updating a file's data
    (e.g. to correct errors): as long as its path renames the same, the existing
    entity will be updated rather than a new one being inserted.

### /now (GET)

Returns the server's current time as integer nanoseconds since the Unix epoch.

### /played (POST)

Records a single play of a song in Datastore. Also saves the reporter's IP
address.

*   `songId` - Integer ID from [Song]'s `SongID` field.
*   `startTime` - RFC 3339 string specifying when playback of the song started.
    Float seconds since the Unix epoch are also accepted.

### /presets (GET)

Returns a JSON-marshaled array of [SearchPreset] objects describing search
presets for the web interface.

### /query (GET)

Queries Datastore and returns a JSON-marshaled array of [Song]s.

*   `album` (optional) - String album name.
*   `albumId` (optional) - String album ID from `MusicBrainz Album Id` field,
    e.g. `124f4108-fec8-4663-b69c-19b37ff1703c`.
*   `artist` (optional) - String artist name.
*   `cacheOnly` (optional) - If `1`, only return cached data. Used by tests.
*   `keywords` (optional) - Space-separated keywords to match against artists,
    titles, and albums.
*   `fallback` (optional) - If `force`, only uses the fallback mode that tries
    to avoid using composite indexes in Datastore. If `never`, doesn't use the
    fallback mode at all. Used by tests.
*   `firstTrack` (optional) - If `1`, only returns songs that are the first
    tracks of first discs.
*   `maxDate` (optional) - RFC 3339 string containing maximum song date.
*   `maxLastPlayed` (optional) - RFC 3339 string specifying the maximum time at
    which songs were last played (to select music that hasn't been played
    recently). Float seconds since the Unix epoch are also accepted.
*   `maxPlays` (optional) - Integer maximum number of plays.
*   `minDate` (optional) - RFC 3339 string containing minimum song date.
*   `minFirstPlayed` (optional) - RFC 3339 string specifying the minimum time at
    which songs were first played (to select recently-added music). Float
    seconds since the Unix epoch are also accepted.
*   `minRating` (optional) - Integer minimum song rating in the range `[1, 5]`.
*   `orderByLastPlayed` (optional) - If `1`, return songs that were last played
    the longest ago.
*   `shuffle` (optional) - If `1`, shuffle the order of returned songs.
*   `unrated` (optional) - If `1`, return only songs that have no rating.
*   `tags` (optional) - Space-separated tags, e.g. `electronic -vocals`. Tags
    preceded by `-` must not be present. All other tags must be present.
*   `title` (optional) - String song title.

### /rate\_and\_tag (POST)

Updates a song's rating and/or tags in Datastore.

*   `rating` (optional) - Integer rating for the song in the range `[1, 5]`,
    or `0` to clear the song's rating. See [Song]'s `Rating` field.
*   `songId` - Integer ID from [Song]'s `SongID` field.
*   `tags` (optional) - Space-separated tags for the song. See [Song]'s `Tags`
    field.
*   `updateDelayNsec` (optional) - Integer value containing nanoseconds to wait
    before writing to Datastore. Used by tests.

### /reindex (POST)

Regenerates fields used for searching across all [Song] objects. Returns a JSON
object containing `scanned` and `updated` number properties and a `cursor`
string property. If the returned cursor is non-empty, another request should be
issued to continue reindexing (App Engine limits requests to 10 minutes).

*   `cursor` (optional) - Query cursor returned by previous call.

### /song (GET)

Returns a song's MP3 data.

*   `filename` - MP3 path from [Song]'s `Filename` field.

### /stats (GET)

Gets previously-computed stats about the database. Returns a JSON-marshaled
[Stats] object.

*   `update` - If `1`, update stats instead of getting them. Called periodically
    by [cron].

### /tags (GET)

Returns a JSON-marshaled array of strings containing known tags.

*   `requireCache` (optional) - If `1`, only return cached data. Used by tests.

[Config]: ./config/config.go
[Play]: ./db/song.go
[Song]: ./db/song.go
[SearchPreset]: ./config/config.go
[Stats]: ./db/stats.go
[cron]: https://cloud.google.com/appengine/docs/standard/go/scheduling-jobs-with-cron-yaml
