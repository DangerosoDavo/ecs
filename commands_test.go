package ecs_test

import (
	"testing"

	"github.com/DangerosoDavo/ecs"
	ecsstorage "github.com/DangerosoDavo/ecs/ecs/storage"
)

func TestCreateEntityCommand(t *testing.T) {
	world := ecs.NewWorld()
	var id ecs.EntityID
	cmd := ecs.NewCreateEntityCommand(&id)
	if err := cmd.Apply(world); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if id.IsZero() {
		t.Fatalf("expected id to be populated")
	}
	if !world.Registry().IsAlive(id) {
		t.Fatalf("expected entity to exist")
	}
}

func TestDestroyEntityCommand(t *testing.T) {
	world := ecs.NewWorld()
	id := world.Registry().Create()
	cmd := ecs.NewDestroyEntityCommand(id)
	if err := cmd.Apply(world); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if world.Registry().IsAlive(id) {
		t.Fatalf("expected entity destroyed")
	}
}

func TestAddRemoveComponentCommands(t *testing.T) {
	world := ecs.NewWorld()
	comp := ecs.ComponentType("comp")
	if err := world.RegisterComponent(comp, ecsstorage.NewDenseStrategy()); err != nil {
		t.Fatalf("register component: %v", err)
	}
	id := world.Registry().Create()

	add := ecs.NewAddComponentCommand(id, comp, 99)
	if err := add.Apply(world); err != nil {
		t.Fatalf("apply add: %v", err)
	}

	view, err := world.ViewComponent(comp)
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	value, ok := view.Get(id)
	if !ok || value.(int) != 99 {
		t.Fatalf("unexpected component state: value=%v, ok=%v", value, ok)
	}

	remove := ecs.NewRemoveComponentCommand(id, comp)
	if err := remove.Apply(world); err != nil {
		t.Fatalf("apply remove: %v", err)
	}
	if view.Has(id) {
		t.Fatalf("component should be removed")
	}
}

func TestDestroyEntityRemovesComponents(t *testing.T) {
	world := ecs.NewWorld()
	compA := ecs.ComponentType("Health")
	compB := ecs.ComponentType("Position")
	if err := world.RegisterComponent(compA, ecsstorage.NewDenseStrategy()); err != nil {
		t.Fatalf("register Health: %v", err)
	}
	if err := world.RegisterComponent(compB, ecsstorage.NewDenseStrategy()); err != nil {
		t.Fatalf("register Position: %v", err)
	}

	id := world.Registry().Create()
	world.ApplyCommands([]ecs.Command{
		ecs.NewAddComponentCommand(id, compA, 100),
		ecs.NewAddComponentCommand(id, compB, "0,0"),
	})

	// Verify components exist before destroy
	viewA, _ := world.ViewComponent(compA)
	viewB, _ := world.ViewComponent(compB)
	if !viewA.Has(id) || !viewB.Has(id) {
		t.Fatalf("expected both components to exist before destroy")
	}

	// Destroy via command — should clean up components automatically
	cmd := ecs.NewDestroyEntityCommand(id)
	if err := cmd.Apply(world); err != nil {
		t.Fatalf("destroy: %v", err)
	}

	if viewA.Has(id) {
		t.Fatalf("Health component should be removed after destroy")
	}
	if viewB.Has(id) {
		t.Fatalf("Position component should be removed after destroy")
	}
	if viewA.Len() != 0 {
		t.Fatalf("Health store should be empty, got %d", viewA.Len())
	}
	if viewB.Len() != 0 {
		t.Fatalf("Position store should be empty, got %d", viewB.Len())
	}
}

func TestDestroyEntityViaWorldMethod(t *testing.T) {
	world := ecs.NewWorld()
	comp := ecs.ComponentType("Cargo")
	if err := world.RegisterComponent(comp, ecsstorage.NewDenseStrategy()); err != nil {
		t.Fatalf("register: %v", err)
	}

	id := world.Registry().Create()
	world.ApplyCommands([]ecs.Command{
		ecs.NewAddComponentCommand(id, comp, "items"),
	})

	view, _ := world.ViewComponent(comp)
	if !view.Has(id) {
		t.Fatalf("expected component before destroy")
	}

	// Use World.DestroyEntity convenience method
	if !world.DestroyEntity(id) {
		t.Fatalf("DestroyEntity returned false")
	}
	if world.Registry().IsAlive(id) {
		t.Fatalf("expected entity destroyed")
	}
	if view.Has(id) {
		t.Fatalf("component should be removed after DestroyEntity")
	}
}

func TestComponentsFor(t *testing.T) {
	world := ecs.NewWorld()
	compA := ecs.ComponentType("A")
	compB := ecs.ComponentType("B")
	compC := ecs.ComponentType("C")
	for _, c := range []ecs.ComponentType{compA, compB, compC} {
		if err := world.RegisterComponent(c, ecsstorage.NewDenseStrategy()); err != nil {
			t.Fatalf("register %s: %v", c, err)
		}
	}

	id := world.Registry().Create()
	world.ApplyCommands([]ecs.Command{
		ecs.NewAddComponentCommand(id, compA, 1),
		ecs.NewAddComponentCommand(id, compC, 3),
	})

	types := world.Storage().ComponentsFor(id)
	if len(types) != 2 {
		t.Fatalf("expected 2 components, got %d: %v", len(types), types)
	}

	found := make(map[ecs.ComponentType]bool)
	for _, ct := range types {
		found[ct] = true
	}
	if !found[compA] || !found[compC] {
		t.Fatalf("expected A and C, got %v", types)
	}
	if found[compB] {
		t.Fatalf("did not expect B, got %v", types)
	}

	// After destroy, should be empty
	world.DestroyEntity(id)
	types = world.Storage().ComponentsFor(id)
	if len(types) != 0 {
		t.Fatalf("expected 0 components after destroy, got %d", len(types))
	}
}

func TestRestoreEntityCommand(t *testing.T) {
	world := ecs.NewWorld()
	id := ecs.EntityIDFromParts(42, 7)
	cmd := ecs.NewRestoreEntityCommand(id)

	if err := cmd.Apply(world); err != nil {
		t.Fatalf("apply restore: %v", err)
	}
	if !world.Registry().IsAlive(id) {
		t.Fatalf("expected restored entity to exist")
	}

	if err := cmd.Apply(world); err == nil {
		t.Fatalf("expected duplicate restore to fail")
	}

	zero := ecs.NewRestoreEntityCommand(ecs.EntityID{})
	if err := zero.Apply(world); err == nil {
		t.Fatalf("expected zero restore to fail")
	}
}
