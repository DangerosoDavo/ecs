package ecs_test

import (
	"testing"

	"github.com/DangerosoDavo/ecs"
)

func TestEntityRegistryCreateAndDestroy(t *testing.T) {
	reg := ecs.NewEntityRegistry()
	a := reg.Create()
	b := reg.Create()

	if a == b {
		t.Fatalf("expected unique entities, got same: %v", a)
	}
	if reg.Count() != 2 {
		t.Fatalf("expected 2 live entities, got %d", reg.Count())
	}
	if !reg.IsAlive(a) || !reg.IsAlive(b) {
		t.Fatalf("expected entities to be alive")
	}

	if !reg.Destroy(a) {
		t.Fatalf("expected destroy to succeed")
	}
	if reg.IsAlive(a) {
		t.Fatalf("entity should be destroyed")
	}
	if reg.Count() != 1 {
		t.Fatalf("expected 1 live entity, got %d", reg.Count())
	}

	// Recycled entity should have new generation.
	c := reg.Create()
	if c.Index() != a.Index() {
		t.Fatalf("expected recycled index %d, got %d", a.Index(), c.Index())
	}
	if c.Generation() == a.Generation() {
		t.Fatalf("expected generation to increment on recycle")
	}
}

func TestEntityRegistryRejectsStaleId(t *testing.T) {
	reg := ecs.NewEntityRegistry()
	id := reg.Create()
	if !reg.Destroy(id) {
		t.Fatalf("destroy failed")
	}

	if reg.Destroy(id) {
		t.Fatalf("expected destroy of stale id to fail")
	}
	if reg.IsAlive(id) {
		t.Fatalf("stale id should not be alive")
	}
}
