package ecs_test

import (
	"testing"

	"github.com/yourorg/ecs"
	ecsstorage "github.com/yourorg/ecs/ecs/storage"
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
