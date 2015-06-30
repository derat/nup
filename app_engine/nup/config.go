package appengine

import (
	"appengine"
	"appengine/datastore"

	"erat.org/cloud"
	"erat.org/nup"
	"erat.org/nup/test"
)

const (
	// Config file path relative to base app directory.
	configPath = "config.json"

	// Datastore kind and ID for storing the server config for testing.
	configKind  = "ServerConfig"
	configKeyId = "config"
)

var baseConfig *nup.ServerConfig

func addTestUserToConfig(cfg *nup.ServerConfig) {
	cfg.BasicAuthUsers = append(cfg.BasicAuthUsers, nup.BasicAuthInfo{test.TestUsername, test.TestPassword})
}

// cleanBaseUrl appends a trailing slash to u if not already present.
// Does nothing if u is empty.
func cleanBaseUrl(u *string) {
	if len(*u) > 0 && (*u)[len(*u)-1] != '/' {
		*u += "/"
	}
}

func loadBaseConfig() {
	baseConfig = &nup.ServerConfig{}
	if err := cloud.ReadJson(configPath, baseConfig); err != nil {
		panic(err)
	}

	cleanBaseUrl(&baseConfig.SongBaseUrl)
	haveSongBucket := len(baseConfig.SongBucket) > 0
	haveSongUrl := len(baseConfig.SongBaseUrl) > 0
	if (haveSongBucket && haveSongUrl) || !(haveSongBucket || haveSongUrl) {
		panic("Exactly one of SongBucket and SongBaseUrl must be set")
	}

	cleanBaseUrl(&baseConfig.CoverBaseUrl)
	haveCoverBucket := len(baseConfig.CoverBucket) > 0
	haveCoverUrl := len(baseConfig.CoverBaseUrl) > 0
	if (haveCoverBucket && haveCoverUrl) || !(haveCoverBucket || haveCoverUrl) {
		panic("Exactly one of CoverBucket and CoverBaseUrl must be set")
	}

	if appengine.IsDevAppServer() {
		addTestUserToConfig(baseConfig)
	}
}

func getTestConfigKey(c appengine.Context) *datastore.Key {
	return datastore.NewKey(c, configKind, configKeyId, 0, nil)
}

func getConfig(c appengine.Context) *nup.ServerConfig {
	if appengine.IsDevAppServer() {
		testConfig := nup.ServerConfig{}
		if err := datastore.Get(c, getTestConfigKey(c), &testConfig); err == nil {
			return &testConfig
		} else if err != datastore.ErrNoSuchEntity {
			panic(err)
		}
	}

	if baseConfig == nil {
		panic("loadBaseConfig() not called")
	}
	return baseConfig
}

func saveTestConfig(c appengine.Context, cfg *nup.ServerConfig) {
	if _, err := datastore.Put(c, getTestConfigKey(c), cfg); err != nil {
		panic(err)
	}
}

func clearTestConfig(c appengine.Context) {
	if err := datastore.Delete(c, getTestConfigKey(c)); err != nil && err != datastore.ErrNoSuchEntity {
		panic(err)
	}
}
