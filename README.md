# ECS Scheduler

An embeddable Go ECS scheduler for authoritative game servers. It provides ordered work groups, deferred commands, generation-checked local entity handles, dense and shared component stores, optional isolated worker execution, and observability hooks.

Requires Go 1.27.4 or newer for the patched standard library. This is an evolving library: validate your workload and read [maintenance and migration notes](docs/maintenance.md) before adopting it. A shipping license remains undecided; the project owner must choose it.

## Start here

```sh
go get github.com/DangerosoDavo/ecs
go test ./...
```

The [executable movement example](example_test.go) creates an entity with position and velocity, runs the scheduler, and asserts its resulting position. The [game examples](docs/examples/game/) demonstrate shared definitions and independent mutable state; CI executes their bootstrap paths.

```go
world := ecs.NewWorld()
if err := world.RegisterComponent("Health", storage.NewDenseStrategy()); err != nil {
    panic(err)
}
var ship ecs.EntityID
err := world.ApplyCommands([]ecs.Command{
    ecs.NewSpawnEntityCommand(&ship, map[ecs.ComponentType]any{"Health": 100}),
})
if err != nil {
    panic(err)
}
```

Here `ecs` is `github.com/DangerosoDavo/ecs`, and `storage` is `github.com/DangerosoDavo/ecs/ecs/storage`. `NewSpawnEntityCommand` allocates the ID and attaches the complete bundle together; its target is assigned only on success. `NewCreateEntityCommand` remains available, but copying its target into follow-on commands before application copies a zero ID. For immediate bootstrap work, allocate through `world.Registry().Create()` before constructing component commands.

## Simulation ownership and barriers

One host goroutine owns mutable world state. Do not access live stores, component pointers, resources, or the registry concurrently with a tick. Thread-safe registry/resource metadata does not make arbitrary component graphs thread-safe. Scheduler ticks reject concurrent or recursive invocation. Builder configuration and shutdown are host operations between ticks; do not invoke them from a system.

Synchronized systems execute in registered group order (or `WithSyncOrder`) and system slice order. Deferred commands normally become visible at tick end. Set `WorkGroupConfig.CommandBarrier: true` to apply pending synchronized commands after that group, before later groups or snapshots. Systems in the same group still see pre-command state; split stages into groups when visibility matters. Existing exclusive writer ownership across groups is retained; several writers may execute serially inside a single synchronized group.

`ApplyCommands` is ordered and stops at the first error. It is not a database transaction: earlier successful commands remain applied. Retry rewinds deferred commands from the failed attempt, but cannot undo direct pointer mutations or external effects. Use deferred writes and idempotent side effects when enabling retries.

## Worker execution

Async groups are read-only consumers. Before dispatch, the simulation owner copies their declared component and resource reads into a private world. Reads from undeclared component buckets fail. Worker mutations of this private world cannot affect live state, and `exec.Defer` reports `ErrReadOnlyExecution`. This isolation handles read/write overlap with synchronized groups without racing live stores.

Plain scalars, exported structs, arrays, pointers, slices, maps, and cyclic pointer graphs can be copied. Opaque/private-field types, channels, functions, and unsafe handles require an explicit `SnapshotCloner` implementation or fail with `ErrUnsupportedSnapshot`. A custom clone must return the same type and detach all mutable state; it is a trusted contract. A snapshot is taken when the group is encountered, after any preceding command barrier. Do not capture the live world in a worker's system fields or access it from another goroutine.

Tick joins all submitted workers even on error. These workers do not provide non-blocking database or network I/O: a noncooperative system can stall its tick. Prefer synchronized simulation and hand bounded immutable messages to host-owned I/O workers. Snapshot copying has a measured cost; see [benchmark results](docs/performance.md). Call `ecs.CloseScheduler(scheduler)` after the final tick to release worker goroutines.

## Storage and joins

- Dense storage uses index-addressed slots and a simple contiguous iteration loop that the compiler can inline. Get/Set are indexed lookups. Memory remains proportional to the highest local entity index; avoid restoring huge externally supplied IDs.
- `storage.NewBitmapStrategy()` is an explicit alternative for sparse component buckets. It reuses indexed slots and tracks occupancy words to skip empty runs while retaining ascending index order. Default dense stores pay no bitmap allocation/update cost. Bitmap iteration may cost more for full populations; select by measured workload.
- Shared storage deduplicates values by deep equality and reference counts. Keep definitions immutable, including nested pointers/maps/slices. Its normal iteration uses Go map order; replacing one entity's value does not mutate other entities' definitions.
- Component keys are strings and payloads use `any`; callers must agree on concrete types. No claim of compile-time component type safety or zero-allocation arbitrary workloads is made.
- `ecs.Query(world, types, ordered, callback)` joins from the smallest component bucket. Disabled components are excluded on every side. Callback values are reused; copy the slice before retaining it.
- `ecs.IterateOrdered(view, callback)` sorts handles by index/generation. Use ordered iteration, fixed delta time, deterministic inputs and seeded randomness wherever result order matters. Scheduler ordering alone does not guarantee replay determinism or cross-platform floating-point equality.

## Lifecycle and change consumers

Use `World.DestroyEntity` or its deferred command, rather than destroying registry slots directly. Both clean components and disabled metadata and emit a deletion tombstone. Commands reject stale/unallocated handles. Handles are local index/generation pairs; they are not persistent globally unique universe IDs. `CreateWithID` supports controlled restoration but is not a save format or migration system.

The legacy `Changed`/`DrainChanged` dirty set remains available for a single consumer. Independent networking and persistence consumers should hold their own cursor:

```go
cursor := world.ChangeCursor() // begin at the current revision
// ... run simulation ...
batch, err := world.ChangesSince(cursor)
if errors.Is(err, ecs.ErrChangeHistoryExpired) {
    // Build a complete snapshot at a simulation barrier, then restart cursor.
} else if err != nil {
    return err
} else {
    // Consume batch.Changes; ChangeDestroyed is an entity deletion tombstone.
    // Read/copy values at this barrier before handing work to another goroutine.
    cursor = batch.Cursor // advance only after successfully consuming the batch
}
```

History is bounded per world (default 1,024 notifications; configurable with `WithChangeHistoryCapacity`). Slow/dormant consumers cannot retain unbounded history. Overflow explicitly requires a full resync. Notifications contain handles and kinds, not historical component payloads; repeated updates may occur. Direct pointer mutations must call `MarkChanged`. Use domain events for durable economic actions.

## Validation and profiling

```sh
go test ./...
go vet ./...
CGO_ENABLED=1 go test -race ./...
go test -run '^$' -bench . -benchmem -count=3 ./...
go test -run '^$' -fuzz '^FuzzEntityLifecycle$' -fuzztime=10s .
```

CI uses the Go version declared by `go.mod`, verifies formatting, runs tests/vet/race detection, and uploads coverage and benchmark artifacts. Performance measurements and limits are recorded in [docs/performance.md](docs/performance.md). Observability configuration is described in [docs/examples/observability.md](docs/examples/observability.md).

## License

TBD. No license choice has been made by the maintenance work.
