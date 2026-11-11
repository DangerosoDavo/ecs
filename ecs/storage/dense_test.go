package storage

import (
	"testing"

	ecs "github.com/DangerosoDavo/ecs"
)

func TestDenseStoreCRUD(t *testing.T) {
	strategy := NewDenseStrategy()
	store := strategy.NewStore(ecs.ComponentType("comp")).(*denseStore)

	reg := ecs.NewEntityRegistry()
	id := reg.Create()

	if err := store.Set(id, 42); err != nil {
		t.Fatalf("set: %v", err)
	}
	if !store.Has(id) {
		t.Fatalf("expected Has to be true")
	}
	if got, ok := store.Get(id); !ok || got.(int) != 42 {
		t.Fatalf("unexpected get result: %#v, ok=%v", got, ok)
	}

	called := false
	store.Iterate(func(e ecs.EntityID, v any) bool {
		called = true
		if e != id {
			t.Fatalf("unexpected entity: %v", e)
		}
		if v.(int) != 42 {
			t.Fatalf("unexpected value: %v", v)
		}
		return true
	})
	if !called {
		t.Fatalf("expected iterate to visit entity")
	}

	if !store.Remove(id) {
		t.Fatalf("remove failed")
	}
	if store.Has(id) {
		t.Fatalf("value should be removed")
	}
	if store.Len() != 0 {
		t.Fatalf("expected empty store, got %d", store.Len())
	}
}

func TestDenseStoreRejectsZeroEntity(t *testing.T) {
	store := NewDenseStrategy().NewStore(ecs.ComponentType("comp"))
	if err := store.Set(ecs.EntityID{}, 10); err == nil {
		t.Fatalf("expected error for zero entity")
	}
}

func TestDenseOccupancyBoundariesAndClear(t *testing.T) {
	for _, strategy := range []ecs.StorageStrategy{NewDenseStrategy(), NewBitmapStrategy()} {
		t.Run(strategy.Name(), func(t *testing.T) { testIndexedStoreOccupancy(t, strategy) })
	}
}

func testIndexedStoreOccupancy(t *testing.T, strategy ecs.StorageStrategy) {
	store := strategy.NewStore("value")
	indices := []uint32{0, 63, 64, 127, 128, 10000}
	for _, idx := range indices {
		if err := store.Set(ecs.EntityIDFromParts(idx, 1), int(idx)); err != nil {
			t.Fatal(err)
		}
	}
	store.Remove(ecs.EntityIDFromParts(64, 1))
	if err := store.Set(ecs.EntityIDFromParts(64, 2), 64); err != nil {
		t.Fatal(err)
	}
	count := 0
	last := uint32(0)
	store.Iterate(func(id ecs.EntityID, value any) bool {
		if count > 0 && id.Index() <= last {
			t.Fatal("order changed")
		}
		last = id.Index()
		count++
		return true
	})
	if count != len(indices) || store.Has(ecs.EntityIDFromParts(64, 1)) {
		t.Fatal("occupancy/generation mismatch")
	}
	count = 0
	store.Iterate(func(ecs.EntityID, any) bool { count++; return false })
	if count != 1 {
		t.Fatal("early stop ignored")
	}
	store.Clear()
	store.Iterate(func(ecs.EntityID, any) bool { t.Fatal("clear left bitmap entries"); return true })
	if store.Len() != 0 {
		t.Fatal("clear left count")
	}
}
