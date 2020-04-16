package main

import (
	"context"

	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"

	"github.com/derat/nup/cloudutil"
	"github.com/derat/nup/types"
)

const (
	// Config file path relative to base directory.
	configPath = "config.json"

	// Datastore kind and ID for storing the server config for testing.
	configKind  = "ServerConfig"
	configKeyId = "config"
)

var baseConfig *types.ServerConfig

func addTestUserToConfig(cfg *types.ServerConfig) {
	cfg.BasicAuthUsers = append(cfg.BasicAuthUsers, types.BasicAuthInfo{cloudutil.TestUsername, cloudutil.TestPassword})
}

// cleanBaseURL appends a trailing slash to u if not already present.
// Does nothing if u is empty.
func cleanBaseURL(u *string) {
	if len(*u) > 0 && (*u)[len(*u)-1] != '/' {
		*u += "/"
	}
}

func loadBaseConfig() {
	baseConfig = &types.ServerConfig{}
	if err := cloudutil.ReadJSON(configPath, baseConfig); err != nil {
		panic(err)
	}

	cleanBaseURL(&baseConfig.SongBaseURL)
	haveSongBucket := len(baseConfig.SongBucket) > 0
	haveSongURL := len(baseConfig.SongBaseURL) > 0
	if (haveSongBucket && haveSongURL) || !(haveSongBucket || haveSongURL) {
		panic("Exactly one of SongBucket and SongBaseURL must be set")
	}

	cleanBaseURL(&baseConfig.CoverBaseURL)
	haveCoverBucket := len(baseConfig.CoverBucket) > 0
	haveCoverURL := len(baseConfig.CoverBaseURL) > 0
	if (haveCoverBucket && haveCoverURL) || !(haveCoverBucket || haveCoverURL) {
		panic("Exactly one of CoverBucket and CoverBaseURL must be set")
	}

	if baseConfig.CacheSongs && !baseConfig.UseMemcache {
		panic("CacheSongs requires UseMemcache to be true")
	}

	if appengine.IsDevAppServer() {
		addTestUserToConfig(baseConfig)
	}
}

func getTestConfigKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, configKind, configKeyId, 0, nil)
}

func getConfig(ctx context.Context) *types.ServerConfig {
	if appengine.IsDevAppServer() {
		testConfig := types.ServerConfig{}
		if err := datastore.Get(ctx, getTestConfigKey(ctx), &testConfig); err == nil {
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

func saveTestConfig(ctx context.Context, cfg *types.ServerConfig) {
	if _, err := datastore.Put(ctx, getTestConfigKey(ctx), cfg); err != nil {
		panic(err)
	}
}

func clearTestConfig(ctx context.Context) {
	if err := datastore.Delete(ctx, getTestConfigKey(ctx)); err != nil && err != datastore.ErrNoSuchEntity {
		panic(err)
	}
}
