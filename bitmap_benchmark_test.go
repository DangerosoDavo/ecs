package ecs_test

import (
	"github.com/DangerosoDavo/ecs"
	"github.com/DangerosoDavo/ecs/ecs/storage"
	"testing"
)

// Keep populations, construction, and callback identical to DenseIteration.
// Factories remain concrete here, rather than passing an interface into a
// shared helper: that would hide compiler devirtualization/inlining differences
// in the default strategy, which are material to this comparison.
func BenchmarkBitmapIteration(b *testing.B) {
	for _, sparse := range []bool{false, true} {
		name := "full_10000"
		if sparse {
			name = "sparse_100_of_10000"
		}
		b.Run(name, func(b *testing.B) {
			store := storage.NewBitmapStrategy().NewStore("value")
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
