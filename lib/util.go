package lib

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"erat.org/cloud"
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

// EncodePathForCloudStorage converts the passed-in original Unix filename to the appropriate path for accessing the file via Cloud Storage.
// This includes:
// - the initial escaping performed by the cloud_sync program (a subset of query escaping),
// - regular query escaping, and
// - replacing "+" with "%20" because Cloud Storage seems unhappy otherwise.
func EncodePathForCloudStorage(p string) string {
	return strings.Replace(url.QueryEscape(cloud.EscapeObjectName(p)), "+", "%20", -1)
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
