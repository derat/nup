package cloudutil

import (
	"fmt"
	"testing"
)

func TestCloudStorageURL(t *testing.T) {
	const bucket = "test-bucket"
	for _, tc := range []struct {
		path string // original file path
		obj  string // expected escaped object name
	}{
		{"simple.mp3", "simple.mp3"},
		{"dir/file.mp3", "dir%2Ffile.mp3"},
		{"100%.mp3", "100%25.mp3"},
		{"[brackets].mp3", "%5Bbrackets%5D.mp3"},
		{"this has spaces.mp3", "this%20has%20spaces.mp3"},
		{"embedded#hash.mp3", "embedded%23hash.mp3"},
		{"star*.mp3", "star%2A.mp3"},
		{"huh?.mp3", "huh%3F.mp3"},
	} {
		webExp := fmt.Sprintf("https://storage.cloud.google.com/%s/%s", bucket, tc.obj)
		if url := CloudStorageURL(bucket, tc.path, WebClient); url != webExp {
			t.Errorf("CloudStorageURL(%q, %q, WebClient) = %q; want %q", bucket, tc.path, url, webExp)
		}
		androidExp := fmt.Sprintf("https://%s.storage.googleapis.com/%s", bucket, tc.obj)
		if url := CloudStorageURL(bucket, tc.path, AndroidClient); url != androidExp {
			t.Errorf("CloudStorageURL(%q, %q, AndroidClient) = %q; want %q", bucket, tc.path, url, androidExp)
		}
	}
}
