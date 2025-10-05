# CI Plan

## Pipeline Overview
1. **Lint**
   - `go fmt ./...`
   - `go vet ./...`
   - `staticcheck ./...` (when introduced)
2. **Unit Tests**
   - `go test ./...`
   - Collect coverage: `go test ./... -coverprofile=coverage.out`
3. **Race Detection** (Stage 2+)
   - `go test -race ./...`
4. **Benchmarks** (Stage 2+)
   - `go test -bench=. ./...`
   - Capture benchmark output artifact
5. **Trace Regression** (Stage 3+)
   - Run trace harness, store `.trace` files for inspection
6. **Release Checks** (Stage 5)
   - `go test ./... -tags release`
   - `golangci-lint run`

## Environments
- Go 1.24.x (minimum supported version)
- Linux amd64 runner; additional OS targets evaluated during Stage 5

## Reporting
- Coverage threshold: 70% (Stage 1), 80% (Stage 2+), 85% (Stage 4+)
- Benchmark trends tracked via historical artifacts (GitHub Actions or alternative)
- Failing stage blocks subsequent stages

## Future Enhancements
- Add nightly soak tests once integration workloads exist
- Evaluate Bazel or Taskfile integration if the build grows complex
