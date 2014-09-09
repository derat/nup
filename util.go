package nup

import (
	"net/url"
	"strings"
)

func GetServerUrl(cfg ClientConfig, path string) (*url.URL, error) {
	u, err := url.Parse(cfg.ServerUrl)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	u.Path += path
	return u, nil
}
