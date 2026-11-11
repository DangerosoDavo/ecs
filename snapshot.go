package ecs

import (
	"fmt"
	"reflect"
	"sort"
)

// SnapshotCloner supplies a detached copy for opaque values (for example types
// with private fields). The returned graph must not alias mutable live state.
// Database connections, locks, channels and other live handles should instead
// be kept outside ECS async systems and consume copied messages.
type SnapshotCloner interface{ CloneForECS() (any, error) }

type snapshotVisit struct {
	typ    reflect.Type
	ptr    uintptr
	length int
}

func cloneSnapshot(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	v, err := cloneSnapshotValue(reflect.ValueOf(value), make(map[snapshotVisit]reflect.Value))
	if err != nil {
		return nil, err
	}
	return v.Interface(), nil
}

func cloneSnapshotValue(v reflect.Value, seen map[snapshotVisit]reflect.Value) (reflect.Value, error) {
	if v.CanInterface() {
		if cloner, ok := v.Interface().(SnapshotCloner); ok {
			// A typed nil pointer is data, not an instance on which to call methods.
			if v.Kind() != reflect.Pointer || !v.IsNil() {
				copy, err := cloner.CloneForECS()
				if err != nil {
					return reflect.Value{}, err
				}
				if copy == nil {
					return reflect.Zero(v.Type()), nil
				}
				result := reflect.ValueOf(copy)
				if !result.Type().AssignableTo(v.Type()) {
					return reflect.Value{}, fmt.Errorf("%w: clone of %s returned %s", ErrUnsupportedSnapshot, v.Type(), result.Type())
				}
				return result, nil
			}
		}
	}
	// EntityID is an immutable value with intentionally private representation.
	if v.Type() == reflect.TypeFor[EntityID]() {
		return v, nil
	}
	switch v.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr, reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128, reflect.String:
		return v, nil
	case reflect.Interface:
		if v.IsNil() {
			return reflect.Zero(v.Type()), nil
		}
		inner, err := cloneSnapshotValue(v.Elem(), seen)
		if err != nil {
			return reflect.Value{}, err
		}
		out := reflect.New(v.Type()).Elem()
		out.Set(inner)
		return out, nil
	case reflect.Pointer, reflect.Map, reflect.Slice:
		if v.IsNil() {
			return reflect.Zero(v.Type()), nil
		}
		key := snapshotVisit{typ: v.Type(), ptr: uintptr(v.UnsafePointer())}
		if v.Kind() == reflect.Slice {
			key.length = v.Len()
		}
		if prior, ok := seen[key]; ok {
			return prior, nil
		}
		var out reflect.Value
		switch v.Kind() {
		case reflect.Pointer:
			out = reflect.New(v.Type().Elem())
			seen[key] = out
			inner, err := cloneSnapshotValue(v.Elem(), seen)
			if err != nil {
				return reflect.Value{}, err
			}
			out.Elem().Set(inner)
		case reflect.Slice:
			out = reflect.MakeSlice(v.Type(), v.Len(), v.Len())
			seen[key] = out
			for i := 0; i < v.Len(); i++ {
				inner, err := cloneSnapshotValue(v.Index(i), seen)
				if err != nil {
					return reflect.Value{}, err
				}
				out.Index(i).Set(inner)
			}
		case reflect.Map:
			out = reflect.MakeMapWithSize(v.Type(), v.Len())
			seen[key] = out
			iter := v.MapRange()
			for iter.Next() {
				k, err := cloneSnapshotValue(iter.Key(), seen)
				if err != nil {
					return reflect.Value{}, err
				}
				value, err := cloneSnapshotValue(iter.Value(), seen)
				if err != nil {
					return reflect.Value{}, err
				}
				out.SetMapIndex(k, value)
			}
		}
		return out, nil
	case reflect.Struct, reflect.Array:
		out := reflect.New(v.Type()).Elem()
		count := v.Len
		if v.Kind() == reflect.Struct {
			for i := 0; i < v.NumField(); i++ {
				if v.Type().Field(i).PkgPath != "" {
					return reflect.Value{}, fmt.Errorf("%w: private field %s.%s", ErrUnsupportedSnapshot, v.Type(), v.Type().Field(i).Name)
				}
				inner, err := cloneSnapshotValue(v.Field(i), seen)
				if err != nil {
					return reflect.Value{}, err
				}
				out.Field(i).Set(inner)
			}
		} else {
			for i := 0; i < count(); i++ {
				inner, err := cloneSnapshotValue(v.Index(i), seen)
				if err != nil {
					return reflect.Value{}, err
				}
				out.Index(i).Set(inner)
			}
		}
		return out, nil
	default:
		return reflect.Value{}, fmt.Errorf("%w: %s", ErrUnsupportedSnapshot, v.Type())
	}
}

// snapshotWorld runs on the simulation owner before worker dispatch. Only
// declared reads are copied. The worker never obtains the live world.
func snapshotWorld(source *World, group *workGroupState) (*World, error) {
	w := NewWorld(WithChangeHistoryCapacity(0))
	r := source.registry
	r.mu.Lock()
	w.registry.generations = append([]uint32(nil), r.generations...)
	w.registry.allocated = append([]bool(nil), r.allocated...)
	w.registry.free = append([]uint32(nil), r.free...)
	w.registry.alive = r.alive
	r.mu.Unlock()
	for _, t := range componentSetToSlice(group.readSet) {
		view, err := source.storage.View(t)
		if err != nil {
			return nil, err
		}
		store := &snapshotStore{typ: t, values: make(map[EntityID]any, view.Len())}
		var cloneErr error
		view.Iterate(func(id EntityID, value any) bool {
			copy, err := cloneSnapshot(value)
			if err != nil {
				cloneErr = fmt.Errorf("component %s: %w", t, err)
				return false
			}
			store.values[id] = copy
			store.ids = append(store.ids, id)
			if source.IsComponentDisabled(id, t) {
				if w.disabled[id] == nil {
					w.disabled[id] = make(map[ComponentType]bool)
				}
				w.disabled[id][t] = true
			}
			return true
		})
		if cloneErr != nil {
			return nil, cloneErr
		}
		sort.Slice(store.ids, func(i, j int) bool { return store.ids[i].Index() < store.ids[j].Index() })
		w.storage.(*storageProvider).stores[t] = store
		if cfg, ok := source.ComponentConfig(t); ok {
			w.componentCfgs[t] = cfg
		}
	}
	for _, name := range stringSetToSlice(group.resourceReads) {
		value, ok := source.resources.Get(name)
		if !ok {
			continue
		}
		copy, err := cloneSnapshot(value)
		if err != nil {
			return nil, fmt.Errorf("resource %s: %w", name, err)
		}
		w.resources.Set(name, copy)
	}
	return w, nil
}

type snapshotStore struct {
	typ    ComponentType
	values map[EntityID]any
	ids    []EntityID
}

func (s *snapshotStore) ComponentType() ComponentType { return s.typ }
func (s *snapshotStore) Len() int                     { return len(s.values) }
func (s *snapshotStore) Has(id EntityID) bool         { _, ok := s.values[id]; return ok }
func (s *snapshotStore) Get(id EntityID) (any, bool)  { value, ok := s.values[id]; return value, ok }
func (s *snapshotStore) Iterate(fn func(EntityID, any) bool) {
	for _, id := range s.ids {
		if value, ok := s.values[id]; ok && !fn(id, value) {
			return
		}
	}
}
func (s *snapshotStore) Set(id EntityID, value any) error {
	if _, ok := s.values[id]; !ok {
		s.ids = append(s.ids, id)
	}
	s.values[id] = value
	return nil
}
func (s *snapshotStore) Remove(id EntityID) bool {
	_, ok := s.values[id]
	delete(s.values, id)
	return ok
}
func (s *snapshotStore) Clear() { clear(s.values); s.ids = nil }
