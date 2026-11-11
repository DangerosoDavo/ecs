package ecs_test

import (
	"context"
	"fmt"
	"time"

	"github.com/DangerosoDavo/ecs"
	"github.com/DangerosoDavo/ecs/ecs/storage"
)

type examplePosition struct{ X float64 }
type exampleVelocity struct{ X float64 }
type exampleMovement struct{}

func (exampleMovement) Descriptor() ecs.SystemDescriptor {
	return ecs.SystemDescriptor{Name: "movement", Reads: []ecs.ComponentType{"Velocity"}, Writes: []ecs.ComponentType{"Position"}}
}
func (exampleMovement) Run(_ context.Context, e ecs.ExecutionContext) ecs.SystemResult {
	positions, err := e.World().ViewComponent("Position")
	if err != nil {
		return ecs.SystemResult{Err: err}
	}
	velocities, err := e.World().ViewComponent("Velocity")
	if err != nil {
		return ecs.SystemResult{Err: err}
	}
	velocities.Iterate(func(id ecs.EntityID, value any) bool {
		if raw, ok := positions.Get(id); ok {
			position := raw.(examplePosition)
			position.X += value.(exampleVelocity).X * e.TimeDelta().Seconds()
			e.Defer(ecs.NewAddComponentCommand(id, "Position", position))
		}
		return true
	})
	return ecs.SystemResult{}
}

func ExampleNewSpawnEntityCommand() {
	w := ecs.NewWorld()
	if err := w.RegisterComponent("Position", storage.NewDenseStrategy()); err != nil {
		panic(err)
	}
	if err := w.RegisterComponent("Velocity", storage.NewDenseStrategy()); err != nil {
		panic(err)
	}
	var id ecs.EntityID
	if err := w.ApplyCommands([]ecs.Command{ecs.NewSpawnEntityCommand(&id, map[ecs.ComponentType]any{
		"Position": examplePosition{}, "Velocity": exampleVelocity{X: 2},
	})}); err != nil {
		panic(err)
	}
	s, err := ecs.NewScheduler(w)
	if err != nil {
		panic(err)
	}
	defer ecs.CloseScheduler(s)
	if _, err = s.RegisterWorkGroup(ecs.WorkGroupConfig{ID: "movement", Systems: []ecs.System{exampleMovement{}}}); err != nil {
		panic(err)
	}
	if err = s.Tick(context.Background(), time.Second); err != nil {
		panic(err)
	}
	view, err := w.ViewComponent("Position")
	if err != nil {
		panic(err)
	}
	value, ok := view.Get(id)
	if !ok {
		panic("spawn missing position")
	}
	fmt.Println(w.Registry().Count(), value.(examplePosition).X)
	// Output: 1 2
}
