# Performance measurements

The unchanged `maintenance_benchmark_test.go` harness exercises full and sparse
iteration, command buffer reuse, a one-command synchronized tick, and a read-only
1,000-entity worker tick. Use the same Go version, machine, GOMAXPROCS, workload
and benchmark duration for both revisions. Run baseline and final sequentially
after builds/race tests finish; simultaneous host workloads can overwhelm small
timing differences. Record ns/op, B/op and allocs/op, and preserve raw samples.

```sh
go test -run '^$' -bench 'Benchmark(DenseIteration|CommandBufferReuse|SchedulerTick|AsyncRead1000)$' -benchmem -count=3 -benchtime=300ms .
```

The separate `bitmap_benchmark_test.go` repeats exactly the dense benchmark
populations/callback with the explicitly selected new bitmap strategy. Its
results compare storage choices; they are not claimed as a default-strategy
speedup. Default Dense iteration keeps its original small loop: an attempted
hybrid dispatch exceeded the compiler inlining budget and regressed full
populations, so that approach was removed. Default stores have no bitmap
metadata/update overhead.

Preliminary local checks show the expected mechanisms: opt-in occupancy bits
avoid scanning thousands of absent components; pooled Reset removes command-slice
allocation during reuse; synchronized ticks retain their command capacity.
Independent change journaling adds a small fixed steady-state cost and bounded
history memory. Async isolation deliberately copies component and registry data,
increasing latency/allocations compared with the earlier unsafe shared-pointer
implementation. Keep authoritative hot loops synchronized and send immutable
payloads to external I/O workers for high-frequency persistence/networking.

## Recorded samples

Measured on 11 November 2026 using Go 1.27.4, Windows amd64, Intel i7-6700K,
GOMAXPROCS=1, -cpu=1, five sequential samples and 300 ms per benchmark. Baseline
is commit 3fec465884e56f3c9852bfdd63e1a0505d9a7196 with the unchanged maintenance
harness. Values are medians; raw before/after samples are in [benchmarks/](benchmarks/).

| Workload | Before ns/op | After ns/op | Before B/op / allocs | After B/op / allocs |
|---|---:|---:|---:|---:|
| Default Dense, 10000 occupied | 17697 | 13635 | 0 / 0 | 0 / 0 |
| Default Dense, 100 of 10000 occupied | 10709 | 11784 | 0 / 0 | 0 / 0 |
| Command buffer, 100 commands reused | 2897 | 287.1 | 4464 / 8 | 0 / 0 |
| Synchronized tick, one deferred update | 1547 | 3809 | 240 / 10 | 256 / 9 |
| Async read, 1000 entities | 34484 | 232736 | 696 / 17 | 87185 / 59 |

The new opt-in Bitmap strategy measured 18321 ns for 10000 occupied
slots and 327.4 ns for 100 occupied of 10000, both zero allocations.
The sparse case is about 33x faster than original Dense on this fixture; its
fully populated traversal costs more than maintained Dense. It is an explicit
storage choice, not a universal default optimization. Default Dense iteration
retains its original implementation; no default-iteration speedup is claimed.

Command-buffer reuse is about 10x faster on this fixture, removing eight
allocations / 4464 bytes per operation. Safe async execution is deliberately
more expensive: copied registry/component state and cancellable worker lifetime
replace unsafe shared live pointers. Keep hot authoritative simulation synchronized.
Synchronized tick timing varies from 1042 to 4506 ns across these samples;
independent change journaling also adds a small fixed byte cost. Do not treat
noisy small timing differences as a proven improvement or capacity estimate.

No broad game-server throughput, whole-world memory reduction, or production
scalability claim follows from these microbenchmarks.

Additional profiling remains necessary for module-heavy entities, projectiles,
shared-definition diversity, lifecycle churn, highest-index memory retention,
large snapshots, network encoding, and persistence backpressure. Query helpers
and snapshot cloning prioritize correct contracts; typed/packed storage and
specialized snapshots should be justified by actual host profiles.
