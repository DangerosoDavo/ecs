# ECS Scheduler

A production-ready Entity-Component-System (ECS) scheduler for Go gameserver workloads. Features deterministic synchronized execution, optional asynchronous offloading, memory-efficient shared components, rich observability, and composable storage backends that embed cleanly into diverse projects.

## Project Status

**✅ Production Ready** - Core implementation complete with comprehensive test coverage. The scheduler, component storage systems, work group management, and observability features are fully functional and tested.

## Features

### Core ECS Architecture

- **Entity Registry**: Generation-based entity IDs with automatic recycling for stale handle detection
- **Component Storage**: Pluggable storage strategies (Dense, Shared) with type-safe component access
- **System Execution**: Deterministic work group ordering with resource conflict detection
- **Command Pipeline**: Deferred mutation system for safe entity/component modifications during system execution
- **Resource Management**: Shared resource container with read/write access control

### Scheduler Capabilities

- **Deterministic Tick Loop**: Configurable synchronized work-group ordering ensures reproducible behavior
- **Async Execution**: Optional non-blocking work groups for analytics, I/O, and non-critical tasks
- **Tick Intervals**: Systems can run every N ticks with configurable offsets
- **Error Policies**: Abort, Continue, or Retry policies per work group
- **Access Validation**: Compile-time-like validation of component/resource read/write conflicts

### Component Storage Strategies

The ECS provides two battle-tested storage strategies:

#### DenseStrategy (Default)
Traditional storage where each entity owns its component instance.

**Use for:**
- Entity-specific data (position, velocity, rotation)
- Frequently modified values (current health, cooldowns)
- Data unique to each entity

**Performance:** O(1) set/get, minimal overhead

#### SharedStrategy (Memory-Efficient)
Multiple entities reference the same component instance in memory.

**Use for:**
- Entity archetypes (all zombies share base stats)
- Configuration data (weapon stats, difficulty settings)
- Template/prototype data
- Large-scale simulations (10,000+ entities with few unique values)

**Benefits:**
- 80-99% memory reduction for shared data
- Automatic deduplication via deep equality
- Reference counting with automatic cleanup
- Stats API for monitoring efficiency

**Memory Savings Example:**
```
1,000 entities with identical stats:
  Dense:  ~40 KB (1000 × 40 bytes)
  Shared: ~8 KB (1 × 40 bytes + overhead)
  Savings: ~80%
```

### Recommended Pattern: BaseStats + CurrentStats

For game entities, use a hybrid approach:

```go
// BaseStats (Shared) - immutable archetype data
type BaseStats struct {
    MaxHealth, BaseAttack, BaseDefense int
}

// CurrentStats (Dense) - mutable per-entity state
type CurrentStats struct {
    CurrentHealth int
    IsDead bool
}

// StatModifiers (Dense) - time-limited buffs/debuffs
type StatModifiers struct {
    Modifiers []StatModifier
}
```

**Benefits:**
- ✓ Memory efficient (shared base stats)
- ✓ Mutable per-entity state (unique current stats)
- ✓ Flexible modifiers (temporary effects)
- ✓ Multiple systems can access same entity's components

See [docs/examples/game/stats_pattern_example.go](docs/examples/game/stats_pattern_example.go) for complete implementation.

### Observability

Production-grade observability built-in:

- **Structured Logging**: JSON or key-value formats via configurable logger
- **Prometheus Metrics**: Work group duration histograms, system execution counts
- **SigNoz Integration**: Distributed tracing spans for slow work groups
- **Work Group Summaries**: Detailed execution metadata after each work group

Configure via `InstrumentationConfig`:
```go
scheduler.Builder().WithInstrumentation(ecs.InstrumentationConfig{
    EnableTrace:   true,
    EnableMetrics: true,
    Observation: ecs.ObservationSettings{
        EnableStructuredLogging: true,
        LoggingFormat:          ecs.ObservationLogFormatJSON,
        EnablePrometheus:       true,
    },
})
```

### Testing & Quality

- **Comprehensive Test Suite**: 30+ tests covering all core functionality
- **Thread Safety**: Concurrent access patterns tested and validated
- **Race Detection**: Clean under `go test -race` (requires CGO)
- **Deterministic Behavior**: Reproducible tick execution for debugging

## Installation

```bash
go get github.com/DangerosoDavo/ecs@latest
```

## Quick Start

### Basic Example

```go
package main

import (
    "context"
    "time"

    "github.com/DangerosoDavo/ecs"
    ecsstorage "github.com/DangerosoDavo/ecs/ecs/storage"
)

// Define components
type Position struct {
    X, Y float64
}

type Velocity struct {
    VX, VY float64
}

// Define a system
type MovementSystem struct{}

func (MovementSystem) Descriptor() ecs.SystemDescriptor {
    return ecs.SystemDescriptor{
        Name:         "movement",
        Reads:        []ecs.ComponentType{"Velocity"},
        Writes:       []ecs.ComponentType{"Position"},
        RunEvery:     ecs.TickInterval{Every: 1},
        AsyncAllowed: false,
    }
}

func (MovementSystem) Run(ctx context.Context, exec ecs.ExecutionContext) ecs.SystemResult {
    posView, _ := exec.World().ViewComponent("Position")
    velView, _ := exec.World().ViewComponent("Velocity")

    posStore := posView.(ecs.ComponentStore)
    velView.Iterate(func(id ecs.EntityID, component any) bool {
        vel := component.(Velocity)
        if pos, ok := posStore.Get(id); ok {
            p := pos.(Position)
            p.X += vel.VX * exec.TimeDelta().Seconds()
            p.Y += vel.VY * exec.TimeDelta().Seconds()
            exec.Defer(ecs.NewAddComponentCommand(id, "Position", p))
        }
        return true
    })

    return ecs.SystemResult{}
}

func main() {
    // Create world
    world := ecs.NewWorld()
    world.RegisterComponent("Position", ecsstorage.NewDenseStrategy())
    world.RegisterComponent("Velocity", ecsstorage.NewDenseStrategy())

    // Create scheduler
    scheduler, _ := ecs.NewScheduler(world)
    scheduler.RegisterWorkGroup(ecs.WorkGroupConfig{
        ID:      "update",
        Mode:    ecs.WorkGroupModeSynchronized,
        Systems: []ecs.System{MovementSystem{}},
    })

    // Spawn an entity
    entityID := world.Registry().Create()
    cmds := ecs.NewCommandBuffer()
    cmds.Push(ecs.NewAddComponentCommand(entityID, "Position", Position{X: 0, Y: 0}))
    cmds.Push(ecs.NewAddComponentCommand(entityID, "Velocity", Velocity{VX: 2, VY: -1}))
    world.ApplyCommands(cmds.Drain())

    // Run simulation
    ticker := time.NewTicker(16 * time.Millisecond)
    defer ticker.Stop()

    for range ticker.C {
        if err := scheduler.Tick(context.Background(), 16*time.Millisecond); err != nil {
            panic(err)
        }
    }
}
```

### Shared Components Example

```go
// Define archetype base stats (shared across entity types)
type BaseStats struct {
    MaxHealth, BaseAttack, BaseDefense int
}

// Define per-entity runtime stats
type CurrentStats struct {
    CurrentHealth int
    IsDead bool
}

func main() {
    world := ecs.NewWorld()

    // BaseStats uses shared storage - memory efficient
    world.RegisterComponent("BaseStats", ecsstorage.NewSharedStrategy())

    // CurrentStats uses dense storage - unique per entity
    world.RegisterComponent("CurrentStats", ecsstorage.NewDenseStrategy())

    // Define zombie archetype
    zombieBaseStats := BaseStats{MaxHealth: 50, BaseAttack: 10, BaseDefense: 5}

    // Spawn 100 zombies - all share the SAME BaseStats instance
    cmds := ecs.NewCommandBuffer()
    for i := 0; i < 100; i++ {
        var zombieID ecs.EntityID
        cmds.Push(ecs.NewCreateEntityCommand(&zombieID))
        cmds.Push(ecs.NewAddComponentCommand(zombieID, "BaseStats", zombieBaseStats))
        cmds.Push(ecs.NewAddComponentCommand(zombieID, "CurrentStats", CurrentStats{
            CurrentHealth: zombieBaseStats.MaxHealth,
            IsDead: false,
        }))
    }
    world.ApplyCommands(cmds.Drain())

    // Memory: 1 BaseStats instance + 100 CurrentStats instances
    // vs 100 complete stat instances with dense storage
}
```

## Repository Layout

```
.
├── api.go                     # Public API and interfaces
├── world.go                   # World implementation (entity/component/resource container)
├── entity.go                  # Entity registry with generation tracking
├── commands.go                # Command system for deferred mutations
├── command_buffer.go          # Command buffer with pooling
├── scheduler_impl.go          # Scheduler implementation
├── storage_provider.go        # Component storage provider
├── resource_container.go      # Shared resource container
├── observability.go           # Observability implementations (logging, metrics, tracing)
├── worker_pool.go             # Worker pool for async execution
├── errors.go                  # Error types
├── ecs/
│   ├── storage/
│   │   ├── dense.go          # Dense storage strategy
│   │   ├── shared.go         # Shared storage strategy
│   │   └── *_test.go         # Storage tests
│   ├── cmd/ecs-trace/        # Trace analysis tool
│   └── */doc.go              # Package documentation
├── docs/
│   ├── examples/
│   │   ├── integration.md              # Integration guide
│   │   ├── shared-components.md        # Shared component patterns
│   │   ├── observability.md            # Observability setup
│   │   └── game/
│   │       ├── stats_components.go     # BaseStats + CurrentStats pattern
│   │       ├── stats_systems.go        # Example systems
│   │       ├── stats_pattern_example.go # Complete pattern examples
│   │       └── shared_stats_example.go  # Basic shared storage demo
│   └── adr/                            # Architecture Decision Records
└── *_test.go                           # Test files
```

## Documentation

### Getting Started
- [Integration Guide](docs/examples/integration.md) - Embed ECS into your project
- [Quick Start Examples](docs/examples/game/) - Working code samples

### Advanced Topics
- [Shared Components](docs/examples/shared-components.md) - Memory optimization patterns
- [Observability](docs/examples/observability.md) - Logging, metrics, and tracing
- [Architecture Decisions](docs/adr/) - Design rationale and trade-offs

### API Reference
See [api.go](api.go) for the complete public API surface.

## Use Cases

### Real-Time Game Servers
```go
// Core Simulation (sync): movement, combat, physics
// Snapshots (sync, low freq): state serialization every 10 ticks
// Analytics (async): write stats to datastore without blocking
scheduler.RegisterWorkGroup(ecs.WorkGroupConfig{
    ID:   "core_sim",
    Mode: ecs.WorkGroupModeSynchronized,
    Systems: []ecs.System{MovementSystem{}, CombatSystem{}, PhysicsSystem{}},
})

scheduler.RegisterWorkGroup(ecs.WorkGroupConfig{
    ID:       "snapshots",
    Mode:     ecs.WorkGroupModeSynchronized,
    Systems:  []ecs.System{SnapshotSystem{}},
    Interval: ecs.TickInterval{Every: 10},
})

scheduler.RegisterWorkGroup(ecs.WorkGroupConfig{
    ID:      "analytics",
    Mode:    ecs.WorkGroupModeAsync,
    Systems: []ecs.System{AnalyticsSystem{}},
})
```

### Large-Scale Simulations
Leverage shared components for memory efficiency:
```go
// 10,000 entities with 5 unique archetypes
// Memory: 5 shared BaseStats vs 10,000 dense instances
// Savings: ~99.95%
```

## Performance Characteristics

| Operation | Complexity | Notes |
|-----------|-----------|-------|
| Entity Create/Destroy | O(1) | Generation tracking prevents stale handles |
| Component Get (Dense) | O(1) | Direct array indexing |
| Component Get (Shared) | O(1) | Hash map lookup |
| Component Set (Dense) | O(1) | Direct write |
| Component Set (Shared) | O(n) | Deep equality check (n = unique values) |
| System Execution | O(entities) | Iterate all entities with component |
| Command Application | O(commands) | Sequential command processing |

**Shared Storage Performance:**
- Best for: High entity count (1000+), low unique values (10-100)
- Deduplication: O(n) where n = unique values (typically < 100)
- Memory: O(unique values) vs O(entities) for dense

## Testing

Run all tests:
```bash
go test ./...
```

Run with race detection (requires CGO):
```bash
CGO_ENABLED=1 go test -race ./...
```

Run with coverage:
```bash
go test -cover ./...
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for:
- Coding standards
- Testing expectations
- Pull request process
- Development workflow

## Goals

- ✅ Deterministic tick loop with configurable work-group ordering
- ✅ Non-blocking execution model for async work
- ✅ Pluggable component storage backends (Dense, Shared)
- ✅ Memory-efficient shared component support
- ✅ Built-in observability (logging, metrics, tracing)
- ✅ Comprehensive test suite with race detection
- ✅ Resource access validation
- ✅ Command pipeline for safe mutations
- 🚧 Benchmarking suite (planned)
- 🚧 Additional storage strategies (planned: Sparse, Hierarchical)

## License

TBD

## Examples

For complete working examples, see:
- [Basic Integration](docs/examples/integration.md)
- [Shared Component Patterns](docs/examples/game/stats_pattern_example.go)
- [Combat System with Stats](docs/examples/game/stats_systems.go)
- [Observability Setup](docs/examples/observability.md)

## Support

- Issues: [GitHub Issues](https://github.com/DangerosoDavo/ecs/issues)
- Discussions: [GitHub Discussions](https://github.com/DangerosoDavo/ecs/discussions)
