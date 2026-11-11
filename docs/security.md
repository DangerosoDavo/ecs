# Toolchain security validation

On 11 November 2026, govulncheck v1.9.0 scanned `./...` with the actual Go 1.27.4
compiler on PATH against the official vulnerability database updated at 02:51:11
UTC. It reported no known advisories at module, package or reachable-symbol level.
The [raw record](security/ecs-govulncheck-go1.27.4.json) is retained for review.

The module requires Go 1.27.4 or newer for the patched standard library. The
earlier installed Go 1.25.1 compiler had standard-library package advisories,
although none were reachable through this ECS package. There are no external
module dependencies. Compiler minimums and known-advisory scanning are part of
CI maintenance, not a guarantee of application safety or complete bug absence.

```sh
go version
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
```
