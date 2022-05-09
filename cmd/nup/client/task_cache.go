// Copyright 2022 Daniel Erat.
// All rights reserved.

package client

import (
	"fmt"
	"sync"
)

// TaskCache runs tasks that each produce one or more key-value pairs.
type TaskCache struct {
	items    map[string]interface{} // cached values
	tasks    map[string]struct{}    // keys of in-progress tasks
	maxTasks int                    // maximum number of simultaneous tasks
	mu       sync.Mutex
	cond     *sync.Cond
}

// NewTaskCache returns a TaskCache that will run up to maxTasks simultaneous tasks.
func NewTaskCache(maxTasks int) *TaskCache {
	c := TaskCache{
		items:    make(map[string]interface{}),
		tasks:    make(map[string]struct{}),
		maxTasks: maxTasks,
	}
	c.cond = sync.NewCond(&c.mu)
	return &c
}

// Size returns the number of cached values.
func (c *TaskCache) Size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

// Task produces one or more key-value pairs.
type Task func() (map[string]interface{}, error)

// Get returns the item with the supplied key from the cache.
// If the item is not already in the cache, task will be executed
// (if another task with the same task key is not already running)
// and the resulting items will be saved to the cache.
func (c *TaskCache) Get(itemKey, taskKey string, task Task) (interface{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Wait until the item is ready or we're allowed to run the task.
	for !c.ready(itemKey, taskKey) {
		c.cond.Wait()
	}

	if v, ok := c.items[itemKey]; ok {
		return v, nil
	}

	if _, ok := c.tasks[taskKey]; ok {
		return nil, fmt.Errorf("task %q already running", taskKey)
	}
	c.tasks[taskKey] = struct{}{}

	c.mu.Unlock()
	m, err := task()
	c.mu.Lock() // see earlier deferred Unlock()

	delete(c.tasks, taskKey)
	defer c.cond.Broadcast()

	if err != nil {
		return nil, err
	}
	for k, v := range m {
		if _, ok := c.items[k]; ok {
			return nil, fmt.Errorf("task %q produced already-present item %q", taskKey, k)
		}
		c.items[k] = v
	}
	if v, ok := c.items[itemKey]; ok {
		return v, nil
	} else {
		return nil, fmt.Errorf("task %q didn't produce item %q", taskKey, itemKey)
	}
}

// ready returns true if either itemKey is in the cache or a new
// task identified by taskKey can be launched.
func (c *TaskCache) ready(itemKey, taskKey string) bool {
	if _, ok := c.items[itemKey]; ok {
		return true
	}
	if _, ok := c.tasks[taskKey]; ok {
		return false
	}
	return len(c.tasks) < c.maxTasks
}

// GetIfExists returns the item with the supplied key only if it's already been computed.
func (c *TaskCache) GetIfExists(itemKey string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.items[itemKey]
	return v, ok
}
