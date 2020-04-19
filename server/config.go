package main

import (
	"context"
	"fmt"

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

var baseCfg *types.ServerConfig

func addTestUserToConfig(cfg *types.ServerConfig) {
	cfg.BasicAuthUsers = append(cfg.BasicAuthUsers, types.BasicAuthInfo{
		Username: cloudutil.TestUsername,
		Password: cloudutil.TestPassword,
	})
}

// cleanBaseURL appends a trailing slash to u if not already present.
// Does nothing if u is empty.
func cleanBaseURL(u *string) {
	if len(*u) > 0 && (*u)[len(*u)-1] != '/' {
		*u += "/"
	}
}

func loadBaseConfig() {
	baseCfg = &types.ServerConfig{}
	if err := cloudutil.ReadJSON(configPath, baseCfg); err != nil {
		panic(err)
	}

	cleanBaseURL(&baseCfg.SongBaseURL)
	haveSongBucket := len(baseCfg.SongBucket) > 0
	haveSongURL := len(baseCfg.SongBaseURL) > 0
	if (haveSongBucket && haveSongURL) || !(haveSongBucket || haveSongURL) {
		panic("Exactly one of SongBucket and SongBaseURL must be set")
	}

	cleanBaseURL(&baseCfg.CoverBaseURL)
	haveCoverBucket := len(baseCfg.CoverBucket) > 0
	haveCoverURL := len(baseCfg.CoverBaseURL) > 0
	if (haveCoverBucket && haveCoverURL) || !(haveCoverBucket || haveCoverURL) {
		panic("Exactly one of CoverBucket and CoverBaseURL must be set")
	}

	validCachePolicy := func(v types.CachePolicy) bool {
		return v == types.NoCaching || v == types.DatastoreCaching || v == types.MemcacheCaching
	}
	if !validCachePolicy(baseCfg.CacheCovers) || baseCfg.CacheCovers == types.DatastoreCaching {
		panic(fmt.Sprintf("Invalid cover caching policy %q", baseCfg.CacheCovers))
	}

	if appengine.IsDevAppServer() {
		addTestUserToConfig(baseCfg)
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

	if baseCfg == nil {
		panic("loadBaseConfig() not called")
	}
	return baseCfg
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
