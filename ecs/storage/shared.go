package storage

import (
	"fmt"
	"reflect"
	"sync"

	ecs "github.com/DangerosoDavo/ecs"
)

// SharedStorageStrategy creates stores where multiple entities can reference the same
// component instance. This is useful for entities with identical data (e.g., all zombies
// sharing the same base stats) and provides memory efficiency for large entity counts.
//
// Shared components are immutable from the perspective of individual entities. To "modify"
// a shared component, remove it and add a new value. This ensures predictable behavior
// when multiple entities reference the same data.
type sharedStrategy struct{}

// NewSharedStrategy constructs a shared storage strategy.
func NewSharedStrategy() ecs.StorageStrategy {
	return sharedStrategy{}
}

func (sharedStrategy) Name() string {
	return "shared"
}

func (sharedStrategy) NewStore(t ecs.ComponentType) ecs.ComponentStore {
	return &sharedStore{
		typ:           t,
		entityToValue: make(map[ecs.EntityID]uint32),
		valueToData:   make(map[uint32]*sharedValue),
		nextValueID:   1,
	}
}

// sharedValue holds a component value and tracks how many entities reference it.
type sharedValue struct {
	data     any
	refCount int
}

// sharedStore implements ComponentStore with shared component instances.
type sharedStore struct {
	mu            sync.RWMutex
	typ           ecs.ComponentType
	entityToValue map[ecs.EntityID]uint32  // maps entity to value ID
	valueToData   map[uint32]*sharedValue  // maps value ID to actual data
	nextValueID   uint32
	count         int // number of entities with components (not unique values)
}

func (s *sharedStore) ComponentType() ecs.ComponentType {
	return s.typ
}

func (s *sharedStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.count
}

func (s *sharedStore) Has(id ecs.EntityID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.entityToValue[id]
	return exists
}

func (s *sharedStore) Get(id ecs.EntityID) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	valueID, exists := s.entityToValue[id]
	if !exists {
		return nil, false
	}

	sharedVal, ok := s.valueToData[valueID]
	if !ok {
		// This should never happen, but handle gracefully
		return nil, false
	}

	return sharedVal.data, true
}

func (s *sharedStore) Iterate(fn func(ecs.EntityID, any) bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for entityID, valueID := range s.entityToValue {
		sharedVal, ok := s.valueToData[valueID]
		if !ok {
			continue
		}
		if !fn(entityID, sharedVal.data) {
			return
		}
	}
}

func (s *sharedStore) Set(id ecs.EntityID, value any) error {
	if id.IsZero() {
		return fmt.Errorf("shared: cannot set zero entity")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// If entity already has this component, remove the old reference first
	if oldValueID, exists := s.entityToValue[id]; exists {
		s.decrementRefCountLocked(oldValueID)
	} else {
		// New entity getting this component
		s.count++
	}

	// Find or create value ID for this component value
	valueID := s.findOrCreateValueLocked(value)
	s.entityToValue[id] = valueID

	return nil
}

func (s *sharedStore) Remove(id ecs.EntityID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	valueID, exists := s.entityToValue[id]
	if !exists {
		return false
	}

	delete(s.entityToValue, id)
	s.decrementRefCountLocked(valueID)
	s.count--

	return true
}

func (s *sharedStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entityToValue = make(map[ecs.EntityID]uint32)
	s.valueToData = make(map[uint32]*sharedValue)
	s.count = 0
}

// findOrCreateValueLocked finds an existing value ID for the given data, or creates a new one.
// This method deduplicates component values using deep equality checks.
func (s *sharedStore) findOrCreateValueLocked(value any) uint32 {
	// Search for existing value with deep equality
	for valueID, sharedVal := range s.valueToData {
		if reflect.DeepEqual(sharedVal.data, value) {
			sharedVal.refCount++
			return valueID
		}
	}

	// Value not found, create new entry
	valueID := s.nextValueID
	s.nextValueID++
	s.valueToData[valueID] = &sharedValue{
		data:     value,
		refCount: 1,
	}

	return valueID
}

// decrementRefCountLocked decreases the reference count for a value and removes it if unused.
func (s *sharedStore) decrementRefCountLocked(valueID uint32) {
	sharedVal, ok := s.valueToData[valueID]
	if !ok {
		return
	}

	sharedVal.refCount--
	if sharedVal.refCount <= 0 {
		delete(s.valueToData, valueID)
	}
}

// Stats returns statistics about the shared store for debugging and optimization.
func (s *sharedStore) Stats() SharedStorageStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return SharedStorageStats{
		EntityCount:      s.count,
		UniqueValueCount: len(s.valueToData),
		SharingRatio:     float64(s.count) / float64(max(len(s.valueToData), 1)),
	}
}

// SharedStorageStats provides metrics about shared component storage efficiency.
type SharedStorageStats struct {
	EntityCount      int     // number of entities with this component
	UniqueValueCount int     // number of unique component values
	SharingRatio     float64 // average entities per unique value (higher = more sharing)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var _ ecs.ComponentStore = (*sharedStore)(nil)
