package nup

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

type ClientType int

const (
	WebClient ClientType = iota
	AndroidClient
)

func GetServerUrl(baseUrl, path string) (*url.URL, error) {
	u, err := url.Parse(baseUrl)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	u.Path += path
	return u, nil
}

// Escapes a string that will be used as part of a cover filename.
func EscapeCoverString(s string) string {
	return strings.Replace(s, "/", "%", -1)
}

// EncodePathForCloudStorage converts the passed-in original Unix filename to the appropriate path for accessing the file via Cloud Storage.
// This includes:
// - the initial escaping performed by the cloud_sync program (a subset of query escaping),
// - regular query escaping, and
// - replacing "+" with "%20" because Cloud Storage seems unhappy otherwise.
func EncodePathForCloudStorage(p string) string {
	return strings.Replace(url.QueryEscape(EscapeObjectName(p)), "+", "%20", -1)
}

func GetCloudStorageUrl(bucketName, filePath string, client ClientType) string {
	switch client {
	case WebClient:
		return fmt.Sprintf("https://storage.cloud.google.com/%s/%s", bucketName, EncodePathForCloudStorage(filePath))
	case AndroidClient:
		return fmt.Sprintf("https://%s.storage.googleapis.com/%s", bucketName, EncodePathForCloudStorage(filePath))
	default:
		panic(fmt.Sprintf("Invalid client type %v", client))
	}
}

func SecondsToTime(s float64) time.Time {
	return time.Unix(0, int64(s*float64(time.Second/time.Nanosecond)))
}

func TimeToSeconds(t time.Time) float64 {
	return float64(t.UnixNano()) / float64(time.Second/time.Nanosecond)
}

func ReadJSON(path string, out interface{}) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	d := json.NewDecoder(f)
	if err = d.Decode(out); err != nil {
		return err
	}
	return nil
}

func EscapeObjectName(s string) string {
	// Per https://developers.google.com/storage/docs/bucketnaming#objectnames.
	for _, r := range []struct{ from, to string }{
		{"%", "%25"}, // Escape pre-existing percents.
		{"#", "%23"}, // Used to denote version numbers.
		{"[", "%5B"}, // Used for wildcards.
		{"]", "%5D"}, // Used for wildcards.
		{"*", "%2A"}, // Used for wildcards.
		{"?", "%3F"}, // Used for wildcards.
	} {
		s = strings.Replace(s, r.from, r.to, -1)
	}
	return s
}
