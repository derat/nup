// Copyright 2020 Daniel Erat.
// All rights reserved.

// Package cache sets and gets data from memcache and datastore.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"google.golang.org/appengine/v2/datastore"
	"google.golang.org/appengine/v2/memcache"
)

// Type describes a cache type.
type Type int

const (
	Memcache Type = iota
	Datastore
)

func (t Type) String() string {
	switch t {
	case Memcache:
		return "memcache"
	case Datastore:
		return "datastore"
	default:
		return strconv.Itoa(int(t))
	}
}

// jsonCodec marshals and unmarshals objects for memcache.
var jsonCodec = memcache.Codec{
	Marshal:   json.Marshal,
	Unmarshal: json.Unmarshal,
}

// GetMemcache fetches a JSON object from memcache and saves it to dst.
// If the object isn't present, ok is false and err is nil.
func GetMemcache(ctx context.Context, key string, dst interface{}) (ok bool, err error) {
	if _, err := jsonCodec.Get(ctx, key, dst); err == memcache.ErrCacheMiss {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

// SetMemcache saves JSON object src at key in memcache.
// If the update fails, the stale object (if present) is deleted.
func SetMemcache(ctx context.Context, key string, src interface{}) error {
	var errs []error
	if err := jsonCodec.Set(ctx, &memcache.Item{Key: key, Object: src}); err != nil {
		errs = append(errs, fmt.Errorf("set failed: %v", err))
		if err := DeleteMemcache(ctx, key); err != nil {
			errs = append(errs, fmt.Errorf("delete failed: %v", err))
		}
	}
	return joinErrors(errs)
}

// DeleteMemcache deletes key from memcache.
// nil is returned if the key isn't present.
func DeleteMemcache(ctx context.Context, key string) error {
	if err := memcache.Delete(ctx, key); err != nil && err != memcache.ErrCacheMiss {
		return err
	}
	return nil
}

// GetDatastore fetches an object from datastore and saves it to dst.
// If the object isn't present, ok is false and err is nil.
func GetDatastore(ctx context.Context, key *datastore.Key, dst interface{}) (ok bool, err error) {
	if err := datastore.Get(ctx, key, dst); err == datastore.ErrNoSuchEntity {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

// SetDatastore saves src at key in datastore.
// If the update fails, the stale object (if present) is deleted.
func SetDatastore(ctx context.Context, key *datastore.Key, src interface{}) error {
	var errs []error
	if _, err := datastore.Put(ctx, key, src); err != nil {
		errs = append(errs, fmt.Errorf("put failed: %v", err))
		if err := DeleteDatastore(ctx, key); err != nil {
			errs = append(errs, fmt.Errorf("delete failed: %v", err))
		}
	}
	return joinErrors(errs)
}

// DeleteDatastore deletes key from datastore.
// nil is returned if the key isn't present.
func DeleteDatastore(ctx context.Context, key *datastore.Key) error {
	if err := datastore.Delete(ctx, key); err != nil && err != datastore.ErrNoSuchEntity {
		return err
	}
	return nil
}

// joinErrors returns a new error all messages from any non-nil errors in errs.
// If no non-nil errors are present, nil is returned.
func joinErrors(errs []error) error {
	var all error
	for _, err := range errs {
		if err == nil {
			continue
		}
		if all == nil {
			all = err
		} else {
			all = fmt.Errorf("%v; %v", all.Error(), err.Error())
		}
	}
	return all
}

// Datastore property name used when serializing objects to JSON.
const jsonPropName = "json"

// LoadJSONProp implements datastore.PropertyLoadSaver's Load method.
func LoadJSONProp(props []datastore.Property, dst interface{}) error {
	if len(props) != 1 {
		return fmt.Errorf("bad property count %v", len(props))
	}
	if props[0].Name != jsonPropName {
		return fmt.Errorf("bad property name %q", props[0].Name)
	}
	b, ok := props[0].Value.([]byte)
	if !ok {
		return errors.New("property value is not byte array")
	}
	return json.Unmarshal(b, dst)
}

// SaveJSONProp implements datastore.PropertyLoadSaver's Save method.
func SaveJSONProp(src interface{}) ([]datastore.Property, error) {
	b, err := json.Marshal(src)
	if err != nil {
		return nil, err
	}
	return []datastore.Property{datastore.Property{
		Name:    jsonPropName,
		Value:   b,
		NoIndex: true},
	}, nil
}
