package nup

import (
	"net/url"
	"strings"

	"erat.org/cloud"
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
