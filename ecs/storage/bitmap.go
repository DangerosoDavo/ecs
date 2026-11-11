package storage

import (
	"math/bits"

	ecs "github.com/DangerosoDavo/ecs"
)

type bitmapStrategy struct{}

// NewBitmapStrategy retains index-addressed lookups and ascending iteration,
// while skipping empty runs using occupancy words. Choose it for sparse buckets;
// NewDenseStrategy retains its simpler, compiler-inlinable loop for dense ones.
// Both retain storage proportional to the highest allocated local entity index.
func NewBitmapStrategy() ecs.StorageStrategy { return bitmapStrategy{} }
func (bitmapStrategy) Name() string          { return "bitmap" }
func (bitmapStrategy) NewStore(t ecs.ComponentType) ecs.ComponentStore {
	return &bitmapStore{denseStore: &denseStore{typ: t}}
}

type bitmapStore struct {
	*denseStore
	occupied []uint64
}

func (s *bitmapStore) Set(id ecs.EntityID, value any) error {
	if err := s.denseStore.Set(id, value); err != nil {
		return err
	}
	words := (len(s.slots) + 63) / 64
	if words > len(s.occupied) {
		s.occupied = append(s.occupied, make([]uint64, words-len(s.occupied))...)
	}
	s.occupied[id.Index()/64] |= uint64(1) << (id.Index() % 64)
	return nil
}
func (s *bitmapStore) Remove(id ecs.EntityID) bool {
	if !s.denseStore.Remove(id) {
		return false
	}
	s.occupied[id.Index()/64] &^= uint64(1) << (id.Index() % 64)
	return true
}
func (s *bitmapStore) Clear() { s.denseStore.Clear(); clear(s.occupied) }
func (s *bitmapStore) Iterate(fn func(ecs.EntityID, any) bool) {
	for wordIndex, word := range s.occupied {
		for word != 0 {
			index := wordIndex*64 + bits.TrailingZeros64(word)
			slot := s.slots[index]
			if !fn(ecs.EntityIDFromParts(uint32(index), slot.generation), slot.value) {
				return
			}
			word &= word - 1
		}
	}
}

var _ ecs.ComponentStore = (*bitmapStore)(nil)
