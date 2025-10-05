package ecs

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"runtime/trace"
	"sort"
	"sync"
	"time"
)

// NewScheduler constructs a scheduler bound to the provided world.
func NewScheduler(world *World) (Scheduler, error) {
	if world == nil {
		world = NewWorld()
	}
	s := &basicScheduler{
		world:             world,
		groupStates:       make(map[WorkGroupID]*workGroupState),
		registrationOrder: make([]WorkGroupID, 0),
		pool:              NewCommandBufferPool(),
		logger:            noopLogger{},
		tracer:            noopTracer{},
		errorPolicies:     make(map[WorkGroupID]ErrorPolicy),
		componentOwners:   make(map[ComponentType]WorkGroupID),
		resourceOwners:    make(map[string]WorkGroupID),
		resourceReaders:   make(map[string]map[WorkGroupID]struct{}),
		observer:          noopObserver{},
	}
	s.applyInstrumentation(InstrumentationConfig{})
	s.rebuildOrder()
	return s, nil
}

type basicScheduler struct {
	mu                sync.RWMutex
	world             *World
	groupStates       map[WorkGroupID]*workGroupState
	registrationOrder []WorkGroupID
	orderedGroups     []*workGroupState
	syncOrder         []WorkGroupID
	pool              *CommandBufferPool
	asyncPool         *workerPool
	logger            Logger
	tracer            Tracer
	instrumentation   InstrumentationConfig
	observer          SchedulerObserver
	errorPolicies     map[WorkGroupID]ErrorPolicy
	tickIndex         uint64
	asyncWorkers      int
	componentOwners   map[ComponentType]WorkGroupID
	resourceOwners    map[string]WorkGroupID
	resourceReaders   map[string]map[WorkGroupID]struct{}
}

type workGroupState struct {
	id             WorkGroupID
	mode           WorkGroupMode
	systems        []System
	interval       TickInterval
	lastRun        uint64
	policy         ErrorPolicy
	priority       int
	readSet        map[ComponentType]struct{}
	writeSet       map[ComponentType]struct{}
	resourceReads  map[string]struct{}
	resourceWrites map[string]struct{}
}

type schedulerBuilder struct {
	scheduler *basicScheduler
}

type workGroupHandle struct {
	id WorkGroupID
}

func (h workGroupHandle) ID() WorkGroupID { return h.id }

// Builder returns a builder that can mutate the scheduler configuration.
func (s *basicScheduler) Builder() SchedulerBuilder {
	return &schedulerBuilder{scheduler: s}
}

func (b *schedulerBuilder) WithSyncOrder(order []WorkGroupID) SchedulerBuilder {
	b.scheduler.mu.Lock()
	b.scheduler.syncOrder = append([]WorkGroupID(nil), order...)
	b.scheduler.rebuildOrder()
	b.scheduler.mu.Unlock()
	return b
}

func (b *schedulerBuilder) WithAsyncWorkers(count int) SchedulerBuilder {
	if count < 0 {
		count = 0
	}
	b.scheduler.mu.Lock()
	b.scheduler.asyncWorkers = count
	if b.scheduler.asyncPool != nil {
		b.scheduler.asyncPool.Close()
		b.scheduler.asyncPool = nil
	}
	if count > 0 {
		b.scheduler.asyncPool = newWorkerPool(count)
	}
	b.scheduler.mu.Unlock()
	return b
}

func (b *schedulerBuilder) WithErrorPolicy(id WorkGroupID, policy ErrorPolicy) SchedulerBuilder {
	b.scheduler.mu.Lock()
	if policy != 0 {
		b.scheduler.errorPolicies[id] = policy
	} else {
		delete(b.scheduler.errorPolicies, id)
	}
	if state, ok := b.scheduler.groupStates[id]; ok {
		state.policy = b.scheduler.resolvePolicy(id, state.policy)
	}
	b.scheduler.mu.Unlock()
	return b
}

func (b *schedulerBuilder) WithInstrumentation(cfg InstrumentationConfig) SchedulerBuilder {
	b.scheduler.mu.Lock()
	b.scheduler.applyInstrumentation(cfg)
	b.scheduler.mu.Unlock()
	return b
}

func (b *schedulerBuilder) Build(world *World) (Scheduler, error) {
	b.scheduler.mu.Lock()
	defer b.scheduler.mu.Unlock()
	if world != nil {
		b.scheduler.world = world
	} else if b.scheduler.world == nil {
		b.scheduler.world = NewWorld()
	}
	return b.scheduler, nil
}

func (s *basicScheduler) applyInstrumentation(cfg InstrumentationConfig) {
	s.instrumentation = cfg
	if !cfg.EnableTrace {
		s.tracer = noopTracer{}
	}
	s.observer = buildObserverChain(s.logger, cfg)
}

func (s *basicScheduler) RegisterWorkGroup(cfg WorkGroupConfig) (WorkGroupHandle, error) {
	if cfg.ID == "" {
		return nil, fmt.Errorf("ecs: work group requires non-empty ID")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.groupStates[cfg.ID]; exists {
		return nil, fmt.Errorf("ecs: work group %s already registered", cfg.ID)
	}

	if cfg.Mode == WorkGroupModeAsync && s.asyncPool == nil {
		workers := s.asyncWorkers
		if workers <= 0 {
			workers = runtime.NumCPU()
			if workers <= 0 {
				workers = 1
			}
			s.asyncWorkers = workers
		}
		s.asyncPool = newWorkerPool(workers)
	}

	systems := make([]System, 0, len(cfg.Systems))
	for _, sys := range cfg.Systems {
		if sys == nil {
			continue
		}
		systems = append(systems, sys)
	}

	reads, writes, resourceReads, resourceWrites, err := validateSystemsAccess(cfg.Mode, systems)
	if err != nil {
		return nil, err
	}

	state := &workGroupState{
		id:             cfg.ID,
		mode:           cfg.Mode,
		systems:        systems,
		interval:       cfg.Interval,
		policy:         s.resolvePolicy(cfg.ID, cfg.ErrorPolicy),
		priority:       cfg.Priority,
		readSet:        reads,
		writeSet:       writes,
		resourceReads:  resourceReads,
		resourceWrites: resourceWrites,
	}

	if err := s.checkCrossGroupConflicts(state); err != nil {
		return nil, err
	}

	s.groupStates[cfg.ID] = state
	s.registrationOrder = append(s.registrationOrder, cfg.ID)
	for comp := range state.writeSet {
		s.componentOwners[comp] = state.id
	}
	for res := range state.resourceWrites {
		s.resourceOwners[res] = state.id
	}
	for res := range state.resourceReads {
		if _, ok := s.resourceReaders[res]; !ok {
			s.resourceReaders[res] = make(map[WorkGroupID]struct{})
		}
		s.resourceReaders[res][state.id] = struct{}{}
	}
	s.rebuildOrder()

	return workGroupHandle{id: cfg.ID}, nil
}

func (s *basicScheduler) resolvePolicy(id WorkGroupID, supplied ErrorPolicy) ErrorPolicy {
	if supplied != 0 {
		return supplied
	}
	if policy, ok := s.errorPolicies[id]; ok {
		return policy
	}
	return ErrorPolicyAbort
}

func validateSystemsAccess(mode WorkGroupMode, systems []System) (map[ComponentType]struct{}, map[ComponentType]struct{}, map[string]struct{}, map[string]struct{}, error) {
	writeOwners := make(map[ComponentType]string)
	reads := make(map[ComponentType]struct{})
	writes := make(map[ComponentType]struct{})
	resourceReads := make(map[string]struct{})
	resourceWrites := make(map[string]struct{})
	resourceWriteOwners := make(map[string]string)
	for _, sys := range systems {
		desc := sys.Descriptor()
		name := desc.Name
		if name == "" {
			name = "<unnamed>"
		}
		if mode == WorkGroupModeAsync {
			if !desc.AsyncAllowed {
				return nil, nil, nil, nil, fmt.Errorf("%w: %s", ErrAsyncSystemNotAllowed, name)
			}
			if len(desc.Writes) > 0 {
				return nil, nil, nil, nil, fmt.Errorf("%w: %s writes components %v", ErrAsyncWritesNotSupported, name, desc.Writes)
			}
			for _, res := range desc.Resources {
				if res.Mode == AccessModeWrite {
					return nil, nil, nil, nil, fmt.Errorf("%w: %s writes resource %s", ErrAsyncResourceWritesNotSupported, name, res.Name)
				}
			}
		}
		for _, comp := range desc.Reads {
			reads[comp] = struct{}{}
		}
		seenLocal := make(map[ComponentType]struct{}, len(desc.Writes))
		for _, comp := range desc.Writes {
			if _, ok := seenLocal[comp]; ok {
				return nil, nil, nil, nil, fmt.Errorf("%w: %s writes component %s multiple times", ErrDuplicateWriteAccess, name, comp)
			}
			seenLocal[comp] = struct{}{}
			if owner, exists := writeOwners[comp]; exists {
				return nil, nil, nil, nil, fmt.Errorf("%w: %s and %s both write component %s", ErrDuplicateWriteAccess, owner, name, comp)
			}
			writeOwners[comp] = name
			writes[comp] = struct{}{}
		}
		resourceSeenLocal := make(map[string]struct{})
		for _, res := range desc.Resources {
			if res.Name == "" {
				continue
			}
			if res.Mode == AccessModeWrite {
				if _, ok := resourceSeenLocal[res.Name]; ok {
					return nil, nil, nil, nil, fmt.Errorf("%w: %s writes resource %s multiple times", ErrDuplicateResourceWriteAccess, name, res.Name)
				}
				resourceSeenLocal[res.Name] = struct{}{}
				if owner, exists := resourceWriteOwners[res.Name]; exists {
					return nil, nil, nil, nil, fmt.Errorf("%w: %s and %s both write resource %s", ErrDuplicateResourceWriteAccess, owner, name, res.Name)
				}
				resourceWriteOwners[res.Name] = name
				resourceWrites[res.Name] = struct{}{}
			} else {
				resourceReads[res.Name] = struct{}{}
			}
			if res.Mode == AccessModeWrite {
				resourceReads[res.Name] = struct{}{}
			}
		}
	}
	return reads, writes, resourceReads, resourceWrites, nil
}

func (s *basicScheduler) checkCrossGroupConflicts(state *workGroupState) error {
	for comp := range state.writeSet {
		if owner, exists := s.componentOwners[comp]; exists && owner != state.id {
			return fmt.Errorf("%w: %s already owns component %s", ErrDuplicateWriteAccess, owner, comp)
		}
	}
	for res := range state.resourceWrites {
		if owner, exists := s.resourceOwners[res]; exists && owner != state.id {
			return fmt.Errorf("%w: %s already owns resource %s", ErrDuplicateResourceWriteAccess, owner, res)
		}
		if readers, exists := s.resourceReaders[res]; exists {
			for reader := range readers {
				if reader != state.id {
					return fmt.Errorf("%w: %s already reads resource %s", ErrDuplicateResourceWriteAccess, reader, res)
				}
			}
		}
	}
	for res := range state.resourceReads {
		if owner, exists := s.resourceOwners[res]; exists && owner != state.id {
			return fmt.Errorf("%w: %s already writes resource %s", ErrDuplicateResourceWriteAccess, owner, res)
		}
	}
	return nil
}

func (s *basicScheduler) rebuildOrder() {
	ordered := make([]*workGroupState, 0, len(s.groupStates))
	seen := make(map[WorkGroupID]struct{}, len(s.groupStates))

	for _, id := range s.syncOrder {
		if state, ok := s.groupStates[id]; ok {
			ordered = append(ordered, state)
			seen[id] = struct{}{}
		}
	}

	for _, id := range s.registrationOrder {
		if _, ok := seen[id]; ok {
			continue
		}
		if state, ok := s.groupStates[id]; ok {
			ordered = append(ordered, state)
			seen[id] = struct{}{}
		}
	}

	s.orderedGroups = ordered
}

func (s *basicScheduler) Tick(ctx context.Context, dt time.Duration) error {
	buf := s.pool.Get()
	defer s.pool.Put(buf)

	s.mu.RLock()
	groups := append([]*workGroupState(nil), s.orderedGroups...)
	tracer := s.tracer
	logger := s.logger
	world := s.world
	tick := s.tickIndex
	s.mu.RUnlock()

	executedGroups := make([]WorkGroupID, 0, len(groups))
	asyncHandles := make([]*jobHandle, 0)
	asyncGroupIDs := make([]WorkGroupID, 0)

	for _, group := range groups {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !shouldRunTick(tick, group.interval) {
			continue
		}
		if group.mode == WorkGroupModeAsync {
			handle := s.dispatchAsync(ctx, group, world, dt, tick, logger, tracer)
			asyncHandles = append(asyncHandles, handle)
			asyncGroupIDs = append(asyncGroupIDs, group.id)
			continue
		}
		summary, err := s.runWorkGroup(ctx, group, world, dt, tick, buf, logger, tracer, false)
		if err != nil {
			if group.policy == ErrorPolicyContinue {
				logger.Error("work group error", "group", string(group.id), "err", err)
				s.publishWorkGroupSummary(summary)
				continue
			}
			s.publishWorkGroupSummary(summary)
			return err
		}
		executedGroups = append(executedGroups, group.id)
		s.publishWorkGroupSummary(summary)
	}

	for idx, handle := range asyncHandles {
		res := handle.Wait()
		if summary := res.Summary(); summary != nil {
			s.publishWorkGroupSummary(*summary)
		}
		if err := res.Err(); err != nil {
			groupID := asyncGroupIDs[idx]
			s.mu.RLock()
			state := s.groupStates[groupID]
			s.mu.RUnlock()
			if state != nil && state.policy == ErrorPolicyContinue {
				logger.Error("async work group error", "group", string(groupID), "err", err)
				continue
			}
			return err
		}
		if commands := res.Commands(); len(commands) > 0 {
			if err := world.ApplyCommands(commands); err != nil {
				return err
			}
		}
		executedGroups = append(executedGroups, asyncGroupIDs[idx])
	}

	drained := buf.Drain()
	if len(drained) > 0 {
		if err := world.ApplyCommands(drained); err != nil {
			return err
		}
	}

	s.mu.Lock()
	for _, id := range executedGroups {
		if state, ok := s.groupStates[id]; ok {
			state.lastRun = tick
		}
	}
	s.tickIndex++
	s.mu.Unlock()
	return nil
}
func (s *basicScheduler) runWorkGroup(ctx context.Context, group *workGroupState, world *World, dt time.Duration, tick uint64, buf *CommandBuffer, logger Logger, tracer Tracer, async bool) (workGroupRunSummary, error) {
	groupLogger := logger.With("work_group", string(group.id))
	execCtx := &systemExecutionContext{
		world:    world,
		dt:       dt,
		tick:     tick,
		logger:   groupLogger,
		tracer:   tracer,
		commands: buf,
	}

	summary := workGroupRunSummary{
		id:              group.id,
		mode:            group.mode,
		async:           async,
		tick:            tick,
		componentReads:  componentSetToSlice(group.readSet),
		componentWrites: componentSetToSlice(group.writeSet),
		resourceReads:   stringSetToSlice(group.resourceReads),
		resourceWrites:  stringSetToSlice(group.resourceWrites),
	}

	start := time.Now()
	for _, system := range group.systems {
		if err := ctx.Err(); err != nil {
			summary.err = err
			summary.duration = time.Since(start)
			return summary, err
		}
		desc := system.Descriptor()
		summary.systemsTotal++
		if !shouldRunTick(tick, desc.RunEvery) {
			summary.systemsSkipped++
			continue
		}
		systemLogger := groupLogger.With("system", desc.Name)
		execCtx.logger = systemLogger

		snapshot := buf.Snapshot()
		result := system.Run(ctx, execCtx)
		if result.Err != nil {
			if group.policy == ErrorPolicyRetry {
				systemLogger.Error("system failed, retrying", "err", result.Err)
				buf.Restore(snapshot)
				result = system.Run(ctx, execCtx)
				if result.Err == nil {
					systemLogger.Info("system retry succeeded")
					if result.Skipped {
						summary.systemsSkipped++
					} else {
						summary.systemsExecuted++
						systemLogger.Info("system executed")
					}
					continue
				}
			}
			buf.Restore(snapshot)
			err := fmt.Errorf("ecs: system %s failed: %w", desc.Name, result.Err)
			summary.err = err
			summary.duration = time.Since(start)
			return summary, err
		}
		if result.Skipped {
			summary.systemsSkipped++
			continue
		}
		summary.systemsExecuted++
		systemLogger.Info("system executed")
	}

	summary.duration = time.Since(start)
	return summary, nil
}

func (s *basicScheduler) dispatchAsync(ctx context.Context, group *workGroupState, world *World, dt time.Duration, tick uint64, logger Logger, tracer Tracer) *jobHandle {
	pool := s.asyncPool
	if pool == nil {
		jobBuf := s.pool.Get()
		summary, err := s.runWorkGroup(ctx, group, world, dt, tick, jobBuf, logger, tracer, true)
		commands := jobBuf.Drain()
		s.pool.Put(jobBuf)
		ch := make(chan jobResult, 1)
		summaryCopy := summary
		res := jobResult{err: err, summary: &summaryCopy}
		if err == nil {
			res.commands = commands
		}
		ch <- res
		close(ch)
		return &jobHandle{result: ch}
	}
	return pool.Submit(ctx, func(jobCtx context.Context) jobResult {
		jobBuf := s.pool.Get()
		defer s.pool.Put(jobBuf)
		summary, err := s.runWorkGroup(jobCtx, group, world, dt, tick, jobBuf, logger, tracer, true)
		summaryCopy := summary
		if err != nil {
			return jobResult{err: err, summary: &summaryCopy}
		}
		commands := jobBuf.Drain()
		return jobResult{commands: commands, summary: &summaryCopy}
	})
}

func (s *basicScheduler) publishWorkGroupSummary(summary workGroupRunSummary) {
	if s.observer == nil {
		return
	}
	s.observer.WorkGroupCompleted(summary.toPublic())
}

type workGroupRunSummary struct {
	id              WorkGroupID
	mode            WorkGroupMode
	async           bool
	tick            uint64
	componentReads  []ComponentType
	componentWrites []ComponentType
	resourceReads   []string
	resourceWrites  []string
	systemsTotal    int
	systemsExecuted int
	systemsSkipped  int
	duration        time.Duration
	err             error
}

func (summary workGroupRunSummary) toPublic() WorkGroupSummary {
	return WorkGroupSummary{
		WorkGroupID:     summary.id,
		Mode:            summary.mode,
		Async:           summary.async,
		Tick:            summary.tick,
		Duration:        summary.duration,
		SystemsTotal:    summary.systemsTotal,
		SystemsExecuted: summary.systemsExecuted,
		SystemsSkipped:  summary.systemsSkipped,
		Error:           summary.err,
		ComponentReads:  append([]ComponentType(nil), summary.componentReads...),
		ComponentWrites: append([]ComponentType(nil), summary.componentWrites...),
		ResourceReads:   append([]string(nil), summary.resourceReads...),
		ResourceWrites:  append([]string(nil), summary.resourceWrites...),
	}
}

func componentSetToSlice(set map[ComponentType]struct{}) []ComponentType {
	if len(set) == 0 {
		return nil
	}
	out := make([]ComponentType, 0, len(set))
	for comp := range set {
		out = append(out, comp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func stringSetToSlice(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for val := range set {
		out = append(out, val)
	}
	sort.Strings(out)
	return out
}

func shouldRunTick(tick uint64, interval TickInterval) bool {
	every := uint64(interval.Every)
	if every == 0 {
		return true
	}
	offset := uint64(interval.Offset % interval.Every)
	return (tick+offset)%every == 0
}

func (s *basicScheduler) Run(ctx context.Context, steps int, dt time.Duration) error {
	for i := 0; i < steps; i++ {
		if err := s.Tick(ctx, dt); err != nil {
			return err
		}
	}
	return nil
}

func (s *basicScheduler) RunWithTrace(ctx context.Context, w io.Writer, fn func() error) error {
	if s.instrumentation.EnableTrace && w != nil {
		if err := trace.Start(w); err != nil {
			return err
		}
		defer trace.Stop()
	}
	return fn()
}

func (s *basicScheduler) resolveLogger() Logger {
	return s.logger
}

func (s *basicScheduler) TickIndex() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tickIndex
}

// Internal execution context used during system runs.
type systemExecutionContext struct {
	world    *World
	dt       time.Duration
	tick     uint64
	logger   Logger
	tracer   Tracer
	commands *CommandBuffer
}

func (c *systemExecutionContext) World() *World { return c.world }

func (c *systemExecutionContext) TimeDelta() time.Duration { return c.dt }

func (c *systemExecutionContext) TickIndex() uint64 { return c.tick }

func (c *systemExecutionContext) Logger() Logger { return c.logger }

func (c *systemExecutionContext) Tracer() Tracer { return c.tracer }

func (c *systemExecutionContext) Defer(cmd Command) { c.commands.Push(cmd) }

// noopLogger is used until a real logger is supplied.
type noopLogger struct{}

func (noopLogger) With(string, any) Logger { return noopLogger{} }
func (noopLogger) Info(string, ...any)     {}
func (noopLogger) Error(string, ...any)    {}

type noopTracer struct{}

func (noopTracer) Start(ctx context.Context, name string) (context.Context, TraceSpan) {
	return ctx, noopSpan{}
}

type noopSpan struct{}

func (noopSpan) End() {}

type noopObserver struct{}

func (noopObserver) WorkGroupCompleted(WorkGroupSummary) {}
