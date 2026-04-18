package ecs_test

import (
	"errors"
	"testing"

	"github.com/DangerosoDavo/ecs"
	ecsstorage "github.com/DangerosoDavo/ecs/ecs/storage"
)

func newDisableTestWorld(t *testing.T, components ...ecs.ComponentType) *ecs.World {
	t.Helper()
	world := ecs.NewWorld()
	for _, c := range components {
		if err := world.RegisterComponent(c, ecsstorage.NewDenseStrategy()); err != nil {
			t.Fatalf("register %s: %v", c, err)
		}
	}
	return world
}

func addComp(t *testing.T, world *ecs.World, id ecs.EntityID, comp ecs.ComponentType, value any) {
	t.Helper()
	if err := ecs.NewAddComponentCommand(id, comp, value).Apply(world); err != nil {
		t.Fatalf("add %s: %v", comp, err)
	}
}

func TestDisableComponent_IterateSkipsGetPassesThrough(t *testing.T) {
	comp := ecs.ComponentType("cargo")
	world := newDisableTestWorld(t, comp)
	id := world.Registry().Create()
	addComp(t, world, id, comp, 42)

	if err := world.DisableComponent(id, comp); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if !world.IsComponentDisabled(id, comp) {
		t.Fatalf("expected IsComponentDisabled true")
	}

	view, err := world.ViewComponent(comp)
	if err != nil {
		t.Fatalf("view: %v", err)
	}

	// Iterate must skip disabled entries
	seen := 0
	view.Iterate(func(ecs.EntityID, any) bool {
		seen++
		return true
	})
	if seen != 0 {
		t.Fatalf("expected Iterate to skip disabled entity, got %d", seen)
	}

	// Get/Has must still return the underlying data so callers can inspect
	val, ok := view.Get(id)
	if !ok || val.(int) != 42 {
		t.Fatalf("Get should pass through disabled data, got %v ok=%v", val, ok)
	}
	if !view.Has(id) {
		t.Fatalf("Has should pass through disabled data")
	}
	if view.Len() != 1 {
		t.Fatalf("Len should reflect underlying storage, got %d", view.Len())
	}
}

func TestEnableComponent_RestoresIteration(t *testing.T) {
	comp := ecs.ComponentType("vision")
	world := newDisableTestWorld(t, comp)
	id := world.Registry().Create()
	addComp(t, world, id, comp, "radius-5")

	if err := world.DisableComponent(id, comp); err != nil {
		t.Fatalf("disable: %v", err)
	}
	world.EnableComponent(id, comp)

	if world.IsComponentDisabled(id, comp) {
		t.Fatalf("component should be enabled")
	}

	view, _ := world.ViewComponent(comp)
	seen := 0
	view.Iterate(func(ecs.EntityID, any) bool {
		seen++
		return true
	})
	if seen != 1 {
		t.Fatalf("expected 1 iteration after enable, got %d", seen)
	}
}

func TestDisableComponent_NonDisableableRejected(t *testing.T) {
	world := ecs.NewWorld()
	comp := ecs.ComponentType("position")
	if err := world.RegisterComponent(comp, ecsstorage.NewDenseStrategy(), ecs.WithNonDisableable()); err != nil {
		t.Fatalf("register: %v", err)
	}
	id := world.Registry().Create()
	addComp(t, world, id, comp, "pos")

	err := world.DisableComponent(id, comp)
	if !errors.Is(err, ecs.ErrComponentNonDisableable) {
		t.Fatalf("expected ErrComponentNonDisableable, got %v", err)
	}
	if world.IsComponentDisabled(id, comp) {
		t.Fatalf("non-disableable component must stay enabled")
	}
}

func TestDisableEntity_SkipsNonDisableable(t *testing.T) {
	world := ecs.NewWorld()
	pos := ecs.ComponentType("position")
	cargo := ecs.ComponentType("cargo")
	vision := ecs.ComponentType("vision")
	if err := world.RegisterComponent(pos, ecsstorage.NewDenseStrategy(), ecs.WithNonDisableable()); err != nil {
		t.Fatalf("register pos: %v", err)
	}
	if err := world.RegisterComponent(cargo, ecsstorage.NewDenseStrategy()); err != nil {
		t.Fatalf("register cargo: %v", err)
	}
	if err := world.RegisterComponent(vision, ecsstorage.NewDenseStrategy()); err != nil {
		t.Fatalf("register vision: %v", err)
	}

	id := world.Registry().Create()
	addComp(t, world, id, pos, "xy")
	addComp(t, world, id, cargo, 10)
	addComp(t, world, id, vision, 5)

	disabled := world.DisableEntity(id)
	if len(disabled) != 2 {
		t.Fatalf("expected 2 disabled components, got %d (%v)", len(disabled), disabled)
	}

	if world.IsComponentDisabled(id, pos) {
		t.Fatalf("position must remain enabled (non-disableable)")
	}
	if !world.IsComponentDisabled(id, cargo) || !world.IsComponentDisabled(id, vision) {
		t.Fatalf("cargo and vision should be disabled")
	}

	// Position still iterates
	posView, _ := world.ViewComponent(pos)
	seenPos := 0
	posView.Iterate(func(ecs.EntityID, any) bool { seenPos++; return true })
	if seenPos != 1 {
		t.Fatalf("non-disableable component should still iterate, got %d", seenPos)
	}

	// Disabled components skip
	cargoView, _ := world.ViewComponent(cargo)
	seenCargo := 0
	cargoView.Iterate(func(ecs.EntityID, any) bool { seenCargo++; return true })
	if seenCargo != 0 {
		t.Fatalf("disabled cargo should not iterate, got %d", seenCargo)
	}
}

func TestEnableEntity_ClearsAll(t *testing.T) {
	comp := ecs.ComponentType("cargo")
	world := newDisableTestWorld(t, comp)
	id := world.Registry().Create()
	addComp(t, world, id, comp, 10)

	world.DisableEntity(id)
	if !world.IsComponentDisabled(id, comp) {
		t.Fatalf("precondition: component must be disabled")
	}

	world.EnableEntity(id)
	if world.IsComponentDisabled(id, comp) {
		t.Fatalf("EnableEntity should clear disabled state")
	}
	if got := world.DisabledComponents(id); got != nil {
		t.Fatalf("expected nil disabled list after enable, got %v", got)
	}
}

func TestDestroyEntity_ClearsDisabledState(t *testing.T) {
	comp := ecs.ComponentType("cargo")
	world := newDisableTestWorld(t, comp)
	id := world.Registry().Create()
	addComp(t, world, id, comp, 10)
	if err := world.DisableComponent(id, comp); err != nil {
		t.Fatalf("disable: %v", err)
	}

	world.DestroyEntity(id)
	if got := world.DisabledComponents(id); got != nil {
		t.Fatalf("expected disabled state cleared after destroy, got %v", got)
	}
}

func TestDisableCommands_DeferredApplication(t *testing.T) {
	comp := ecs.ComponentType("cargo")
	world := newDisableTestWorld(t, comp)
	id := world.Registry().Create()
	addComp(t, world, id, comp, 10)

	if err := world.ApplyCommands([]ecs.Command{ecs.NewDisableComponentCommand(id, comp)}); err != nil {
		t.Fatalf("apply disable cmd: %v", err)
	}
	if !world.IsComponentDisabled(id, comp) {
		t.Fatalf("expected disabled after command")
	}

	if err := world.ApplyCommands([]ecs.Command{ecs.NewEnableComponentCommand(id, comp)}); err != nil {
		t.Fatalf("apply enable cmd: %v", err)
	}
	if world.IsComponentDisabled(id, comp) {
		t.Fatalf("expected enabled after command")
	}
}

func TestDisableEntityCommand_DeferredApplication(t *testing.T) {
	world := ecs.NewWorld()
	pos := ecs.ComponentType("position")
	cargo := ecs.ComponentType("cargo")
	if err := world.RegisterComponent(pos, ecsstorage.NewDenseStrategy(), ecs.WithNonDisableable()); err != nil {
		t.Fatalf("register pos: %v", err)
	}
	if err := world.RegisterComponent(cargo, ecsstorage.NewDenseStrategy()); err != nil {
		t.Fatalf("register cargo: %v", err)
	}
	id := world.Registry().Create()
	addComp(t, world, id, pos, "xy")
	addComp(t, world, id, cargo, 99)

	if err := world.ApplyCommands([]ecs.Command{ecs.NewDisableEntityCommand(id)}); err != nil {
		t.Fatalf("disable entity cmd: %v", err)
	}
	if !world.IsComponentDisabled(id, cargo) {
		t.Fatalf("cargo should be disabled after entity command")
	}
	if world.IsComponentDisabled(id, pos) {
		t.Fatalf("position must remain enabled")
	}

	if err := world.ApplyCommands([]ecs.Command{ecs.NewEnableEntityCommand(id)}); err != nil {
		t.Fatalf("enable entity cmd: %v", err)
	}
	if world.IsComponentDisabled(id, cargo) {
		t.Fatalf("cargo should be enabled after entity command")
	}
}

func TestDisableComponent_MarksEntityChanged(t *testing.T) {
	comp := ecs.ComponentType("cargo")
	world := newDisableTestWorld(t, comp)
	id := world.Registry().Create()
	addComp(t, world, id, comp, 10)
	world.ClearChanged()

	if err := world.DisableComponent(id, comp); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if !world.Changed()[id] {
		t.Fatalf("DisableComponent should mark entity changed")
	}

	world.ClearChanged()
	world.EnableComponent(id, comp)
	if !world.Changed()[id] {
		t.Fatalf("EnableComponent should mark entity changed")
	}
}
