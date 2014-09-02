package nup

type SongData struct {
	// SHA1 hash of the audio portion of the file.
	Sha1 string

	// Relative path from the base of the music directory.
	Filename string

	// These are empty if unset.
	Artist, Title, Album string

	// Track and disc number or 0 if unset.
	Track, Disc int

	// Length as rounded seconds.
	LengthMs int64

	// Primary key for the song in the DB.
	// Optional; this just exists for preserving IDs when importing data.
	SongId int `json:",omitempty"`

	// Error encountered when processing the file.
	Error error `json:"-"`
}

type TagData struct {
	Name         string
	CreationTime int
}

type PlayData struct {
	StartTime int
	IpAddress string
}

type ExtraSongData struct {
	// Primary key for the song in the DB.
	SongId int

	// Rating in the range [0.0, 1.0] or -1 if unrated.
	Rating float32

	Tags  []TagData  `json:",omitempty"`
	Plays []PlayData `json:",omitempty"`

	// Last time the DB row was modified (apart from playback stats) as usec since the epoch.
	LastModifiedUsec int64

	// Has the underlying file been deleted?
	Deleted bool `json:",omitempty"`
}
