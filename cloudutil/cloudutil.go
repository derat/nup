// Package cloudutil provides common server-related functionality.
package cloudutil

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
)

type ClientType int

const (
	WebClient ClientType = iota
	AndroidClient
)

func ServerURL(baseURL, path string) (*url.URL, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	u.Path += path
	return u, nil
}

// encodePathForCloudStorage converts the passed-in original Unix filename to
// the appropriate path for accessing the file via Cloud Storage. This includes
// both regular query escaping and replacing "+" with "%20" because Cloud
// Storage seems unhappy otherwise.
//
// See https://developers.google.com/storage/docs/bucketnaming#objectnames for
// additional object naming suggestions.
func encodePathForCloudStorage(p string) string {
	return strings.Replace(url.QueryEscape(p), "+", "%20", -1)
}

func CloudStorageURL(bucketName, filePath string, client ClientType) string {
	switch client {
	case WebClient:
		return fmt.Sprintf("https://storage.cloud.google.com/%s/%s", bucketName, encodePathForCloudStorage(filePath))
	case AndroidClient:
		return fmt.Sprintf("https://%s.storage.googleapis.com/%s", bucketName, encodePathForCloudStorage(filePath))
	default:
		panic(fmt.Sprintf("Invalid client type %v", client))
	}
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
