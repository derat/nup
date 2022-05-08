// Copyright 2022 Daniel Erat.
// All rights reserved.

package client

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestTaskCache(t *testing.T) {
	const maxTasks = 2
	c := NewTaskCache(maxTasks)

	type res struct {
		k   string
		v   interface{}
		err error
	}
	done := make(chan res)

	type taskCtl struct {
		running <-chan struct{}               // closed if/when task is running
		finish  chan<- map[string]interface{} // send to tell task to finish, close to report error
	}

	taskErr := errors.New("intentional error")

	var gets int
	get := func(itemKey, taskKey string) taskCtl {
		gets++
		running := make(chan struct{})
		finish := make(chan map[string]interface{})
		go func() {
			v, err := c.Get(itemKey, taskKey, func() (map[string]interface{}, error) {
				close(running)
				m, ok := <-finish
				if !ok {
					return nil, taskErr
				}
				return m, nil
			})
			done <- res{itemKey, v, err}
		}()
		return taskCtl{running, finish}
	}

	// Returns true if ch appears to be closed.
	closed := func(ch <-chan struct{}) bool {
		select {
		case <-ch:
			return true
		case <-time.After(20 * time.Millisecond):
			return false
		}
	}

	// Request '0' via a task named 'a'.
	t0 := get("0", "a")
	<-t0.running

	// Request '1' via a duplicate task that shouldn't be run.
	t1 := get("1", "a")
	if closed(t1.running) {
		t.Error("Duplicate task was started eventually")
	}

	// Request '2' and wait for it to start running.
	t2 := get("2", "b")
	<-t2.running

	// Request '3' and check that its task doesn't start immediately (since we're already running
	// two tasks).
	t3 := get("3", "c")
	if closed(t3.running) {
		t.Error("Extra task was started")
	}

	close(t2.finish) // trigger error
	<-t3.running
	t3.finish <- map[string]interface{}{"3": "soup"}
	t0.finish <- map[string]interface{}{"0": "foo", "1": "bar"}

	// Request '4' via a new task with the same key as the one that returned an error.
	t4 := get("4", "b")
	<-t4.running
	t4.finish <- map[string]interface{}{"4": "bananas"}

	if closed(t1.running) {
		t.Error("Duplicate task was started eventually")
	}

	got := make(map[string]interface{})
	for i := 0; i < gets; i++ {
		if r := <-done; r.err != nil {
			got[r.k] = r.err
		} else {
			got[r.k] = r.v
		}
	}
	if want := map[string]interface{}{
		"0": "foo",
		"1": "bar",
		"2": taskErr,
		"3": "soup",
		"4": "bananas",
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("Got %v; want %v", got, want)
	}
}
