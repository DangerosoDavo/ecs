package ecs

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkerPoolExecuteJobs(t *testing.T) {
	pool := newWorkerPool(2)
	defer pool.Close()

	var count atomic.Int32
	job := func(ctx context.Context) jobResult {
		select {
		case <-time.After(5 * time.Millisecond):
			count.Add(1)
			return jobResult{}
		case <-ctx.Done():
			return jobResult{err: ctx.Err()}
		}
	}

	handles := []*jobHandle{
		pool.Submit(context.Background(), job),
		pool.Submit(context.Background(), job),
		pool.Submit(context.Background(), job),
	}

	for i, h := range handles {
		if res := h.Wait(); res.err != nil {
			t.Fatalf("job %d failed: %v", i, res.err)
		}
	}

	if count.Load() != 3 {
		t.Fatalf("expected 3 jobs to run, got %d", count.Load())
	}
}

func TestWorkerPoolClosedRejectsJobs(t *testing.T) {
	pool := newWorkerPool(1)
	pool.Close()

	handle := pool.Submit(context.Background(), func(context.Context) jobResult { return jobResult{} })
	if res := handle.Wait(); res.err != ErrWorkerPoolClosed {
		t.Fatalf("expected ErrWorkerPoolClosed, got %v", res.err)
	}
}

func TestWorkerPoolNilExecutesInline(t *testing.T) {
	var ran atomic.Bool
	var pool *workerPool
	handle := pool.Submit(context.Background(), func(context.Context) jobResult {
		ran.Store(true)
		return jobResult{}
	})
	if res := handle.Wait(); res.err != nil {
		t.Fatalf("expected nil error, got %v", res.err)
	}
	if !ran.Load() {
		t.Fatalf("expected inline job to run")
	}
}
