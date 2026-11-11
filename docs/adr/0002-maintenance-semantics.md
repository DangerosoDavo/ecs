# ADR 0002: Simulation ownership, copied workers and explicit command barriers

Status: Accepted for the current maintenance implementation, 2026-11-11.

The initial scheduler exposed one mutable world to synchronized systems and
workers concurrently. Component read/write declarations did not prevent mutable
pointer access, and dense stores were not synchronized. Worker I/O also blocked
tick completion despite earlier descriptions of non-blocking execution.

The live world has one host owner. Async groups now receive private snapshots
of their declared reads, copied on that owner before dispatch. Unsupported
opaque data fails explicitly unless it implements `SnapshotCloner`. Async
deferred mutations are errors. The scheduler joins every dispatched worker even
when another group fails. Configuring or closing a scheduler is a host operation
between ticks; concurrent/recursive ticks fail rather than racing.

This intentionally tightens async semantics while preserving the `System` and
`ExecutionContext` interfaces. It costs copying time and memory, so synchronous
simulation remains the default recommendation. Truly independent I/O uses
host-owned workers receiving immutable messages, rather than live world access.
Descriptor declarations are useful dependency metadata, not a security sandbox:
a system must not capture the live world or retain mutable values across ticks.

The existing end-of-tick deferred visibility remains the default. A synchronized
group's `CommandBarrier` optionally applies pending commands before the next
group. A barrier is not rollback or a database transaction. Existing exclusive
writer ownership between groups remains for API compatibility; read/write
overlap in serial synchronized groups is safe, while async readers are isolated.

Local handles retain index/generation identity, with explicit allocated-slot
tracking and exhausted-generation retirement. Destroying through the world
cleans components and disabled state. A bundle spawn validates component buckets
before allocation, rolls back its own allocation on a store failure, and assigns
its target only after success. Custom stores must support correct removal for
this rollback guarantee; arbitrary external side effects cannot be undone.

A fixed-capacity world journal publishes independent sequence cursors and
deletion tombstones. Expired cursors require a complete resync; stalled consumers
cannot retain history indefinitely. Notifications do not store historical payload
values. Durable gameplay transactions and schema migrations remain host concerns.

Ordered iteration/query helpers make entity-order dependencies explicit. Fixed
steps, input ordering, seeded randomness and floating-point policy remain host
requirements; this change does not promise deterministic replay by itself.
