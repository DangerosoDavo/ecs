# ADR 0001: ECS Scheduler Architecture Baseline

## Status
Draft (Stage 0)

## Context
We are building a reusable ECS scheduler for Go gameservers. The system must support deterministic synchronized work groups, optional asynchronous offloading, deferred command processing, and rich observability. Architecture goals and roadmap are captured in `agents.md` and `implementation_plan.md`.

## Decision
- Structure the Go module as `github.com/yourorg/ecs` with top-level package `ecs` and supporting subpackages (`system`, `storage`, `scheduler`, `trace`).
- Use a builder pattern (`SchedulerBuilder`) to register work groups, configure execution order, and construct schedulers.
- Represent work groups with metadata capturing mode (synchronized/async), component access intents, execution intervals, and error policies.
- Provide pluggable component storage backends managed through the `World` type, enabling dense, sparse, and archetype layouts.
- Expose observability hooks compatible with Go tooling (`runtime/trace`, `pprof`, logging interfaces, metrics exporters).

## Consequences
- Interfaces defined during Stage 0 will guide implementation in later stages; breaking changes after Stage 1 require ADR updates.
- The builder pattern centralizes configuration but requires careful documentation to avoid misuse.
- Component storage abstraction increases flexibility but adds complexity to the world implementation and system APIs.
- Observability integration from the start ensures profiling support but requires additional maintenance (e.g., CLI tooling).

## Follow-Up
- Finalize API signatures in code during Stage 0 and circulate for review.
- Capture storage backend details in a dedicated ADR before Stage 4.
- Update this ADR to **Accepted** once Stage 0 review completes.
