# Coding Standards

These guidelines complement the high-level contribution rules in `CONTRIBUTING.md` and set expectations for code written in the ECS scheduler project.

## Go Style
- Follow standard Go formatting (`gofmt`) and linting rules (`go vet`, staticcheck once enabled).
- Prefer small, focused packages with clear ownership boundaries (e.g., `ecs/scheduler`, `ecs/storage`).
- Exported identifiers require GoDoc comments that describe *why* the construct exists, not just *what* it is.

## Error Handling
- Return wrapped errors using `fmt.Errorf("context: %w", err)` for additional context.
- Avoid panics in production paths; propagate errors up to the scheduler or host application for handling.
- For temporary failures (e.g., async worker overload), prefer sentinel errors or typed errors that the caller can inspect.

## Logging & Tracing
- Use the `Logger` abstraction provided by the scheduler. Include work group and system identifiers in every log entry.
- Enclose trace regions with `Tracer.Start` / `TraceSpan.End`; ensure spans close in deferred statements to avoid leaks.
- Avoid logging secrets or personally identifiable information.

## Concurrency
- Access shared data structures through the deferred command pipeline or scheduler-managed synchronization primitives.
- Favor immutability for configuration structs; pass them by value when values are small.
- When using channels, document ownership semantics (who closes the channel and when).

## Generics & Interfaces
- Use generics for component storage helpers where they materially reduce boilerplate without obscuring readability.
- Keep public interfaces narrow; prefer multiple small interfaces over a single large one.
- Avoid exposing concrete struct fields directly; use constructor functions or builder patterns.

## Testing
- Follow table-driven tests and keep fixtures near the test definitions.
- Use `t.Helper()` in helper functions.
- When testing concurrency, leverage `t.Parallel()` judiciously and ensure deterministic behavior.
- Benchmarks should document their setup and expected outcomes in comments; record baseline numbers in PR descriptions.

## Documentation
- Update `agents.md` and related docs when architecture or behavior changes.
- Record significant decisions in `docs/adr/` with incremental numbering.
- Keep examples in sync with the public API; ensure they compile via `go test ./...` (examples run as tests).
