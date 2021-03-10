// Copyright 2020 Daniel Erat.
// All rights reserved.

package main

import (
	"context"
	"errors"

	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"

	"github.com/derat/nup/server/types"
)

const (
	// Config file path relative to base directory.
	configPath = "config.json"

	// Datastore kind and ID for storing the server config for testing.
	configKind  = "ServerConfig"
	configKeyID = "config"
)

// Singleton loaded from disk by loadConfig().
var baseCfg *types.ServerConfig

func addTestUserToConfig(cfg *types.ServerConfig) {
	cfg.BasicAuthUsers = append(cfg.BasicAuthUsers, types.BasicAuthInfo{
		Username: types.TestUsername,
		Password: types.TestPassword,
	})
}

// cleanBaseURL appends a trailing slash to u if not already present.
// Does nothing if u is empty.
func cleanBaseURL(u *string) {
	if len(*u) > 0 && (*u)[len(*u)-1] != '/' {
		*u += "/"
	}
}

// loadConfig loads the server configuration from disk.
// It should be called once at the start of main().
func loadConfig() error {
	var err error
	if baseCfg, err = types.LoadServerConfig(configPath); err != nil {
		return err
	}

	cleanBaseURL(&baseCfg.SongBaseURL)
	haveSongBucket := len(baseCfg.SongBucket) > 0
	haveSongURL := len(baseCfg.SongBaseURL) > 0
	if (haveSongBucket && haveSongURL) || !(haveSongBucket || haveSongURL) {
		return errors.New("exactly one of SongBucket and SongBaseURL must be set")
	}

	cleanBaseURL(&baseCfg.CoverBaseURL)
	haveCoverBucket := len(baseCfg.CoverBucket) > 0
	haveCoverURL := len(baseCfg.CoverBaseURL) > 0
	if (haveCoverBucket && haveCoverURL) || !(haveCoverBucket || haveCoverURL) {
		return errors.New("exactly one of CoverBucket and CoverBaseURL must be set")
	}

	if appengine.IsDevAppServer() {
		addTestUserToConfig(baseCfg)
	}
	return nil
}

func testConfigKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, configKind, configKeyID, 0, nil)
}

// getConfig returns the currently-in-use config.
func getConfig(ctx context.Context) *types.ServerConfig {
	if appengine.IsDevAppServer() {
		var testConfig types.ServerConfig
		if err := datastore.Get(ctx, testConfigKey(ctx), &testConfig); err == nil {
			return &testConfig
		} else if err != datastore.ErrNoSuchEntity {
			panic(err)
		}
	}

	if baseCfg == nil {
		panic("loadConfig() not called")
	}
	return baseCfg
}

func saveTestConfig(ctx context.Context, cfg *types.ServerConfig) {
	if _, err := datastore.Put(ctx, testConfigKey(ctx), cfg); err != nil {
		panic(err)
	}
}

func clearTestConfig(ctx context.Context) {
	if err := datastore.Delete(ctx, testConfigKey(ctx)); err != nil && err != datastore.ErrNoSuchEntity {
		panic(err)
	}
}
