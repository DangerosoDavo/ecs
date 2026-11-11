package ecs_test

import (
	"context"
	"testing"
	"time"

	"github.com/DangerosoDavo/ecs"
	"github.com/DangerosoDavo/ecs/ecs/storage"
)

var benchmarkSink int

func closeBenchmarkScheduler(s ecs.Scheduler) {
	// Optional interface keeps this exact harness executable on the original
	// revision, which did not expose worker shutdown.
	if closer, ok := s.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
}

// Keep these workloads unchanged when comparing maintenance revisions.
func BenchmarkDenseIteration(b *testing.B) {
	for _, sparse := range []bool{false, true} {
		name := "full_10000"
		if sparse {
			name = "sparse_100_of_10000"
		}
		b.Run(name, func(b *testing.B) {
			store := storage.NewDenseStrategy().NewStore("value")
			for i := 0; i < 10000; i++ {
				_ = store.Set(ecs.EntityIDFromParts(uint32(i), 1), i)
			}
			if sparse {
				for i := 0; i < 10000; i++ {
					if i%100 != 0 {
						store.Remove(ecs.EntityIDFromParts(uint32(i), 1))
					}
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				sum := 0
				store.Iterate(func(_ ecs.EntityID, value any) bool { sum += value.(int); return true })
				benchmarkSink = sum
			}
		})
	}
}

func BenchmarkCommandBufferReuse(b *testing.B) {
	pool := ecs.NewCommandBufferPool()
	cmd := ecs.NewCreateEntityCommand(nil)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf := pool.Get()
		for j := 0; j < 100; j++ {
			buf.Push(cmd)
		}
		pool.Put(buf)
	}
}

type benchmarkMovement struct{ id ecs.EntityID }

func (s benchmarkMovement) Descriptor() ecs.SystemDescriptor {
	return ecs.SystemDescriptor{Name: "movement", Writes: []ecs.ComponentType{"value"}}
}
func (s benchmarkMovement) Run(_ context.Context, e ecs.ExecutionContext) ecs.SystemResult {
	e.Defer(ecs.NewAddComponentCommand(s.id, "value", 42))
	return ecs.SystemResult{}
}

func BenchmarkSchedulerTick(b *testing.B) {
	w := ecs.NewWorld()
	_ = w.RegisterComponent("value", storage.NewDenseStrategy())
	id := w.Registry().Create()
	s, _ := ecs.NewScheduler(w)
	b.Cleanup(func() { closeBenchmarkScheduler(s) })
	_, _ = s.RegisterWorkGroup(ecs.WorkGroupConfig{ID: "sim", Systems: []ecs.System{benchmarkMovement{id}}})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := s.Tick(context.Background(), time.Millisecond); err != nil {
			b.Fatal(err)
		}
	}
}

type benchmarkReader struct{}

func (benchmarkReader) Descriptor() ecs.SystemDescriptor {
	return ecs.SystemDescriptor{Name: "reader", Reads: []ecs.ComponentType{"value"}, AsyncAllowed: true}
}
func (benchmarkReader) Run(_ context.Context, e ecs.ExecutionContext) ecs.SystemResult {
	v, err := e.World().ViewComponent("value")
	if err != nil {
		return ecs.SystemResult{Err: err}
	}
	sum := 0
	v.Iterate(func(_ ecs.EntityID, value any) bool { sum += value.(int); return true })
	benchmarkSink = sum
	return ecs.SystemResult{}
}

func BenchmarkAsyncRead1000(b *testing.B) {
	w := ecs.NewWorld()
	_ = w.RegisterComponent("value", storage.NewDenseStrategy())
	for i := 0; i < 1000; i++ {
		id := w.Registry().Create()
		_ = w.ApplyCommands([]ecs.Command{ecs.NewAddComponentCommand(id, "value", i)})
	}
	s, _ := ecs.NewScheduler(w)
	b.Cleanup(func() { closeBenchmarkScheduler(s) })
	s.Builder().WithAsyncWorkers(1)
	_, _ = s.RegisterWorkGroup(ecs.WorkGroupConfig{ID: "read", Mode: ecs.WorkGroupModeAsync, Systems: []ecs.System{benchmarkReader{}}})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := s.Tick(context.Background(), time.Millisecond); err != nil {
			b.Fatal(err)
		}
	}
}
