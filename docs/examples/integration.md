# Integrating the ECS Scheduler Into Another Project

This guide shows how to embed `github.com/yourorg/ecs` into a separate codebase. You’ll install the module, define components/systems, bootstrap the scheduler, and see how this architecture powers a real-time arena combat server.

## 1. Add the Dependency

```bash
go get github.com/DangerosoDavo/ecs@latest
```

If you mirror dependencies, replace the module path accordingly.

## 2. Define Components and Systems

Create simple Go structs for components and implement `ecs.System` for your behaviour.

```go
// components.go
package game

type Position struct {
    X, Y float64
}

type Velocity struct {
    VX, VY float64
}

type GameStats struct {
    Health       int
    AttackDamage int
    Defense      int
}
```

```go
// movement_system.go
package game

import "github.com/DangerosoDavo/ecs"

type MovementSystem struct{}

func (MovementSystem) Descriptor() ecs.SystemDescriptor {
    return ecs.SystemDescriptor{
        Name:         "movement",
        Reads:        []ecs.ComponentType{"Velocity"},
        Writes:       []ecs.ComponentType{"Position"},
        RunEvery:     ecs.TickInterval{Every: 1},
        AsyncAllowed: true, // read-only velocity, writes position via deferred commands
    }
}

func (MovementSystem) Run(ctx ecs.ExecutionContext) ecs.SystemResult {
    posView, err := ctx.World().ViewComponent("Position")
    if err != nil {
        return ecs.SystemResult{Err: err}
    }
    velView, err := ctx.World().ViewComponent("Velocity")
    if err != nil {
        return ecs.SystemResult{Err: err}
    }

    posStore := posView.(ecs.ComponentStore)
    velView.(ecs.ComponentView).Iterate(func(id ecs.EntityID, component any) bool {
        vel := component.(Velocity)
        if pos, ok := posStore.Get(id); ok {
            p := pos.(Position)
            p.X += vel.VX * ctx.TimeDelta().Seconds()
            p.Y += vel.VY * ctx.TimeDelta().Seconds()
            ctx.Defer(ecs.NewAddComponentCommand(id, "Position", p))
        }
        return true
    })

    return ecs.SystemResult{}
}
```

## 3. Bootstrap the World and Scheduler

```go
package main

import (
    "context"
    "time"

    "github.com/DangerosoDavo/ecs"
    ecsstorage "github.com/DangerosoDavo/ecs/ecs/storage"
    "github.com/DangerosoDavo/ecs/examples/game"
)

func buildScheduler() ecs.Scheduler {
    world := ecs.NewWorld()

    // Use DenseStrategy for components unique to each entity
    world.RegisterComponent("Position", ecsstorage.NewDenseStrategy())
    world.RegisterComponent("Velocity", ecsstorage.NewDenseStrategy())

    // Use SharedStrategy for components where many entities share the same values
    // (e.g., all zombies share the same base stats)
    world.RegisterComponent("GameStats", ecsstorage.NewSharedStrategy())

    scheduler, _ := ecs.NewScheduler(world)

    scheduler.Builder().WithInstrumentation(ecs.InstrumentationConfig{
        EnableTrace: true,
        Observation: ecs.ObservationSettings{EnableStructuredLogging: true},
    })

    scheduler.RegisterWorkGroup(ecs.WorkGroupConfig{
        ID:      "update",
        Mode:    ecs.WorkGroupModeSynchronized,
        Systems: []ecs.System{game.MovementSystem{}},
    })

    // Seed an entity
    id := world.Registry().Create()
    cmds := ecs.NewCommandBuffer()
    cmds.Push(ecs.NewAddComponentCommand(id, "Position", game.Position{X: 0, Y: 0}))
    cmds.Push(ecs.NewAddComponentCommand(id, "Velocity", game.Velocity{VX: 2, VY: -1}))
    world.ApplyCommands(cmds.Drain())

    return scheduler
}
```

## 4. Run the Simulation Loop

```go
scheduler := buildScheduler()
ticker := time.NewTicker(16 * time.Millisecond)
defer ticker.Stop()

for range ticker.C {
    if err := scheduler.Tick(context.Background(), 16*time.Millisecond); err != nil {
        panic(err)
    }
}
```

Wrap the loop with `scheduler.RunWithTrace` if you want Go execution traces, or configure the observability options for logging/metrics as described in [observability.md](observability.md).

## Example Use Case: Real-Time Arena Combat

A multiplayer arena combat server can split logic into work groups:

- **Core Simulation (sync):** movement, projectile physics, damage resolution.
- **Snapshots (sync, lower frequency):** serialize state every N ticks for client reconciliation.
- **Analytics (async):** write combat stats to a datastore without blocking the main tick.
- **Observability:** structured JSON logs to Graylog, Prometheus metrics scraped by operations, SigNoz spans for slow work groups.

The scheduler’s deterministic order, resource/component access validation, and deferred command pipeline make it easier to reason about simulation state while still allowing optional async work for non-critical tasks.

## Choosing Storage Strategies

The ECS provides two storage strategies for components:

### DenseStrategy (Default)
Use for components that are unique to each entity:
- Position, velocity, rotation
- Entity-specific state
- Frequently modified data

```go
world.RegisterComponent("Position", ecsstorage.NewDenseStrategy())
```

### SharedStrategy (Memory-Efficient)
Use for components where many entities share identical values:
- Base stats for entity types (all zombies share zombie stats)
- Configuration data
- Archetype/template data

```go
world.RegisterComponent("GameStats", ecsstorage.NewSharedStrategy())

// All zombies can share the same stats instance
zombieStats := game.GameStats{Health: 50, AttackDamage: 10, Defense: 5}
for i := 0; i < 100; i++ {
    zombieID := world.Registry().Create()
    cmds.Push(ecs.NewAddComponentCommand(zombieID, "GameStats", zombieStats))
}
// Memory: 1 GameStats instance instead of 100!
```

Shared components are immutable per entity. To modify a shared component, remove and re-add with new values:

```go
cmds.Push(ecs.NewRemoveComponentCommand(entityID, "GameStats"))
cmds.Push(ecs.NewAddComponentCommand(entityID, "GameStats", upgradedStats))
```

See `docs/examples/game/shared_stats_example.go` for a complete working example.

## Additional Resources

- [Observability guide](observability.md)
- [Shared component examples](game/shared_stats_example.go)
- `docs/examples` for more patterns (contributions welcome!)
