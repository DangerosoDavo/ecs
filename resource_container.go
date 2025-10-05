package ecs

import "sync"

type resourceMap struct {
	mu     sync.RWMutex
	values map[string]any
}

func newResourceContainer() *resourceMap {
	return &resourceMap{values: make(map[string]any)}
}

func (r *resourceMap) Get(name string) (any, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.values[name]
	return v, ok
}

func (r *resourceMap) Set(name string, value any) {
	r.mu.Lock()
	r.values[name] = value
	r.mu.Unlock()
}

func (r *resourceMap) Delete(name string) {
	r.mu.Lock()
	delete(r.values, name)
	r.mu.Unlock()
}

func (r *resourceMap) Range(fn func(string, any) bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for k, v := range r.values {
		if !fn(k, v) {
			return
		}
	}
}

var _ ResourceContainer = (*resourceMap)(nil)
