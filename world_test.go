package ecs_test

import (
	"testing"

	"github.com/DangerosoDavo/ecs"
	ecsstorage "github.com/DangerosoDavo/ecs/ecs/storage"
)

func TestWorldRegisterComponent(t *testing.T) {
	world := ecs.NewWorld()

	strategy := ecsstorage.NewDenseStrategy()
	compType := ecs.ComponentType("position")

	if err := world.RegisterComponent(compType, strategy); err != nil {
		t.Fatalf("register component: %v", err)
	}

	if err := world.RegisterComponent(compType, strategy); err == nil {
		t.Fatalf("expected duplicate registration to fail")
	}

	view, err := world.ViewComponent(compType)
	if err != nil {
		t.Fatalf("view component: %v", err)
	}
	if view.ComponentType() != compType {
		t.Fatalf("unexpected component type: %v", view.ComponentType())
	}
}

func TestChangeTrackingManualMark(t *testing.T) {
	world := ecs.NewWorld()
	id := world.Registry().Create()

	// Initially empty
	if len(world.Changed()) != 0 {
		t.Fatalf("expected empty changed set, got %d", len(world.Changed()))
	}

	// Mark and verify
	world.MarkChanged(id)
	if !world.Changed()[id] {
		t.Fatalf("expected entity to be in changed set")
	}

	// Drain returns and clears
	drained := world.DrainChanged()
	if !drained[id] {
		t.Fatalf("expected entity in drained set")
	}
	if len(world.Changed()) != 0 {
		t.Fatalf("expected empty changed set after drain, got %d", len(world.Changed()))
	}
}

func TestChangeTrackingClear(t *testing.T) {
	world := ecs.NewWorld()
	id := world.Registry().Create()

	world.MarkChanged(id)
	world.ClearChanged()
	if len(world.Changed()) != 0 {
		t.Fatalf("expected empty changed set after clear")
	}
}

func TestChangeTrackingAutoMarkOnAddComponent(t *testing.T) {
	world := ecs.NewWorld()
	comp := ecs.ComponentType("health")
	if err := world.RegisterComponent(comp, ecsstorage.NewDenseStrategy()); err != nil {
		t.Fatalf("register: %v", err)
	}
	id := world.Registry().Create()
	world.ClearChanged()

	cmd := ecs.NewAddComponentCommand(id, comp, 100)
	if err := cmd.Apply(world); err != nil {
		t.Fatalf("apply: %v", err)
	}

	if !world.Changed()[id] {
		t.Fatalf("expected AddComponent to auto-mark entity as changed")
	}
}

func TestChangeTrackingAutoMarkOnRemoveComponent(t *testing.T) {
	world := ecs.NewWorld()
	comp := ecs.ComponentType("health")
	if err := world.RegisterComponent(comp, ecsstorage.NewDenseStrategy()); err != nil {
		t.Fatalf("register: %v", err)
	}
	id := world.Registry().Create()
	ecs.NewAddComponentCommand(id, comp, 100).Apply(world)
	world.ClearChanged()

	cmd := ecs.NewRemoveComponentCommand(id, comp)
	if err := cmd.Apply(world); err != nil {
		t.Fatalf("apply: %v", err)
	}

	if !world.Changed()[id] {
		t.Fatalf("expected RemoveComponent to auto-mark entity as changed")
	}
}

func TestChangeTrackingMultipleEntities(t *testing.T) {
	world := ecs.NewWorld()
	id1 := world.Registry().Create()
	id2 := world.Registry().Create()
	id3 := world.Registry().Create()

	world.MarkChanged(id1)
	world.MarkChanged(id3)

	changed := world.DrainChanged()
	if len(changed) != 2 {
		t.Fatalf("expected 2 changed entities, got %d", len(changed))
	}
	if !changed[id1] || !changed[id3] {
		t.Fatalf("expected id1 and id3 in changed set")
	}
	if changed[id2] {
		t.Fatalf("id2 should not be in changed set")
	}
}

func TestChangeTrackingIdempotent(t *testing.T) {
	world := ecs.NewWorld()
	id := world.Registry().Create()

	// Mark same entity multiple times
	world.MarkChanged(id)
	world.MarkChanged(id)
	world.MarkChanged(id)

	changed := world.DrainChanged()
	if len(changed) != 1 {
		t.Fatalf("expected 1 changed entity, got %d", len(changed))
	}
}

func TestResourceContainer(t *testing.T) {
	world := ecs.NewWorld()
	world.Resources().Set("clock", 123)

	value, ok := world.Resources().Get("clock")
	if !ok {
		t.Fatalf("expected resource")
	}
	if value.(int) != 123 {
		t.Fatalf("unexpected resource value: %v", value)
	}

	seen := 0
	world.Resources().Range(func(k string, v any) bool {
		seen++
		return true
	})
	if seen == 0 {
		t.Fatalf("expected Range to visit entries")
	}

	world.Resources().Delete("clock")
	if _, ok := world.Resources().Get("clock"); ok {
		t.Fatalf("resource should be deleted")
	}
}
