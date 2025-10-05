# Stage 1 Backlog (Core MVP Scheduler)

This backlog breaks Stage 1 into discrete tasks with ownership placeholders and estimated effort. Update as work progresses.

## Epic S1-E1: Entity & Component Foundations
- **S1-T1**: Implement `EntityID` struct with generation counters (2 pts).
- **S1-T2**: Build `EntityRegistry` with allocate/recycle support and tests (3 pts).
- **S1-T3**: Define base `ComponentStorage` interface and dense slice implementation (3 pts).
- **S1-T4**: Integrate storage registration into `World`, with lookup helpers (2 pts).

## Epic S1-E2: Deferred Command Pipeline
- **S1-T5**: Specify command types (create, destroy, add component, remove component) (1 pt).
- **S1-T6**: Implement deferred command buffer with apply/flush mechanics (3 pts).
- **S1-T7**: Add unit tests verifying command ordering and idempotency (2 pts).

## Epic S1-E3: Scheduler Core Loop
- **S1-T8**: Implement `SchedulerBuilder` with synchronized group configuration (2 pts).
- **S1-T9**: Implement `Scheduler.Tick` with deterministic work group iteration (4 pts).
- **S1-T10**: Provide execution context plumbing (World reference, tick metadata, logger hooks) (2 pts).
- **S1-T11**: Record work results (executed/skipped/error) and basic logging instrumentation (2 pts).

## Epic S1-E4: Testing & Examples
- **S1-T12**: Write table-driven tests covering scheduling order and interval handling (2 pts).
- **S1-T13**: Add smoke-test example with two work groups and dummy systems (1 pt).
- **S1-T14**: Ensure `go test ./... -cover` >= 70% (verification task) (1 pt).

## Cross-Cutting
- **S1-T15**: Update documentation (`agents.md`, `implementation_plan.md`) with Stage 1 implementation notes (1 pt).
- **S1-T16**: Create tracking issues in GitHub and link to this backlog (1 pt).

> Points use a simple Fibonacci scale (1,2,3,5,8). Adjust as we learn more.
