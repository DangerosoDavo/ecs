# ECS Scheduler Architecture

## Overview
- Build a reusable ECS (Entity-Component-System) scheduler for Go gameservers that can embed into diverse host projects.
- Provide deterministic synchronized execution plus optional asynchronous offloading without blocking tick progress.
- Maintain flexible registration APIs so plugins, systems, and custom component storage integrate cleanly.
- Deliver deep observability (Go trace, metrics, logging) and strong test coverage to ensure reliability under load.

## Design Goals
- Deterministic core loop with configurable work group ordering.
- Non-blocking work groups; every group receives an execution opportunity each tick.
- Safe integration into other Go projects through idiomatic Go APIs and generics.
- Extensible storage model supporting multiple component layouts.
- First-class tooling for profiling, tracing, and automated testing.

## Core Concepts
- **EntityID**: Numeric identifier managed by `EntityRegistry`; reused via generation counters to avoid stale references.
- **Component Storage**: Pluggable storage backends (dense slice, sparse map, archetype chunk) managed by `World`.
- **System**: Go interface with `Run(ctx ExecutionContext)` and metadata describing component access (read/write), frequency, and async eligibility.
- **WorkGroup**: Logical collection of systems plus configuration (mode, priority, interval, error policy).
- **Scheduler**: Orchestrates ticks, dispatches work groups, applies deferred commands, manages instrumentation.
- **Plugin**: Module implementing `Register(world *World, builder *SchedulerBuilder)` to add systems, resources, and storages during bootstrap.

## Module Responsibilities
| Package | Responsibility |
| --- | --- |
| `ecs` | Public entry types (`Scheduler`, `SchedulerBuilder`, `World`, `EntityRegistry`). |
| `ecs/system` | System definitions, metadata structs, helper decorators for component access declarations. |
| `ecs/storage` | Component storage implementations and interfaces. |
| `ecs/scheduler` | Internal scheduling logic, worker pools, tick lifecycle helpers. |
| `ecs/trace` | Instrumentation utilities integrating `runtime/trace`, metrics exporters, logging hooks. |
| `ecs/cmd/ecs-trace` | CLI for running workloads under trace/pprof. |
| `tests` | Integration and benchmark suites shared across packages. |

## Architecture Overview
1. **World Initialization**
   - Create `World`, register component storages via plugins.
   - Build scheduler using `SchedulerBuilder` with ordered synchronized group list and async worker settings.
2. **Tick Execution**
   - Apply deferred entity/component commands from previous tick.
   - Execute synchronized groups sequentially in configured order.
   - Dispatch async groups to worker pool; collect futures for barrier synchronization.
   - Await async completions (with timeout) and merge their deferred commands.
   - Run post-tick maintenance (cleanup, event emission).
3. **Lifecycle Hooks**
   - Optional `PreTick`, `PostTick`, `Maintenance` callbacks for host integrations (network sync, persistence, analytics).

## Execution Flow (Pseudocode)
```go
func (s *Scheduler) Tick(ctx context.Context, dt time.Duration) error {
    s.applyDeferred()

    // Synchronized groups
    for _, group := range s.syncOrder {
        result := group.Run(ctx, s.execContext(dt))
        s.logResult(group, result)
    }

    // Async groups
    handles := s.dispatchAsync(ctx, dt)

    if err := s.awaitAsync(handles); err != nil {
        s.handleAsyncError(err)
    }

    s.flushDeferred()
    s.maintenance()
    return nil
}
```

## Concurrency & Safety
- **Modes**: `WorkGroupModeSynchronized` (in-order execution) vs `WorkGroupModeAsync` (dispatched to worker pool).
- **Worker Pool**: Configurable size (default `runtime.NumCPU()`); tasks submitted via buffered channels; supports per-group queue depth limits.
- **Deferred Commands**: All mutations (entity create/destroy, component add/remove, resource updates) queued and applied in `applyDeferred`/`flushDeferred` stages to avoid data races. Command buffers support snapshot/restore so retries cannot leak partial writes.
- **Access Declarations**: Systems declare read/write sets; the scheduler enforces per-system uniqueness, blocks async groups from mutating components unless explicitly allowed, and guarantees a single work group owns each writable component.
- **Async Guardrails**: Async work groups require `SystemDescriptor.AsyncAllowed` and are limited to read-only component access today; conflicting writers across groups are rejected during registration.
- **Error Handling**: Work results typed (`Executed`, `SkippedEmpty`, `Errored`). Error policies include abort, continue, and single retry with buffer rewind for transient failures.
- **Determinism**: Synchronized groups ensure consistent ordering; async groups remain non-blocking and must reconcile via deferred commands.

## Profiling & Observability
- `Scheduler.RunWithTrace(ctx, writer)` wraps ticks with `runtime/trace` start/stop, enabling `go tool trace` visualization.
- Each group/system region annotated using `trace.WithRegion` and `trace.Log` for fine-grained timing.
- Optional profiling build tag enabling `pprof` HTTP handlers and `expvar` metrics.
- Structured logging interface; default adapter to `log/slog`, pluggable for custom loggers.
- Metrics exporters (tick duration histograms, queue depths, skipped group counts) exposed via interfaces for Prometheus or StatsD.
- `SchedulerObserver` hook publishes per-work-group summaries (duration, component/resource access, async flag, error state) so hosts can record metrics or visualize contention.
- Built-in observation settings cover JSON/key-value logging (Graylog friendly), Prometheus collectors, and SigNoz/OpenTelemetry exporters via configurable observers.
- Observability guide (`docs/examples/observability.md`) shows end-to-end wiring for structured logs, Prometheus scraping, and SigNoz span export.

## Testing Strategy
- **Unit Tests**: Table-driven tests for scheduler ordering, deferred command correctness, entity lifecycle, plugin registration.
- **Race Tests**: `go test -race ./...` integrated into CI to catch concurrency issues, especially async group interactions.
- **Benchmarks**: `testing.B` benchmarks comparing synchronized vs async throughput across entity counts; record baseline numbers.
- **Trace Regression**: Automated trace capture for representative workloads, stored as artifacts for regression analysis.
- **Coverage Target**: Maintain >80% coverage; measure with `go test ./... -coverprofile=coverage.out` and gate in CI.

## Implementation Roadmap
1. **MVP (Phase 1)**: Core types, single-threaded scheduler, deferred command pipeline, basic logging, unit tests.
2. **Phase 2**: Async worker pool, access validation, race-tested coverage, error handling policies.
3. **Phase 3**: Instrumentation (trace, metrics, logging adapters), CLI tracing tool, observability docs.
4. **Phase 4**: Advanced storage backends, snapshot serialization, plugin examples, performance tuning.

## Staged Implementation Plan
### Stage 0: Foundations & Planning
- **Objectives**: Confirm requirements, lock API surface sketches, choose repo layout, scaffold Go module.
- **Key Tasks**: Draft Go interfaces/types, spike simple world + scheduler skeleton, document coding standards, define CI pipeline expectations.
- **Deliverables**: ADR/architecture notes, initial `go.mod`, empty package scaffolding, backlog of tasks per package.
- **Exit Criteria**: API review completed, project skeleton builds (`go build ./...`), CI workflow blueprint ready.

### Stage 1: Core MVP Scheduler
- **Objectives**: Deliver deterministic single-thread scheduler with deferred command processing and minimal observability.
- **Key Tasks**: Implement `EntityRegistry`, `World`, storage interfaces, synchronized `Scheduler` loop, `WorkGroup` registration, deferred command buffers.
- **Tests**: Unit tests for entity lifecycles, component storage, scheduler ordering; smoke test demonstrating two groups.
- **Deliverables**: Working MVP, documentation updates in `agents.md`, Stage 1 demo sample.
- **Exit Criteria**: `go test ./...` passes with coverage report produced; deterministic tick verified via integration test.

### Stage 2: Async Execution & Safety
- **Objectives**: Introduce async work groups and worker pool while preserving deterministic synchronized path.
- **Key Tasks**: Build worker pool, dispatch logic, async handles/barrier, component access validation (per-system and cross-group), error policy hooks, and ownership tracking for writable components.
- **Tests**: `-race` test run, stress harness simulating mixed sync/async workloads, property tests for deferred command integrity.
- **Deliverables**: Configurable async scheduler variant, docs explaining usage, metrics for queue depth.
- **Exit Criteria**: Async workloads complete without race detector warnings; throughput benchmarks captured.

### Stage 3: Observability & Tooling
- **Objectives**: Add tracing, profiling, logging, metrics integrations, and developer tooling.
- **Key Tasks**: Implement `RunWithTrace`, `trace.WithRegion` wrappers, metrics exporters, structured logging adapters, `ecs-trace` CLI.
- **Key Tasks**: Implement `SchedulerObserver` sinks that translate work-group summaries into metrics/trace events; expose default Prometheus-friendly collector and SigNoz/Graylog logging adapters.
- **Tests**: Automated trace capture test, logging/metrics snapshot tests, CLI integration test.
- **Deliverables**: Instrumented scheduler, CLI tooling, documentation for profiling workflows.
- **Exit Criteria**: `go tool trace` outputs validated, metrics scraped in sample app, docs published.

### Stage 4: Advanced Storage & Ecosystem
- **Objectives**: Expand storage options, snapshotting, plugin ecosystem examples, performance tuning.
- **Key Tasks**: Implement alternate storages (archetype/dense), snapshot serialization, plugin samples, optimization passes, benchmark suite.
- **Tests**: Benchmark regression pipeline, snapshot round-trip tests, plugin integration tests.
- **Deliverables**: Extended storage module, snapshot APIs, example plugins, benchmark reports.
- **Exit Criteria**: Benchmarks show targeted performance, snapshot format documented, final release notes drafted.
- **What This Adds**: Unlocks high-performance data layouts tailored to specific workloads, enables save/restore and rollback flows, and supplies reusable plugin patterns to accelerate downstream adoption.
- **Component Storage Explained**: The storage layer controls how component data is organized in memory (dense slices for hot components, sparse maps for rare ones, archetype chunks for cache-friendly iteration). Stage 4 provides these interchangeable backends so teams can match storage strategy to gameplay data without rewriting systems.

### Stage 5: Hardening & Release
- **Objectives**: Stabilize API, polish documentation, prepare production release.
- **Key Tasks**: Address backlog issues, finalize API docs, produce migration guides, audit tests/coverage, set semantic versioning.
- **Tests**: Full CI matrix (unit, race, benchmarks, trace), load test on reference project.
- **Deliverables**: v1.0.0 release candidate, change log, deployment checklist, support plan.
- **Exit Criteria**: Stakeholder sign-off, release artifacts tagged, maintenance roadmap established.

## Open Questions & Risks
- Component access declaration ergonomics: reflect-based vs code-generated wrappers for zero-cost runtime checks.
- Serialization format and versioning for snapshots (e.g., gob, JSON, custom binary).
- Determinism guarantees when mixing async groups that modify shared resources—may require additional validation or restrictions.
- Hosting environments: need to confirm integration requirements (e.g., cloud functions, container lifecycle).
- Tooling expectations for downstream users (code generation, hot-reload support?).

## Next Steps
- Finalize API drafts (`Scheduler`, `WorkGroup`, `System` interfaces) and confirm naming conventions.
- Prototype MVP scheduler and run smoke tests with simple systems.
- Define CI pipeline (linting, tests, race, coverage, benchmarks).
- Draft developer guides and examples explaining synchronized vs async configuration.
