# Observability Integration Guide

This guide shows how to consume work-group summaries emitted by the scheduler and route them to logging systems, Prometheus, and SigNoz/OpenTelemetry.

## Structured Logging (Graylog/SigNoz)

Enable JSON logging:

```go
scheduler, _ := ecs.NewScheduler(world)
observerLogger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

scheduler.Builder().WithInstrumentation(ecs.InstrumentationConfig{
    Observation: ecs.ObservationSettings{
        EnableStructuredLogging: true,
        LoggingFormat:          ecs.ObservationLogFormatJSON,
        StructuredLogger:       observerLogger,
    },
})
```

Key fields (Graylog-friendly) include `work_group_id`, `mode`, `async`, `duration_ms`, `systems_*`, and component/resource access arrays.

## Prometheus Metrics

The scheduler ships with an in-memory Prometheus collector you can expose via HTTP:

```go
collector := ecs.NewPrometheusWorkGroupCollector(&ecs.PrometheusCollectorOptions{
    DurationBuckets: []time.Duration{
        1 * time.Millisecond,
        10 * time.Millisecond,
        100 * time.Millisecond,
    },
})

scheduler.Builder().WithInstrumentation(ecs.InstrumentationConfig{
    Observation: ecs.ObservationSettings{
        EnablePrometheus:    true,
        PrometheusCollector: collector,
    },
})

http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
    if err := collector.WriteMetrics(w); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
})
```

### Exported Metrics

- `ecs_work_group_duration_seconds` (summary)
- `ecs_work_group_systems_executed_total`
- `ecs_work_group_systems_skipped_total`
- `ecs_work_group_errors_total`

Each is labelled with `work_group_id`, `mode`, and `async`.

## SigNoz / OpenTelemetry-Compatible JSON

The built-in SigNoz exporter writes OTEL-style JSON spans to any writer:

```go
var sigBuffer bytes.Buffer
sigExporter := ecs.NewSigNozSpanExporter(&ecs.SigNozOptions{
    Writer:      &sigBuffer,
    ServiceName: "ecs-scheduler",
})

scheduler.Builder().WithInstrumentation(ecs.InstrumentationConfig{
    Observation: ecs.ObservationSettings{
        EnableSigNoz:   true,
        SigNozExporter: sigExporter,
    },
})
```

The exported JSON span contains fields such as `service_name`, `name`, timestamps, duration, and attribute dictionaries that mirror component/resource usage and system counts.

## Go Trace (`go tool trace`)

Enabling trace capture uses the built-in `RunWithTrace` helper:

```go
traceFile, _ := os.Create("trace.out")
defer traceFile.Close()

scheduler.Builder().WithInstrumentation(ecs.InstrumentationConfig{
    EnableTrace: true,
})

_ = scheduler.RunWithTrace(ctx, traceFile, func() error {
    return scheduler.Run(ctx, 200, 16*time.Millisecond)
})

// Inspect with:
//   go tool trace trace.out
```

The trace view shows goroutine scheduling plus your work-group execution timeline. Combine it with structured logging and metrics for full observability.

## Combining Observers

You can enable multiple targets simultaneously:

```go
scheduler.Builder().WithInstrumentation(ecs.InstrumentationConfig{
    Observer: customObserver,
    Observation: ecs.ObservationSettings{
        EnableStructuredLogging: true,
        StructuredLogger:        slog.New(slog.NewJSONHandler(os.Stdout, nil)),
        EnablePrometheus:        true,
        EnableSigNoz:            true,
        SigNozOptions: &ecs.SigNozOptions{
            Writer: os.Stdout,
        },
    },
})
```

Observers are executed in order: custom observer (if any), structured logger, Prometheus collector, then SigNoz exporter.

## Summary Fields

Each `WorkGroupSummary` includes:

- `WorkGroupID`, `Mode`, `Async`, `Tick`
- `Duration`, `SystemsTotal`, `SystemsExecuted`, `SystemsSkipped`
- `ComponentReads`, `ComponentWrites`, `ResourceReads`, `ResourceWrites`
- `Error` (if the work group failed)

Use these fields to drive alerts, dashboards, or trace spans in your logging/metrics systems.
