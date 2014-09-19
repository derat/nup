package nup

import (
	"net/url"
	"strings"
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
