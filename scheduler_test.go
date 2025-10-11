package ecs_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/DangerosoDavo/ecs"
)

type testSystem struct {
	name      string
	desc      ecs.SystemDescriptor
	executed  *[]string
	deferCmd  func(ctx ecs.ExecutionContext)
	mu        sync.Mutex
	failLimit int
	failCount int
}

type recordingObserver struct {
	mu        sync.Mutex
	summaries []ecs.WorkGroupSummary
}

func (o *recordingObserver) WorkGroupCompleted(summary ecs.WorkGroupSummary) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.summaries = append(o.summaries, summary)
}

type recordingPromCollector struct {
	mu       sync.Mutex
	observed []ecs.WorkGroupSummary
}

func (c *recordingPromCollector) ObserveWorkGroup(summary ecs.WorkGroupSummary) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.observed = append(c.observed, summary)
}

type recordingSigNozExporter struct {
	mu       sync.Mutex
	exported []ecs.WorkGroupSummary
}

func (e *recordingSigNozExporter) ExportWorkGroup(summary ecs.WorkGroupSummary) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.exported = append(e.exported, summary)
}

func (s *testSystem) Descriptor() ecs.SystemDescriptor {
	if s.desc.Name == "" {
		s.desc.Name = s.name
	}
	return s.desc
}

func (s *testSystem) Run(_ context.Context, ctx ecs.ExecutionContext) ecs.SystemResult {
	if s.deferCmd != nil {
		s.deferCmd(ctx)
	}
	if s.executed != nil {
		s.mu.Lock()
		*s.executed = append(*s.executed, s.name)
		s.mu.Unlock()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failLimit > 0 && s.failCount < s.failLimit {
		s.failCount++
		return ecs.SystemResult{Err: fmt.Errorf("forced failure %s", s.name)}
	}
	return ecs.SystemResult{}
}

func TestSchedulerRunsGroupsInOrder(t *testing.T) {
	world := ecs.NewWorld()
	scheduler, err := ecs.NewScheduler(world)
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}

	order := make([]string, 0)
	sysA := &testSystem{name: "A", executed: &order}
	sysB := &testSystem{name: "B", executed: &order}

	group1 := ecs.WorkGroupConfig{ID: "group1", Mode: ecs.WorkGroupModeSynchronized, Systems: []ecs.System{sysA}}
	group2 := ecs.WorkGroupConfig{ID: "group2", Mode: ecs.WorkGroupModeSynchronized, Systems: []ecs.System{sysB}}

	if _, err := scheduler.RegisterWorkGroup(group1); err != nil {
		t.Fatalf("register group1: %v", err)
	}
	if _, err := scheduler.RegisterWorkGroup(group2); err != nil {
		t.Fatalf("register group2: %v", err)
	}

	if err := scheduler.Tick(context.Background(), time.Millisecond); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if len(order) != 2 || order[0] != "A" || order[1] != "B" {
		t.Fatalf("unexpected execution order: %#v", order)
	}
}

func TestSchedulerAppliesDeferredCommands(t *testing.T) {
	world := ecs.NewWorld()
	scheduler, err := ecs.NewScheduler(world)
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}

	var created ecs.EntityID
	sys := &testSystem{
		name: "creator",
		deferCmd: func(ctx ecs.ExecutionContext) {
			ctx.Defer(ecs.NewCreateEntityCommand(&created))
		},
	}

	cfg := ecs.WorkGroupConfig{ID: "create", Mode: ecs.WorkGroupModeSynchronized, Systems: []ecs.System{sys}}
	if _, err := scheduler.RegisterWorkGroup(cfg); err != nil {
		t.Fatalf("register group: %v", err)
	}

	if err := scheduler.Tick(context.Background(), time.Millisecond); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if created.IsZero() {
		t.Fatalf("expected deferred command to populate entity")
	}
	if !world.Registry().IsAlive(created) {
		t.Fatalf("expected entity to exist after tick")
	}
}

func TestSchedulerRunsAsyncGroup(t *testing.T) {
	world := ecs.NewWorld()
	scheduler, err := ecs.NewScheduler(world)
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}

	if _, err := scheduler.Builder().WithAsyncWorkers(2).Build(nil); err != nil {
		t.Fatalf("configure async workers: %v", err)
	}

	order := make([]string, 0)
	asyncSys := &testSystem{name: "async", executed: &order, desc: ecs.SystemDescriptor{AsyncAllowed: true}}
	syncSys := &testSystem{name: "sync", executed: &order}

	asyncGroup := ecs.WorkGroupConfig{ID: "async", Mode: ecs.WorkGroupModeAsync, Systems: []ecs.System{asyncSys}}
	syncGroup := ecs.WorkGroupConfig{ID: "sync", Mode: ecs.WorkGroupModeSynchronized, Systems: []ecs.System{syncSys}}

	if _, err := scheduler.RegisterWorkGroup(asyncGroup); err != nil {
		t.Fatalf("register async group: %v", err)
	}
	if _, err := scheduler.RegisterWorkGroup(syncGroup); err != nil {
		t.Fatalf("register sync group: %v", err)
	}

	if err := scheduler.Tick(context.Background(), time.Millisecond); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if len(order) != 2 {
		t.Fatalf("expected two systems to run, got %d", len(order))
	}
	foundAsync := false
	foundSync := false
	for _, name := range order {
		switch name {
		case "async":
			foundAsync = true
		case "sync":
			foundSync = true
		}
	}
	if !foundAsync || !foundSync {
		t.Fatalf("expected both async and sync systems to execute: %#v", order)
	}
}

func TestSchedulerHonorsTickInterval(t *testing.T) {
	world := ecs.NewWorld()
	scheduler, err := ecs.NewScheduler(world)
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}

	runCounts := 0
	sys := &testSystem{
		name:     "periodic",
		desc:     ecs.SystemDescriptor{RunEvery: ecs.TickInterval{Every: 2}},
		executed: &[]string{},
	}
	executions := make([]string, 0)
	sys.executed = &executions

	cfg := ecs.WorkGroupConfig{ID: "periodic", Mode: ecs.WorkGroupModeSynchronized, Systems: []ecs.System{sys}}
	if _, err := scheduler.RegisterWorkGroup(cfg); err != nil {
		t.Fatalf("register: %v", err)
	}

	for i := 0; i < 4; i++ {
		if err := scheduler.Tick(context.Background(), time.Millisecond); err != nil {
			t.Fatalf("tick %d: %v", i, err)
		}
		runCounts += len(executions)
		executions = executions[:0]
	}

	if runCounts != 2 {
		t.Fatalf("expected system to run twice, got %d", runCounts)
	}
}

func TestSchedulerAsyncGroupRejectsWrites(t *testing.T) {
	world := ecs.NewWorld()
	scheduler, _ := ecs.NewScheduler(world)
	scheduler.Builder().WithAsyncWorkers(1)
	system := &testSystem{name: "writer", desc: ecs.SystemDescriptor{AsyncAllowed: true, Writes: []ecs.ComponentType{"comp"}}}
	cfg := ecs.WorkGroupConfig{ID: "async-writer", Mode: ecs.WorkGroupModeAsync, Systems: []ecs.System{system}}
	if _, err := scheduler.RegisterWorkGroup(cfg); err == nil {
		t.Fatalf("expected registration to fail for async writer")
	} else if !errors.Is(err, ecs.ErrAsyncWritesNotSupported) {
		t.Fatalf("expected ErrAsyncWritesNotSupported, got %v", err)
	}
}

func TestSchedulerAsyncGroupRespectsAsyncAllowed(t *testing.T) {
	world := ecs.NewWorld()
	scheduler, _ := ecs.NewScheduler(world)
	scheduler.Builder().WithAsyncWorkers(1)
	system := &testSystem{name: "no-async", desc: ecs.SystemDescriptor{AsyncAllowed: false}}
	cfg := ecs.WorkGroupConfig{ID: "async-disallowed", Mode: ecs.WorkGroupModeAsync, Systems: []ecs.System{system}}
	if _, err := scheduler.RegisterWorkGroup(cfg); err == nil {
		t.Fatalf("expected registration to fail when system disallows async")
	} else if !errors.Is(err, ecs.ErrAsyncSystemNotAllowed) {
		t.Fatalf("expected ErrAsyncSystemNotAllowed, got %v", err)
	}
}

func TestSchedulerRejectsConflictingWritersAcrossGroups(t *testing.T) {
	world := ecs.NewWorld()
	scheduler, err := ecs.NewScheduler(world)
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}

	writerA := &testSystem{name: "writerA", desc: ecs.SystemDescriptor{Writes: []ecs.ComponentType{"comp"}}}
	writerB := &testSystem{name: "writerB", desc: ecs.SystemDescriptor{Writes: []ecs.ComponentType{"comp"}}}

	if _, err := scheduler.RegisterWorkGroup(ecs.WorkGroupConfig{ID: "A", Mode: ecs.WorkGroupModeSynchronized, Systems: []ecs.System{writerA}}); err != nil {
		t.Fatalf("register writerA: %v", err)
	}

	if _, err := scheduler.RegisterWorkGroup(ecs.WorkGroupConfig{ID: "B", Mode: ecs.WorkGroupModeSynchronized, Systems: []ecs.System{writerB}}); err == nil {
		t.Fatalf("expected conflict when registering second writer")
	} else if !errors.Is(err, ecs.ErrDuplicateWriteAccess) {
		t.Fatalf("expected ErrDuplicateWriteAccess, got %v", err)
	}
}

func TestSchedulerRejectsResourceWriteConflicts(t *testing.T) {
	world := ecs.NewWorld()
	scheduler, err := ecs.NewScheduler(world)
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}

	resWriterA := &testSystem{name: "resA", desc: ecs.SystemDescriptor{Resources: []ecs.ResourceAccess{{Name: "clock", Mode: ecs.AccessModeWrite}}}}
	resWriterB := &testSystem{name: "resB", desc: ecs.SystemDescriptor{Resources: []ecs.ResourceAccess{{Name: "clock", Mode: ecs.AccessModeWrite}}}}

	if _, err := scheduler.RegisterWorkGroup(ecs.WorkGroupConfig{ID: "resA", Mode: ecs.WorkGroupModeSynchronized, Systems: []ecs.System{resWriterA}}); err != nil {
		t.Fatalf("register resA: %v", err)
	}

	if _, err := scheduler.RegisterWorkGroup(ecs.WorkGroupConfig{ID: "resB", Mode: ecs.WorkGroupModeSynchronized, Systems: []ecs.System{resWriterB}}); err == nil {
		t.Fatalf("expected resource write conflict")
	} else if !errors.Is(err, ecs.ErrDuplicateResourceWriteAccess) {
		t.Fatalf("expected ErrDuplicateResourceWriteAccess, got %v", err)
	}
}

func TestSchedulerAllowsMultipleResourceReaders(t *testing.T) {
	world := ecs.NewWorld()
	scheduler, err := ecs.NewScheduler(world)
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}

	readerA := &testSystem{name: "readerA", desc: ecs.SystemDescriptor{Resources: []ecs.ResourceAccess{{Name: "clock", Mode: ecs.AccessModeRead}}}}
	readerB := &testSystem{name: "readerB", desc: ecs.SystemDescriptor{Resources: []ecs.ResourceAccess{{Name: "clock", Mode: ecs.AccessModeRead}}}}

	if _, err := scheduler.RegisterWorkGroup(ecs.WorkGroupConfig{ID: "readerA", Mode: ecs.WorkGroupModeSynchronized, Systems: []ecs.System{readerA}}); err != nil {
		t.Fatalf("register readerA: %v", err)
	}

	if _, err := scheduler.RegisterWorkGroup(ecs.WorkGroupConfig{ID: "readerB", Mode: ecs.WorkGroupModeSynchronized, Systems: []ecs.System{readerB}}); err != nil {
		t.Fatalf("register readerB: %v", err)
	}
}

func TestSchedulerAsyncResourceWritesRejected(t *testing.T) {
	world := ecs.NewWorld()
	scheduler, _ := ecs.NewScheduler(world)
	scheduler.Builder().WithAsyncWorkers(1)
	writer := &testSystem{name: "asyncRes", desc: ecs.SystemDescriptor{AsyncAllowed: true, Resources: []ecs.ResourceAccess{{Name: "clock", Mode: ecs.AccessModeWrite}}}}
	if _, err := scheduler.RegisterWorkGroup(ecs.WorkGroupConfig{ID: "async-resource", Mode: ecs.WorkGroupModeAsync, Systems: []ecs.System{writer}}); err == nil {
		t.Fatalf("expected async resource write rejection")
	} else if !errors.Is(err, ecs.ErrAsyncResourceWritesNotSupported) {
		t.Fatalf("expected ErrAsyncResourceWritesNotSupported, got %v", err)
	}
}

func TestSchedulerObserverReceivesSummary(t *testing.T) {
	world := ecs.NewWorld()
	scheduler, err := ecs.NewScheduler(world)
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}

	observer := &recordingObserver{}
	prom := &recordingPromCollector{}
	sig := &recordingSigNozExporter{}
	if _, err := scheduler.Builder().WithInstrumentation(ecs.InstrumentationConfig{
		Observer: observer,
		Observation: ecs.ObservationSettings{
			EnablePrometheus:    true,
			PrometheusCollector: prom,
			EnableSigNoz:        true,
			SigNozExporter:      sig,
		},
	}).Build(nil); err != nil {
		t.Fatalf("configure instrumentation: %v", err)
	}

	sys := &testSystem{name: "observer"}
	if _, err := scheduler.RegisterWorkGroup(ecs.WorkGroupConfig{ID: "obs", Mode: ecs.WorkGroupModeSynchronized, Systems: []ecs.System{sys}}); err != nil {
		t.Fatalf("register: %v", err)
	}

	if err := scheduler.Tick(context.Background(), time.Millisecond); err != nil {
		t.Fatalf("tick: %v", err)
	}

	observer.mu.Lock()
	if len(observer.summaries) != 1 {
		observer.mu.Unlock()
		t.Fatalf("expected 1 summary, got %d", len(observer.summaries))
	}
	summary := observer.summaries[0]
	observer.mu.Unlock()
	if summary.WorkGroupID != "obs" {
		t.Fatalf("unexpected work group id: %s", summary.WorkGroupID)
	}
	if summary.SystemsExecuted != 1 {
		t.Fatalf("expected 1 executed system, got %d", summary.SystemsExecuted)
	}
	prom.mu.Lock()
	if len(prom.observed) != 1 {
		prom.mu.Unlock()
		t.Fatalf("expected prom collector to observe 1 summary, got %d", len(prom.observed))
	}
	prom.mu.Unlock()
	sig.mu.Lock()
	if len(sig.exported) != 1 {
		sig.mu.Unlock()
		t.Fatalf("expected sig exporter to receive 1 summary, got %d", len(sig.exported))
	}
	sig.mu.Unlock()
}

func TestSchedulerRetryPolicy(t *testing.T) {
	world := ecs.NewWorld()
	scheduler, err := ecs.NewScheduler(world)
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}

	failing := &testSystem{name: "flaky", failLimit: 1}
	cfg := ecs.WorkGroupConfig{ID: "retry", Mode: ecs.WorkGroupModeSynchronized, Systems: []ecs.System{failing}, ErrorPolicy: ecs.ErrorPolicyRetry}
	if _, err := scheduler.RegisterWorkGroup(cfg); err != nil {
		t.Fatalf("register: %v", err)
	}

	if err := scheduler.Tick(context.Background(), time.Millisecond); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if failing.failCount != 1 {
		t.Fatalf("expected exactly one failure before retry success, got %d", failing.failCount)
	}
}
