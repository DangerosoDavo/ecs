# ADR 0001: ECS Scheduler API Sketch

## Status
Proposed

## Context
Stage 0 requires agreement on the public-facing API for the ECS scheduler so downstream packages can plan integrations. The API must support:
- Configurable work groups that run synchronously or asynchronously per tick.
- Pluggable component storage strategies.
- System registration with explicit component access metadata.
- Deferred command pipelines for safe mutations.
- Observability hooks for tracing and metrics.

## Decision
Adopt the following initial interfaces and types (subject to refinement as implementation progresses):

```go
package ecs

type Scheduler interface {
    Tick(ctx context.Context, dt time.Duration) error
    Run(ctx context.Context, steps int, dt time.Duration) error
    RunWithTrace(ctx context.Context, w io.Writer, fn func() error) error
    RegisterWorkGroup(cfg WorkGroupConfig) (WorkGroupHandle, error)
    Builder() SchedulerBuilder
}

type SchedulerBuilder interface {
    WithSyncOrder(order []WorkGroupID) SchedulerBuilder
    WithAsyncWorkers(count int) SchedulerBuilder
    WithErrorPolicy(WorkGroupID, ErrorPolicy) SchedulerBuilder
    WithInstrumentation(InstrumentationConfig) SchedulerBuilder
    Build(world *World) (Scheduler, error)
}

type WorkGroupConfig struct {
    ID          WorkGroupID
    Mode        WorkGroupMode
    Systems     []System
    Interval    TickInterval
    ErrorPolicy ErrorPolicy
    Priority    int
}

type WorkGroupMode uint8

const (
    WorkGroupModeSynchronized WorkGroupMode = iota
    WorkGroupModeAsync
)

type System interface {
    Descriptor() SystemDescriptor
    Run(ctx ExecutionContext) SystemResult
}

type SystemDescriptor struct {
    Name         string
    Reads        []ComponentType
    Writes       []ComponentType
    Resources    []ResourceAccess
    Tags         []string
    RunEvery     TickInterval
    AsyncAllowed bool
}

type ExecutionContext interface {
    World() *World
    TimeDelta() time.Duration
    TickIndex() uint64
    Logger() Logger
    Tracer() trace.Tracer
    Defer(cmd Command)
}

type World struct {
    registry *EntityRegistry
    storage  StorageProvider
    resources ResourceContainer
}

type StorageProvider interface {
    RegisterComponent(ComponentType, StorageStrategy) error
    View(ComponentType) (ComponentView, error)
    Apply(commands []Command) error
}
```

Supporting types (`EntityID`, `ComponentType`, `Command`, etc.) will be defined alongside these structures. The `System` abstraction remains minimal to enable both functional and struct-based implementations.

## Consequences
- Interfaces serve as compilation targets for scaffolding and tests created in Stage 1.
- Builders encapsulate configuration complexity, keeping scheduler initialization flexible.
- Deferred command routing is explicit via `ExecutionContext.Defer`, ensuring async systems remain safe.

## Follow-up
- Refine error handling semantics (return vs panic) during Stage 1 implementation.
- Validate whether `Run` should accept a `dt` override per system.
- Explore code generation or generics to reduce boilerplate in component access declarations.
