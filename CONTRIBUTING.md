# Contributing Guidelines

## Development Flow
1. Discuss significant changes in an issue or design review before submitting code.
2. Work branches should target the active milestone stage (see `implementation_plan.md`).
3. Ensure commits are small, focused, and include meaningful messages.
4. Keep architecture notes in `docs/adr/` up to date when decisions are made.

## Coding Standards
- Go modules use Go 1.24. Enable modules with `GO111MODULE=on` when working outside GOPATH.
- Run `go fmt ./...` before committing. CI will reject unformatted code.
- Avoid global state; prefer dependency injection via constructors/builders.
- Document exported symbols with GoDoc comments. Include rationale for complex algorithms.
- Keep files ASCII unless otherwise required.
- Refer to `docs/coding-standards.md` for detailed guidelines covering errors, logging, tracing, concurrency, generics, and testing.

## Testing Expectations
- Add or update unit tests alongside code changes.
- Run the full suite before opening a PR: `go test ./...`.
- For concurrency changes, also run `go test -race ./...`.
- Update or add benchmarks when touching performance-sensitive paths.
- Maintain coverage above the threshold defined per stage in `implementation_plan.md`.

## Tooling Targets (to be automated in CI)
- `go fmt ./...`
- `go vet ./...`
- `go test ./...`
- `go test -race ./...`
- `go test -bench=. ./...` (for performance work)

## Code Review
- All changes require review by at least one maintainer.
- Prioritize clarity, correctness, and testability. Point out risks or missing tests.
- Address reviewer feedback promptly; use follow-up issues for large follow-on work.

## Reporting Issues
- Include reproduction steps, expected vs actual behavior, and environment details.
- Label issues by stage (`stage-1`, `stage-2`, etc.) to align with the roadmap.

## License / CLA
- License terms are TBD. Contributors may be asked to agree to a Contributor License Agreement when the license is finalized.
