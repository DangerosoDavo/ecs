package storage

import (
	"fmt"

	ecs "github.com/DangerosoDavo/ecs"
)

type denseStrategy struct{}

// NewDenseStrategy constructs a dense storage strategy.
func NewDenseStrategy() ecs.StorageStrategy {
	return denseStrategy{}
}

func (denseStrategy) Name() string {
	return "dense"
}

func (denseStrategy) NewStore(t ecs.ComponentType) ecs.ComponentStore {
	return &denseStore{typ: t}
}

type denseStore struct {
	typ   ecs.ComponentType
	slots []denseSlot
	count int
}

type denseSlot struct {
	generation uint32
	value      any
	occupied   bool
}

func (s *denseStore) ComponentType() ecs.ComponentType {
	return s.typ
}

func (s *denseStore) Len() int {
	return s.count
}

func (s *denseStore) Has(id ecs.EntityID) bool {
	idx := id.Index()
	if int(idx) >= len(s.slots) {
		return false
	}
	slot := s.slots[int(idx)]
	return slot.occupied && slot.generation == id.Generation()
}

func (s *denseStore) Get(id ecs.EntityID) (any, bool) {
	if !s.Has(id) {
		return nil, false
	}
	slot := s.slots[int(id.Index())]
	return slot.value, true
}

func (s *denseStore) Iterate(fn func(ecs.EntityID, any) bool) {
	for idx, slot := range s.slots {
		if !slot.occupied {
			continue
		}
		id := ecs.EntityIDFromParts(uint32(idx), slot.generation)
		if !fn(id, slot.value) {
			return
		}
	}
}

func (s *denseStore) Set(id ecs.EntityID, value any) error {
	if id.IsZero() {
		return fmt.Errorf("dense: cannot set zero entity")
	}
	s.ensureCapacity(int(id.Index()) + 1)
	slot := &s.slots[int(id.Index())]
	if !slot.occupied {
		s.count++
	}
	slot.occupied = true
	slot.generation = id.Generation()
	slot.value = value
	return nil
}

func (s *denseStore) Remove(id ecs.EntityID) bool {
	if !s.Has(id) {
		return false
	}
	slot := &s.slots[int(id.Index())]
	slot.occupied = false
	slot.value = nil
	s.count--
	return true
}

func (s *denseStore) Clear() {
	for i := range s.slots {
		s.slots[i] = denseSlot{}
	}
	s.count = 0
}

func (s *denseStore) ensureCapacity(size int) {
	if size <= len(s.slots) {
		return
	}
	diff := size - len(s.slots)
	s.slots = append(s.slots, make([]denseSlot, diff)...)
}

var _ ecs.ComponentStore = (*denseStore)(nil)
