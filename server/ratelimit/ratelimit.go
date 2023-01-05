// Copyright 2023 Daniel Erat.
// All rights reserved.

// Package ratelimit is used to rate-limit requests.
package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/appengine/v2/datastore"
)

const rateInfoKind = "RateInfo" // datastore key for rateInfo entities

// rateInfo holds information about a single client's requests.
type rateInfo struct {
	// Times holds the times at which recent successful requests were received.
	Times []time.Time
}

// Attempt determines if a new request by the client identified by user is allowed.
// An error is returned if max or more successful attempts were already made in interval.
// Errors can also be returned for datastore failures.
func Attempt(ctx context.Context, user string, now time.Time, max int, interval time.Duration) error {
	key := datastore.NewKey(ctx, rateInfoKind, user, 0, nil)
	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		var info rateInfo
		if err := datastore.Get(ctx, key, &info); err != nil && err != datastore.ErrNoSuchEntity {
			return fmt.Errorf("get rate info: %v", err)
		}

		// Count the previous requests in the interval while also dropping older requests.
		var count int
		start := now.Add(-interval)
		for _, t := range info.Times {
			if !t.Before(start) {
				info.Times[count] = t
				count++
			}
		}
		if count >= max {
			return errors.New("request rate exceeded")
		}

		// Only update the saved info if the attempt was successful.
		info.Times = append(info.Times[:count], now)
		if _, err := datastore.Put(ctx, key, &info); err != nil {
			return fmt.Errorf("save rate info: %v", err)
		}
		return nil
	}, nil)
}

// Clear deletes all rate-limiting information from datastore for testing.
func Clear(ctx context.Context) error {
	if keys, err := datastore.NewQuery(rateInfoKind).KeysOnly().GetAll(ctx, nil); err != nil {
		return fmt.Errorf("getting %v keys failed: %v", rateInfoKind, err)
	} else if err := datastore.DeleteMulti(ctx, keys); err != nil {
		return fmt.Errorf("deleting all %v entities failed: %v", rateInfoKind, err)
	}
	return nil
}
