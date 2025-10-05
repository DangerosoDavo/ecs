package ecs

import (
	"context"
	"io"
	"time"
)

// Scheduler coordinates work group execution each tick.
type Scheduler interface {
	Tick(ctx context.Context, dt time.Duration) error
	Run(ctx context.Context, steps int, dt time.Duration) error
	RunWithTrace(ctx context.Context, w io.Writer, fn func() error) error
	RegisterWorkGroup(cfg WorkGroupConfig) (WorkGroupHandle, error)
	Builder() SchedulerBuilder
}

// SchedulerBuilder configures scheduler options prior to construction.
type SchedulerBuilder interface {
	WithSyncOrder(order []WorkGroupID) SchedulerBuilder
	WithAsyncWorkers(count int) SchedulerBuilder
	WithErrorPolicy(id WorkGroupID, policy ErrorPolicy) SchedulerBuilder
	WithInstrumentation(cfg InstrumentationConfig) SchedulerBuilder
	Build(world *World) (Scheduler, error)
}

// WorkGroupConfig declares a set of systems and execution preferences.
type WorkGroupConfig struct {
	ID          WorkGroupID
	Mode        WorkGroupMode
	Systems     []System
	Interval    TickInterval
	ErrorPolicy ErrorPolicy
	Priority    int
}

// WorkGroupMode selects synchronous or asynchronous execution.
type WorkGroupMode uint8

const (
	WorkGroupModeSynchronized WorkGroupMode = iota
	WorkGroupModeAsync
)

// WorkGroupID uniquely identifies a work group within the scheduler.
type WorkGroupID string

// WorkGroupHandle references a registered work group for future configuration.
type WorkGroupHandle interface {
	ID() WorkGroupID
}

// TickInterval controls how frequently a system or work group runs.
type TickInterval struct {
	Every  uint32
	Offset uint32
}

// ErrorPolicy defines how scheduler responds to system failures.
type ErrorPolicy uint8

const (
	ErrorPolicyAbort ErrorPolicy = iota
	ErrorPolicyContinue
	ErrorPolicyRetry
)

// InstrumentationConfig configures logging, tracing, and metrics sinks.
type InstrumentationConfig struct {
	EnableTrace   bool
	EnableMetrics bool
	Observer      SchedulerObserver
	Observation   ObservationSettings
}

// ObservationSettings toggles built-in observer integrations.
type ObservationSettings struct {
	EnableStructuredLogging bool
	LoggingFormat           ObservationLogFormat
	StructuredLogger        Logger
	EnablePrometheus        bool
	PrometheusCollector     PrometheusCollector
	PrometheusOptions       *PrometheusCollectorOptions
	EnableSigNoz            bool
	SigNozExporter          SigNozExporter
	SigNozOptions           *SigNozOptions
}

// ObservationLogFormat controls structured logging encoding.
type ObservationLogFormat uint8

const (
	ObservationLogFormatJSON ObservationLogFormat = iota
	ObservationLogFormatKeyValue
)

// SchedulerObserver receives summaries after work groups complete.
type SchedulerObserver interface {
	WorkGroupCompleted(summary WorkGroupSummary)
}

// PrometheusCollector handles work-group summaries for Prometheus-style metrics.
type PrometheusCollector interface {
	ObserveWorkGroup(summary WorkGroupSummary)
}

type PrometheusCollectorOptions struct {
	Writer          io.Writer
	DurationBuckets []time.Duration
}

// SigNozExporter handles work-group summaries for SigNoz platforms.
type SigNozExporter interface {
	ExportWorkGroup(summary WorkGroupSummary)
}

type SigNozOptions struct {
	Writer      io.Writer
	ServiceName string
}

// WorkGroupSummary captures execution metadata for a work group.
type WorkGroupSummary struct {
	WorkGroupID     WorkGroupID
	Mode            WorkGroupMode
	Async           bool
	Tick            uint64
	Duration        time.Duration
	SystemsTotal    int
	SystemsExecuted int
	SystemsSkipped  int
	Error           error
	ComponentReads  []ComponentType
	ComponentWrites []ComponentType
	ResourceReads   []string
	ResourceWrites  []string
}

// System represents executable logic within a work group.
type System interface {
	Descriptor() SystemDescriptor
	Run(ctx context.Context, exec ExecutionContext) SystemResult
}

// SystemDescriptor describes resource usage and metadata for a system.
type SystemDescriptor struct {
	Name         string
	Reads        []ComponentType
	Writes       []ComponentType
	Resources    []ResourceAccess
	Tags         []string
	RunEvery     TickInterval
	AsyncAllowed bool
}

// SystemResult indicates how a system behaved during execution.
type SystemResult struct {
	Skipped bool
	Err     error
}

// ExecutionContext supplies a system with scoped access to the world.
type ExecutionContext interface {
	World() *World
	TimeDelta() time.Duration
	TickIndex() uint64
	Logger() Logger
	Defer(cmd Command)
}

// World encapsulates entity/component storage and resources.
type World struct {
	registry  *EntityRegistry
	storage   StorageProvider
	resources ResourceContainer
}

// StorageProvider manages component storage backends.
type StorageProvider interface {
	RegisterComponent(ComponentType, StorageStrategy) error
	View(ComponentType) (ComponentView, error)
	Apply(*World, []Command) error
}

// StorageStrategy describes how a component type is stored internally.
type StorageStrategy interface {
	Name() string
	NewStore(ComponentType) ComponentStore
}

// ComponentType identifies a component storage bucket.
type ComponentType string

// ResourceAccess declares mutable or immutable access to a resource.
type ResourceAccess struct {
	Name string
	Mode AccessMode
}

// AccessMode indicates read or write intent when using a resource.
type AccessMode uint8

const (
	AccessModeRead AccessMode = iota
	AccessModeWrite
)

// ComponentStore permits read/write access to component instances.
type ComponentStore interface {
	ComponentView
	Set(EntityID, any) error
	Remove(EntityID) bool
	Clear()
}

// ComponentView exposes read-only iteration over stored components.
type ComponentView interface {
	ComponentType() ComponentType
	Len() int
	Has(EntityID) bool
	Get(EntityID) (any, bool)
	Iterate(func(EntityID, any) bool)
}

// Command represents a deferred mutation applied outside system execution.
type Command interface {
	Apply(world *World) error
}

// Logger captures structured log output from systems.
type Logger interface {
	With(key string, value any) Logger
	Info(msg string, args ...any)
	Error(msg string, args ...any)
}

// ResourceContainer holds shared resources accessible to systems.
type ResourceContainer interface {
	Get(name string) (any, bool)
	Set(name string, value any)
	Delete(name string)
	Range(func(string, any) bool)
}

// Tracer coordinates tracing spans for observability tooling.
type Tracer interface {
	Start(ctx context.Context, name string) (context.Context, TraceSpan)
}

// TraceSpan represents an active tracing region.
type TraceSpan interface {
	End()
}

// NOTE: scaffolding only; implementations arrive in future stages.
