package ecs_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/DangerosoDavo/ecs"
	"github.com/DangerosoDavo/ecs/ecs/storage"
)

type maintenanceSystem struct {
	desc ecs.SystemDescriptor
	run  func(ecs.ExecutionContext) ecs.SystemResult
}

type contextMaintenanceSystem struct {
	desc ecs.SystemDescriptor
	run  func(context.Context, ecs.ExecutionContext) ecs.SystemResult
}

func (s contextMaintenanceSystem) Descriptor() ecs.SystemDescriptor { return s.desc }
func (s contextMaintenanceSystem) Run(ctx context.Context, e ecs.ExecutionContext) ecs.SystemResult {
	return s.run(ctx, e)
}

func (s maintenanceSystem) Descriptor() ecs.SystemDescriptor { return s.desc }
func (s maintenanceSystem) Run(_ context.Context, e ecs.ExecutionContext) ecs.SystemResult {
	return s.run(e)
}

func register(t *testing.T, w *ecs.World, name ecs.ComponentType, strategy ecs.StorageStrategy) {
	t.Helper()
	if err := w.RegisterComponent(name, strategy); err != nil {
		t.Fatal(err)
	}
}
func apply(t *testing.T, w *ecs.World, commands ...ecs.Command) {
	t.Helper()
	if err := w.ApplyCommands(commands); err != nil {
		t.Fatal(err)
	}
}

func TestCommandBarrierVisibility(t *testing.T) {
	for _, barrier := range []bool{false, true} {
		t.Run(map[bool]string{false: "end_of_tick", true: "after_group"}[barrier], func(t *testing.T) {
			w := ecs.NewWorld()
			register(t, w, "value", storage.NewDenseStrategy())
			id := w.Registry().Create()
			apply(t, w, ecs.NewAddComponentCommand(id, "value", 0))
			s, _ := ecs.NewScheduler(w)
			defer ecs.CloseScheduler(s)
			writer := maintenanceSystem{desc: ecs.SystemDescriptor{Name: "writer", Writes: []ecs.ComponentType{"value"}}, run: func(e ecs.ExecutionContext) ecs.SystemResult {
				e.Defer(ecs.NewAddComponentCommand(id, "value", 1))
				return ecs.SystemResult{}
			}}
			seen := -1
			reader := maintenanceSystem{desc: ecs.SystemDescriptor{Name: "reader", Reads: []ecs.ComponentType{"value"}}, run: func(e ecs.ExecutionContext) ecs.SystemResult {
				view, _ := e.World().ViewComponent("value")
				value, _ := view.Get(id)
				seen = value.(int)
				return ecs.SystemResult{}
			}}
			if _, err := s.RegisterWorkGroup(ecs.WorkGroupConfig{ID: "write", Systems: []ecs.System{writer}, CommandBarrier: barrier}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.RegisterWorkGroup(ecs.WorkGroupConfig{ID: "read", Systems: []ecs.System{reader}}); err != nil {
				t.Fatal(err)
			}
			if err := s.Tick(context.Background(), time.Millisecond); err != nil {
				t.Fatal(err)
			}
			want := 0
			if barrier {
				want = 1
			}
			if seen != want {
				t.Fatalf("seen %d want %d", seen, want)
			}
			view, _ := w.ViewComponent("value")
			value, _ := view.Get(id)
			if value != 1 {
				t.Fatal("commands not applied at tick end")
			}
		})
	}
}

type snapshotPayload struct {
	Numbers []int
	Labels  map[string]*int
}

func TestAsyncCannotAliasLiveWorld(t *testing.T) {
	w := ecs.NewWorld()
	register(t, w, "payload", storage.NewDenseStrategy())
	register(t, w, "secret", storage.NewDenseStrategy())
	id := w.Registry().Create()
	number := 4
	payload := &snapshotPayload{Numbers: []int{3}, Labels: map[string]*int{"n": &number}}
	apply(t, w, ecs.NewAddComponentCommand(id, "payload", payload))
	w.Resources().Set("resource", []int{7})
	s, _ := ecs.NewScheduler(w)
	defer ecs.CloseScheduler(s)
	started := make(chan struct{})
	mutated := make(chan struct{})
	reader := maintenanceSystem{desc: ecs.SystemDescriptor{Name: "isolated", Reads: []ecs.ComponentType{"payload"}, Resources: []ecs.ResourceAccess{{Name: "resource"}}, AsyncAllowed: true}, run: func(e ecs.ExecutionContext) ecs.SystemResult {
		view, _ := e.World().ViewComponent("payload")
		value, _ := view.Get(id)
		copy := value.(*snapshotPayload)
		if _, err := e.World().ViewComponent("secret"); !errors.Is(err, ecs.ErrComponentNotRegistered) {
			return ecs.SystemResult{Err: errors.New("undeclared component exposed")}
		}
		close(started)
		<-mutated
		if copy.Numbers[0] != 3 || *copy.Labels["n"] != 4 {
			return ecs.SystemResult{Err: errors.New("snapshot aliased live values")}
		}
		copy.Numbers[0] = 99
		*copy.Labels["n"] = 99
		resource, _ := e.World().Resources().Get("resource")
		resource.([]int)[0] = 99
		e.World().DestroyEntity(id) // mutation stays private, including registry.
		return ecs.SystemResult{}
	}}
	writer := maintenanceSystem{desc: ecs.SystemDescriptor{Name: "live", Writes: []ecs.ComponentType{"payload"}}, run: func(e ecs.ExecutionContext) ecs.SystemResult {
		<-started
		payload.Numbers[0] = 8
		number = 9
		close(mutated)
		return ecs.SystemResult{}
	}}
	if _, err := s.RegisterWorkGroup(ecs.WorkGroupConfig{ID: "reader", Mode: ecs.WorkGroupModeAsync, Systems: []ecs.System{reader}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RegisterWorkGroup(ecs.WorkGroupConfig{ID: "writer", Systems: []ecs.System{writer}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Tick(context.Background(), time.Millisecond); err != nil {
		t.Fatal(err)
	}
	resource, _ := w.Resources().Get("resource")
	if payload.Numbers[0] != 8 || number != 9 || resource.([]int)[0] != 7 || !w.Registry().IsAlive(id) {
		t.Fatal("worker changed live world")
	}
}

func TestAsyncRejectsOpaqueHandlesAndDeferredWrites(t *testing.T) {
	for _, opaque := range []bool{false, true} {
		w := ecs.NewWorld()
		if opaque {
			w.Resources().Set("handle", make(chan int))
		}
		s, _ := ecs.NewScheduler(w)
		defer ecs.CloseScheduler(s)
		system := maintenanceSystem{desc: ecs.SystemDescriptor{Name: "invalid", AsyncAllowed: true, Resources: []ecs.ResourceAccess{{Name: "handle"}}}, run: func(e ecs.ExecutionContext) ecs.SystemResult {
			e.Defer(ecs.NewCreateEntityCommand(nil))
			return ecs.SystemResult{}
		}}
		_, err := s.RegisterWorkGroup(ecs.WorkGroupConfig{ID: "async", Mode: ecs.WorkGroupModeAsync, Systems: []ecs.System{system}})
		if err != nil {
			t.Fatal(err)
		}
		err = s.Tick(context.Background(), time.Millisecond)
		want := ecs.ErrReadOnlyExecution
		if opaque {
			want = ecs.ErrUnsupportedSnapshot
		}
		if !errors.Is(err, want) {
			t.Fatalf("got %v want %v", err, want)
		}
		if w.Registry().Count() != 0 {
			t.Fatal("invalid worker mutated registry")
		}
	}
}

func TestTickJoinsWorkersOnSynchronousFailure(t *testing.T) {
	w := ecs.NewWorld()
	s, _ := ecs.NewScheduler(w)
	defer ecs.CloseScheduler(s)
	started := make(chan struct{})
	release := make(chan struct{})
	failed := make(chan struct{})
	async := maintenanceSystem{desc: ecs.SystemDescriptor{Name: "worker", AsyncAllowed: true}, run: func(ecs.ExecutionContext) ecs.SystemResult { close(started); <-release; return ecs.SystemResult{} }}
	failure := maintenanceSystem{desc: ecs.SystemDescriptor{Name: "failure"}, run: func(ecs.ExecutionContext) ecs.SystemResult {
		<-started
		close(failed)
		return ecs.SystemResult{Err: errors.New("expected failure")}
	}}
	_, _ = s.RegisterWorkGroup(ecs.WorkGroupConfig{ID: "async", Mode: ecs.WorkGroupModeAsync, Systems: []ecs.System{async}})
	_, _ = s.RegisterWorkGroup(ecs.WorkGroupConfig{ID: "failure", Systems: []ecs.System{failure}})
	done := make(chan error, 1)
	go func() { done <- s.Tick(context.Background(), time.Millisecond) }()
	<-failed
	if err := s.Tick(context.Background(), time.Millisecond); !errors.Is(err, ecs.ErrConcurrentTick) {
		t.Fatalf("concurrent tick: %v", err)
	}
	select {
	case <-done:
		t.Fatal("Tick returned with a live worker")
	default:
	}
	close(release)
	if err := <-done; err == nil {
		t.Fatal("failure lost")
	}
}

func TestTickFailureCancelsCooperativeWorkersBeforeJoin(t *testing.T) {
	w := ecs.NewWorld()
	s, _ := ecs.NewScheduler(w)
	defer ecs.CloseScheduler(s)
	started := make(chan struct{})
	cancelled := make(chan struct{})
	expected := errors.New("synchronous failure")
	worker := contextMaintenanceSystem{desc: ecs.SystemDescriptor{Name: "cooperative", AsyncAllowed: true}, run: func(ctx context.Context, _ ecs.ExecutionContext) ecs.SystemResult {
		close(started)
		<-ctx.Done()
		close(cancelled)
		return ecs.SystemResult{Err: ctx.Err()}
	}}
	failure := maintenanceSystem{desc: ecs.SystemDescriptor{Name: "failure"}, run: func(ecs.ExecutionContext) ecs.SystemResult { <-started; return ecs.SystemResult{Err: expected} }}
	if _, err := s.RegisterWorkGroup(ecs.WorkGroupConfig{ID: "worker", Mode: ecs.WorkGroupModeAsync, Systems: []ecs.System{worker}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RegisterWorkGroup(ecs.WorkGroupConfig{ID: "failure", Systems: []ecs.System{failure}}); err != nil {
		t.Fatal(err)
	}
	// Parent cancellation is deliberately absent until test cleanup. Tick must
	// cancel its own child context to release the cooperative worker.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Tick(ctx, time.Millisecond) }()
	select {
	case err := <-done:
		if !errors.Is(err, expected) {
			t.Fatalf("synchronous failure lost: %v", err)
		}
		select {
		case <-cancelled:
		default:
			t.Fatal("Tick returned before worker joined")
		}
	case <-time.After(time.Second):
		cancel()
		<-done
		t.Fatal("Tick did not cancel cooperative worker")
	}
}

func TestRestorationNotifiesAndLifecycleCommandsRejectStaleIDs(t *testing.T) {
	w := ecs.NewWorld()
	register(t, w, "value", storage.NewDenseStrategy())
	id := ecs.EntityIDFromParts(4, 7)
	cursor := w.ChangeCursor()
	apply(t, w, ecs.NewRestoreEntityCommand(id))
	batch, err := w.ChangesSince(cursor)
	if err != nil || len(batch.Changes) != 1 || batch.Changes[0].Entity != id || batch.Changes[0].Kind != ecs.ChangeUpdated || !w.Changed()[id] {
		t.Fatalf("restoration was not published: %+v %v", batch, err)
	}
	apply(t, w, ecs.NewDestroyEntityCommand(id))
	for name, command := range map[string]ecs.Command{
		"disable_component": ecs.NewDisableComponentCommand(id, "value"),
		"enable_component":  ecs.NewEnableComponentCommand(id, "value"),
		"disable_entity":    ecs.NewDisableEntityCommand(id),
		"enable_entity":     ecs.NewEnableEntityCommand(id),
	} {
		t.Run(name, func(t *testing.T) {
			if err := command.Apply(w); !errors.Is(err, ecs.ErrStaleEntity) {
				t.Fatalf("stale command accepted: %v", err)
			}
		})
	}
}

func TestLifecycleAndIndependentChangeCursors(t *testing.T) {
	w := ecs.NewWorld(ecs.WithChangeHistoryCapacity(8))
	register(t, w, "value", storage.NewDenseStrategy())
	cursorA, cursorB := w.ChangeCursor(), w.ChangeCursor()
	var id ecs.EntityID
	apply(t, w, ecs.NewSpawnEntityCommand(&id, map[ecs.ComponentType]any{"value": 1}))
	apply(t, w, ecs.NewDisableEntityCommand(id), ecs.NewDestroyEntityCommand(id))
	if len(w.DisabledComponents(id)) != 0 {
		t.Fatal("destroy left disabled metadata")
	}
	if err := w.ApplyCommands([]ecs.Command{ecs.NewAddComponentCommand(id, "value", 2)}); !errors.Is(err, ecs.ErrStaleEntity) {
		t.Fatalf("stale add: %v", err)
	}
	w.DrainChanged() // legacy consumer must not consume either new cursor.
	a, err := w.ChangesSince(cursorA)
	if err != nil {
		t.Fatal(err)
	}
	b, err := w.ChangesSince(cursorB)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) || len(a.Changes) != 3 || a.Changes[2].Kind != ecs.ChangeDestroyed {
		t.Fatalf("lost independent history/tombstone: %+v", a)
	}
	if w.Changed()[id] {
		t.Fatal("deleted entity retained in legacy dirty set")
	}
	if batch, err := w.ChangesSince(a.Cursor); err != nil || len(batch.Changes) != 0 {
		t.Fatal("cursor did not advance")
	}
	view, _ := w.ViewComponent("value")
	if view.Has(id) {
		t.Fatal("destroy left component")
	}
	if w.DestroyEntity(id) {
		t.Fatal("double destroy succeeded")
	}
}

func TestChangeHistoryBoundedAndOverflowExplicit(t *testing.T) {
	w := ecs.NewWorld(ecs.WithChangeHistoryCapacity(2))
	id := w.Registry().Create()
	for i := 0; i < 100; i++ {
		w.MarkChanged(id)
	}
	if _, err := w.ChangesSince(0); !errors.Is(err, ecs.ErrChangeHistoryExpired) {
		t.Fatalf("overflow hidden: %v", err)
	}
	batch, err := w.ChangesSince(98)
	if err != nil || len(batch.Changes) != 2 || batch.Changes[0].Sequence != 99 {
		t.Fatalf("ring corrupted: %+v %v", batch, err)
	}
	w = ecs.NewWorld(ecs.WithChangeHistoryCapacity(0))
	w.MarkChanged(id)
	if _, err := w.ChangesSince(0); !errors.Is(err, ecs.ErrChangeHistoryExpired) {
		t.Fatal("disabled history should require resync")
	}
}

func TestSpawnAndOrderedQuery(t *testing.T) {
	w := ecs.NewWorld()
	register(t, w, "left", storage.NewDenseStrategy())
	register(t, w, "right", storage.NewSharedStrategy())
	var ids [3]ecs.EntityID
	for i := range ids {
		apply(t, w, ecs.NewSpawnEntityCommand(&ids[i], map[ecs.ComponentType]any{"left": i, "right": 10}))
	}
	apply(t, w, ecs.NewDisableComponentCommand(ids[1], "left"))
	var got []ecs.EntityID
	if err := ecs.Query(w, []ecs.ComponentType{"right", "left"}, true, func(id ecs.EntityID, values []any) bool {
		got = append(got, id)
		if values[0] != 10 {
			t.Fatal("query component order changed")
		}
		return true
	}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []ecs.EntityID{ids[0], ids[2]}) {
		t.Fatalf("query omitted disabled join or ordering: %v", got)
	}
	count := w.Registry().Count()
	var invalid ecs.EntityID
	if err := w.ApplyCommands([]ecs.Command{ecs.NewSpawnEntityCommand(&invalid, map[ecs.ComponentType]any{"missing": 1})}); !errors.Is(err, ecs.ErrComponentNotRegistered) {
		t.Fatal(err)
	}
	if !invalid.IsZero() || w.Registry().Count() != count {
		t.Fatal("failed spawn leaked allocation")
	}
}

type failingStrategy struct{}

func (failingStrategy) Name() string                                    { return "failure" }
func (failingStrategy) NewStore(t ecs.ComponentType) ecs.ComponentStore { return &failingStore{typ: t} }

type failingStore struct{ typ ecs.ComponentType }

func (s *failingStore) ComponentType() ecs.ComponentType     { return s.typ }
func (s *failingStore) Len() int                             { return 0 }
func (s *failingStore) Has(ecs.EntityID) bool                { return false }
func (s *failingStore) Get(ecs.EntityID) (any, bool)         { return nil, false }
func (s *failingStore) Iterate(func(ecs.EntityID, any) bool) {}
func (s *failingStore) Set(ecs.EntityID, any) error          { return errors.New("store rejected payload") }
func (s *failingStore) Remove(ecs.EntityID) bool             { return false }
func (s *failingStore) Clear()                               {}

func TestSpawnRollsBackStoreFailure(t *testing.T) {
	w := ecs.NewWorld()
	register(t, w, "a", storage.NewDenseStrategy())
	register(t, w, "z", failingStrategy{})
	var target ecs.EntityID
	err := w.ApplyCommands([]ecs.Command{ecs.NewSpawnEntityCommand(&target, map[ecs.ComponentType]any{"a": 1, "z": 2})})
	view, _ := w.ViewComponent("a")
	if err == nil || !target.IsZero() || w.Registry().Count() != 0 || view.Len() != 0 {
		t.Fatalf("spawn rollback failed: %v %v", target, err)
	}
	if len(w.Changed()) != 0 {
		t.Fatal("rolled back entity published as live")
	}
}

func TestQueryStopsEarlySkipsMissingAndRejectsInvalidBucket(t *testing.T) {
	w := ecs.NewWorld()
	register(t, w, "left", storage.NewDenseStrategy())
	register(t, w, "right", storage.NewSharedStrategy())
	for i := 0; i < 4; i++ {
		id := w.Registry().Create()
		apply(t, w, ecs.NewAddComponentCommand(id, "left", i))
		if i > 1 {
			apply(t, w, ecs.NewAddComponentCommand(id, "right", i))
		}
	}
	// Raw storage is an escape hatch: queries must still exclude stale IDs.
	raw, _ := w.Storage().View("right")
	store := raw.(ecs.ComponentStore)
	_ = store.Set(ecs.EntityIDFromParts(100, 1), 100)
	visited := 0
	if err := ecs.Query(w, []ecs.ComponentType{"right", "left"}, false, func(_ ecs.EntityID, values []any) bool { visited++; return true }); err != nil || visited != 2 {
		t.Fatalf("join included missing/stale rows: %d %v", visited, err)
	}
	visited = 0
	if err := ecs.Query(w, []ecs.ComponentType{"left", "right"}, true, func(ecs.EntityID, []any) bool { visited++; return false }); err != nil || visited != 1 {
		t.Fatalf("early stop ignored: %d %v", visited, err)
	}
	if err := ecs.Query(w, nil, true, func(ecs.EntityID, []any) bool { t.Fatal("empty query visited row"); return true }); err != nil {
		t.Fatal(err)
	}
	if err := ecs.Query(w, []ecs.ComponentType{"unknown"}, true, func(ecs.EntityID, []any) bool { return true }); !errors.Is(err, ecs.ErrComponentNotRegistered) {
		t.Fatalf("unknown bucket: %v", err)
	}
	if _, err := w.ChangesSince(w.ChangeCursor() + 1); !errors.Is(err, ecs.ErrChangeHistoryExpired) {
		t.Fatalf("future cursor accepted: %v", err)
	}
}

func TestSpawnCapturesBundleMapAndOrderedGroupsHonorRetryPolicy(t *testing.T) {
	w := ecs.NewWorld()
	register(t, w, "value", storage.NewDenseStrategy())
	values := map[ecs.ComponentType]any{"value": 1}
	var spawned ecs.EntityID
	command := ecs.NewSpawnEntityCommand(&spawned, values)
	values["value"] = 99
	values["missing"] = 1
	apply(t, w, command)
	view, _ := w.ViewComponent("value")
	value, _ := view.Get(spawned)
	if value != 1 {
		t.Fatal("spawn map changed after enqueue")
	}
	s, _ := ecs.NewScheduler(w)
	defer ecs.CloseScheduler(s)
	order := []string{}
	attempts := 0
	flaky := maintenanceSystem{desc: ecs.SystemDescriptor{Name: "flaky"}, run: func(e ecs.ExecutionContext) ecs.SystemResult {
		attempts++
		e.Defer(ecs.NewSpawnEntityCommand(nil, map[ecs.ComponentType]any{"value": 2}))
		if attempts == 1 {
			return ecs.SystemResult{Err: errors.New("transient")}
		}
		order = append(order, "first")
		return ecs.SystemResult{}
	}}
	last := maintenanceSystem{desc: ecs.SystemDescriptor{Name: "last"}, run: func(ecs.ExecutionContext) ecs.SystemResult { order = append(order, "last"); return ecs.SystemResult{} }}
	if _, err := s.RegisterWorkGroup(ecs.WorkGroupConfig{ID: "last", Systems: []ecs.System{last}}); err != nil {
		t.Fatal(err)
	}
	handle, err := s.RegisterWorkGroup(ecs.WorkGroupConfig{ID: "first", Systems: []ecs.System{flaky}})
	if err != nil || handle.ID() != "first" {
		t.Fatalf("invalid handle: %v %v", handle, err)
	}
	s.Builder().WithSyncOrder([]ecs.WorkGroupID{"first", "last"}).WithErrorPolicy("first", ecs.ErrorPolicyRetry)
	if err := s.Tick(context.Background(), time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || w.Registry().Count() != 2 || !reflect.DeepEqual(order, []string{"first", "last"}) {
		t.Fatalf("retry leaked commands or ordering ignored: %v attempts%d entities%d", order, attempts, w.Registry().Count())
	}
}

func TestAsyncPreservesDisabledInspection(t *testing.T) {
	w := ecs.NewWorld()
	register(t, w, "value", storage.NewDenseStrategy())
	id := w.Registry().Create()
	apply(t, w, ecs.NewAddComponentCommand(id, "value", 2), ecs.NewDisableComponentCommand(id, "value"))
	s, _ := ecs.NewScheduler(w)
	defer ecs.CloseScheduler(s)
	reader := maintenanceSystem{desc: ecs.SystemDescriptor{Name: "inspect", Reads: []ecs.ComponentType{"value"}, AsyncAllowed: true}, run: func(e ecs.ExecutionContext) ecs.SystemResult {
		view, _ := e.World().ViewComponent("value")
		value, ok := view.Get(id)
		if !ok || value != 2 || !e.World().IsComponentDisabled(id, "value") {
			return ecs.SystemResult{Err: errors.New("disabled inspection changed")}
		}
		view.Iterate(func(ecs.EntityID, any) bool { t.Error("disabled entry iterated"); return true })
		return ecs.SystemResult{}
	}}
	if _, err := s.RegisterWorkGroup(ecs.WorkGroupConfig{ID: "read", Mode: ecs.WorkGroupModeAsync, Systems: []ecs.System{reader}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Tick(context.Background(), time.Millisecond); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryRejectsForgedFreeGenerationAndRestoresHoles(t *testing.T) {
	r := ecs.NewEntityRegistry()
	id := r.Create()
	r.Destroy(id)
	if r.IsAlive(ecs.EntityIDFromParts(id.Index(), r.GenerationOf(id.Index()))) {
		t.Fatal("free slot treated as live")
	}
	r = ecs.NewEntityRegistry()
	restored := ecs.EntityIDFromParts(3, 7)
	if err := r.CreateWithID(restored); err != nil {
		t.Fatal(err)
	}
	seen := map[uint32]bool{}
	for i := 0; i < 3; i++ {
		seen[r.Create().Index()] = true
	}
	if !seen[0] || !seen[1] || !seen[2] || r.Count() != 4 {
		t.Fatalf("restore holes leaked: %v", seen)
	}
	r = ecs.NewEntityRegistry()
	exhausted := ecs.EntityIDFromParts(0, ^uint32(0))
	if err := r.CreateWithID(exhausted); err != nil {
		t.Fatal(err)
	}
	r.Destroy(exhausted)
	if r.Create().Index() == 0 {
		t.Fatal("exhausted generation reused")
	}
}

func FuzzEntityLifecycle(f *testing.F) {
	f.Add([]byte{0, 0, 1, 2, 1, 0, 3, 1})
	f.Fuzz(func(t *testing.T, actions []byte) {
		r := ecs.NewEntityRegistry()
		var handles []ecs.EntityID
		live := map[ecs.EntityID]bool{}
		for _, action := range actions {
			if action%3 == 0 || len(handles) == 0 {
				id := r.Create()
				handles = append(handles, id)
				live[id] = true
			} else {
				id := handles[int(action)%len(handles)]
				deleted := r.Destroy(id)
				if deleted != live[id] {
					t.Fatal("destroy disagreed with model")
				}
				delete(live, id)
			}
			if r.Count() != len(live) {
				t.Fatal("count disagreed with model")
			}
			for id := range live {
				if !r.IsAlive(id) {
					t.Fatal("live handle invalid")
				}
			}
		}
	})
}
