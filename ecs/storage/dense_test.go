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
