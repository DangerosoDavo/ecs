# ECS Scheduler

A modular Entity-Component-System (ECS) scheduler for Go gameserver workloads. The project targets deterministic synchronized execution with optional asynchronous offloading, rich observability, and composable storage backends so it can embed cleanly into diverse projects.

## Project Status
Stage 0 (Foundations & Planning). APIs, coding standards, and project scaffolding are being finalized before the core scheduler implementation lands.

## Goals
- Deterministic tick loop with configurable synchronized work-group ordering.
- Non-blocking execution model: each work group gets a chance to run every tick.
- Pluggable component storage backends and system registration APIs.
- Built-in trace, profiling, metrics, and logging hooks compatible with Go tooling.
- High-quality automated tests, race detection, and benchmarks suitable for CI pipelines.

## Repository Layout
```
.
├── agents.md                  # Architecture overview
├── implementation_plan.md     # Detailed staged delivery plan
├── docs/                      # ADRs and supplementary documentation
├── ecs/                       # Go packages (scheduler, storage, system, trace)
├── tests/                     # Cross-package integration and benchmark suites
└── README.md
```

## Quick Start
> ⚠️ Implementation work has not started. Interfaces and module layout may change during Stage 0.

When the MVP is complete, the scheduler will expose builder APIs so you can register systems, configure work groups, and drive ticks inside your game loop. Follow the implementation plan for progress and upcoming milestones.

## Contributing
See [CONTRIBUTING.md](CONTRIBUTING.md) for coding standards, testing expectations, and the contribution process.

## License
TBD.

## Observability

The scheduler emits a detailed `WorkGroupSummary` after each work-group execution. Configure structured logging, Prometheus, or SigNoz exporters via `InstrumentationConfig.Observation`. See [docs/examples/observability.md](docs/examples/observability.md) for integration samples.


## Getting Started

- Integrate into your project: [docs/examples/integration.md](docs/examples/integration.md)
- Observability examples: [docs/examples/observability.md](docs/examples/observability.md)
