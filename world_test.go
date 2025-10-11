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
