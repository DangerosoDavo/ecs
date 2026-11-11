# Maintenance and migration

This maintenance slice preserves existing component storage and scheduler
interfaces while correcting lifecycle, concurrency and example behavior.

## Migrate async systems

Async systems get a detached world containing only the union of their work
group's declared component/resource reads. Add all required reads to descriptors.
Do not capture the live world in system fields or spawn goroutines retaining live
component values. Deep-copy-supported plain Go data is detached, including nested
maps, slices and pointers. Opaque types fail with `ErrUnsupportedSnapshot`; supply
`CloneForECS() (any, error)` only when you can guarantee a detached same-type copy.
Avoid cloning connections/locks into worker worlds. Private fields such as those
in `time.Time` require an application wrapper with an appropriate clone method.

Async `exec.Defer` now reports `ErrReadOnlyExecution`, rather than silently allowing
an undeclared mutation. Move authoritative writes to synchronized systems. Async
workers are joined at tick end and also on error; cancellation must be honored by
user systems. Snapshotting prevents races with synchronized writers, but does not
turn these workers into non-blocking database I/O.

## Stage visibility and mutation

Commands remain deferred until tick end unless a synchronized group sets
`CommandBarrier: true`. A later group then sees those applied changes; a later
system inside the same group does not. The existing single-writer-group rule is
retained. One host goroutine owns the live world. All builder configuration and
`CloseScheduler` calls occur outside systems and between ticks.

Use `NewSpawnEntityCommand` for deferred entity bundles. Existing create commands
populate their pointer target when applied, not when enqueued. Bootstrap examples
that allocate IDs immediately still work. View objects already provide `Get`;
casting a disable-filtered view to `ComponentStore` is not supported.

Use world destruction rather than direct registry destruction. Add/remove/disable
commands reject stale IDs; generation wrap retires slots. Restored-index gaps are
reusable. Restoring huge numeric indices is still inappropriate: local storage is
index-addressed and can consume memory proportional to the highest index.

The legacy dirty set remains a single-consumer API. Networking and persistence
can use separate `ChangeCursor`/`ChangesSince` cursors. Configure the fixed history
size for expected consumption gaps and treat `ErrChangeHistoryExpired` as a full
resync requirement. Destruction appears as `ChangeDestroyed`. Read/copy component
values at a barrier; the journal contains notifications, not historical snapshots.

## Improvements and limits

The opt-in `storage.NewBitmapStrategy()` skips empty index runs through occupancy
bits while retaining ascending index order. Default dense storage retains its
original small, compiler-inlinable loop and pays no bitmap metadata/update cost.
A combined hybrid loop was rejected after paired benchmarks exposed a dense
population regression caused by exceeding the Go inlining budget. Command
buffers have `Reset`, which retains capacity and clears references; `Drain`
continues to transfer ownership and cannot reuse that backing array. Scheduler
command application uses the retained buffer path. Typed storage/archetype layouts
and full save/load schemas are intentionally outside this maintenance slice.

`Query` joins from the smallest component bucket and excludes disabled state on
all joined components. `IterateOrdered` and ordered queries sort IDs explicitly.
Neither is a replay engine. Shared storage deduplicates using deep equality and
does not enforce deep immutability. Custom storage and clone implementations must
meet their contracts; arbitrary `any` values cannot provide compile-time safety.

Tests cover phase visibility, worker/live isolation, unsupported snapshots, worker
shutdown/cancellation, stale handles, spawn bundles, disabled-state cleanup,
bounded independent history, ordered joins, bitmap boundaries and example runtime
paths. The benchmark harness is unchanged between baseline and final source;
see [performance notes](performance.md). The shipping license remains an owner
decision; no license was selected during maintenance.

On the final Go 1.27.4 coverage run, the core package measured 82.1%, storage
91.3%, and executable game examples 86.1%; the aggregate profile was 83.7%.
Tests include detached heterogeneous graphs, opaque clone failures, query
termination/missing rows, spawn map capture, ordered groups and retry command
rollback. CI enforces the architecture's greater-than-80% aggregate coverage
target. The trace CLI remains a scaffold with no behavioral test coverage.
