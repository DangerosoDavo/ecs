# Integration

Start with the runnable, output-checked [movement example](../../example_test.go) and [README](../../README.md).

1. Construct `ecs.NewWorld()` and register component stores.
2. Bootstrap through `NewSpawnEntityCommand` bundles, checking errors.
3. Construct a scheduler and register synchronized work groups with explicit read/write descriptors.
4. Drive `Tick(context, fixedDelta)` from one owner goroutine. Tick does not choose the wall-clock pacing policy; the host must handle overload without silently changing the simulation step.
5. Use `CommandBarrier` between stages requiring deferred-write visibility. Work groups without it retain end-of-tick application.
6. Copy networking/persistence payloads at a simulation barrier and dispatch them through bounded host queues.
7. Call `ecs.CloseScheduler` during shutdown after the last tick.

A component view already supports `Get`; do not cast it to `ComponentStore`. `World.ViewComponent` intentionally returns a disable-filtered read view. Mutate through deferred commands or owner-only pointers with `MarkChanged`.

Async groups receive private copied reads, cannot defer commands, and are joined before Tick returns. Unsupported opaque data requires `SnapshotCloner`. See [maintenance notes](../maintenance.md) for migration and [performance results](../performance.md) for costs.

[Shared component examples](game/) allocate bootstrap IDs immediately before capturing them in component commands. Runtime tests execute these examples, while the canonical movement example asserts entity count and position.
