package ecs_test

import (
	"testing"

	"github.com/yourorg/ecs"
)

func TestCommandBufferPushDrain(t *testing.T) {
	buf := ecs.NewCommandBuffer()
	if buf.Len() != 0 {
		t.Fatalf("expected empty buffer")
	}

	cmd := ecs.NewDestroyEntityCommand(ecs.EntityID{})
	buf.Push(cmd)
	if buf.Len() != 1 {
		t.Fatalf("expected length 1, got %d", buf.Len())
	}

	drained := buf.Drain()
	if len(drained) != 1 {
		t.Fatalf("expected drained commands")
	}
	if buf.Len() != 0 {
		t.Fatalf("expected buffer reset")
	}
}

func TestCommandBufferPoolReuses(t *testing.T) {
	pool := ecs.NewCommandBufferPool()
	buf := pool.Get()
	buf.Push(ecs.NewDestroyEntityCommand(ecs.EntityID{}))
	pool.Put(buf)

	reused := pool.Get()
	if reused.Len() != 0 {
		t.Fatalf("expected buffer to be cleared when reused")
	}
}

func TestCommandBufferSnapshotRestore(t *testing.T) {
	buf := ecs.NewCommandBuffer()
	buf.Push(ecs.NewDestroyEntityCommand(ecs.EntityID{}))
	snap := buf.Snapshot()
	buf.Push(ecs.NewCreateEntityCommand(nil))
	if buf.Len() != 2 {
		t.Fatalf("expected len 2")
	}
	buf.Restore(snap)
	if buf.Len() != 1 {
		t.Fatalf("expected len reset to 1, got %d", buf.Len())
	}
}
