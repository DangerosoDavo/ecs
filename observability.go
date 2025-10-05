package ecs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

type compositeObserver struct {
	observers []SchedulerObserver
}

func (c compositeObserver) WorkGroupCompleted(summary WorkGroupSummary) {
	for _, observer := range c.observers {
		observer.WorkGroupCompleted(summary)
	}
}

type loggingObserver struct {
	logger Logger
	format ObservationLogFormat
}

func newLoggingObserver(logger Logger, format ObservationLogFormat) SchedulerObserver {
	if logger == nil {
		return noopObserver{}
	}
	if format != ObservationLogFormatKeyValue {
		format = ObservationLogFormatJSON
	}
	return loggingObserver{logger: logger, format: format}
}

func (o loggingObserver) WorkGroupCompleted(summary WorkGroupSummary) {
	switch o.format {
	case ObservationLogFormatKeyValue:
		o.logKeyValue(summary)
	default:
		o.logJSON(summary)
	}
}

func (o loggingObserver) logJSON(summary WorkGroupSummary) {
	payload := map[string]any{
		"work_group_id":    summary.WorkGroupID,
		"mode":             summary.Mode,
		"async":            summary.Async,
		"tick":             summary.Tick,
		"duration_ms":      float64(summary.Duration) / float64(time.Millisecond),
		"systems_total":    summary.SystemsTotal,
		"systems_executed": summary.SystemsExecuted,
		"systems_skipped":  summary.SystemsSkipped,
		"component_reads":  summary.ComponentReads,
		"component_writes": summary.ComponentWrites,
		"resource_reads":   summary.ResourceReads,
		"resource_writes":  summary.ResourceWrites,
	}
	if summary.Error != nil {
		payload["error"] = summary.Error.Error()
	}
	data, err := json.Marshal(payload)
	if err != nil {
		o.logger.With("work_group", summary.WorkGroupID).Error("workgroup summary marshal error", "err", err)
		return
	}
	o.logger.Info(string(data))
}

func (o loggingObserver) logKeyValue(summary WorkGroupSummary) {
	builder := o.logger.With("work_group", summary.WorkGroupID)
	args := []any{
		"mode", summary.Mode,
		"async", summary.Async,
		"tick", summary.Tick,
		"duration", summary.Duration,
		"systems_total", summary.SystemsTotal,
		"systems_executed", summary.SystemsExecuted,
		"systems_skipped", summary.SystemsSkipped,
		"component_reads", strings.Join(convertComponentTypes(summary.ComponentReads), ","),
		"component_writes", strings.Join(convertComponentTypes(summary.ComponentWrites), ","),
		"resource_reads", strings.Join(summary.ResourceReads, ","),
		"resource_writes", strings.Join(summary.ResourceWrites, ","),
	}
	if summary.Error != nil {
		args = append(args, "error", summary.Error.Error())
	}
	builder.Info("workgroup summary", args...)
}

type prometheusObserver struct {
	collector PrometheusCollector
}

func newPrometheusObserver(collector PrometheusCollector) SchedulerObserver {
	if collector == nil {
		return noopObserver{}
	}
	return prometheusObserver{collector: collector}
}

func (o prometheusObserver) WorkGroupCompleted(summary WorkGroupSummary) {
	o.collector.ObserveWorkGroup(summary)
}

type sigNozObserver struct {
	exporter SigNozExporter
}

func newSigNozObserver(exporter SigNozExporter) SchedulerObserver {
	if exporter == nil {
		return noopObserver{}
	}
	return sigNozObserver{exporter: exporter}
}

func (o sigNozObserver) WorkGroupCompleted(summary WorkGroupSummary) {
	o.exporter.ExportWorkGroup(summary)
}

func convertComponentTypes(types []ComponentType) []string {
	if len(types) == 0 {
		return nil
	}
	out := make([]string, 0, len(types))
	for _, t := range types {
		out = append(out, string(t))
	}
	sort.Strings(out)
	return out
}

func buildObserverChain(logger Logger, cfg InstrumentationConfig) SchedulerObserver {
	var observers []SchedulerObserver

	if cfg.Observer != nil {
		observers = append(observers, cfg.Observer)
	}

	obs := cfg.Observation

	if obs.EnableStructuredLogging {
		structuredLogger := obs.StructuredLogger
		if structuredLogger == nil {
			structuredLogger = logger
		}
		observers = append(observers, newLoggingObserver(structuredLogger, obs.LoggingFormat))
	}

	if obs.EnablePrometheus {
		collector := obs.PrometheusCollector
		if collector == nil {
			collector = NewPrometheusWorkGroupCollector(obs.PrometheusOptions)
		}
		if collector != nil {
			observers = append(observers, newPrometheusObserver(collector))
		}
	}

	if obs.EnableSigNoz {
		exporter := obs.SigNozExporter
		if exporter == nil {
			exporter = NewSigNozSpanExporter(obs.SigNozOptions)
		}
		if exporter != nil {
			observers = append(observers, newSigNozObserver(exporter))
		}
	}

	if len(observers) == 0 {
		return noopObserver{}
	}
	if len(observers) == 1 {
		return observers[0]
	}
	return compositeObserver{observers: observers}
}

type PrometheusWorkGroupCollector struct {
	options *PrometheusCollectorOptions
	mu      sync.Mutex
	samples map[prometheusKey]*prometheusSample
}

type prometheusKey struct {
	WorkGroupID string
	Mode        string
	Async       bool
}

type prometheusSample struct {
	durationSum   float64
	durationCount float64
	buckets       []float64
	executed      float64
	skipped       float64
	errors        float64
}

func NewPrometheusWorkGroupCollector(opts *PrometheusCollectorOptions) PrometheusCollector {
	if opts == nil {
		opts = &PrometheusCollectorOptions{}
	}
	return &PrometheusWorkGroupCollector{
		options: opts,
		samples: make(map[prometheusKey]*prometheusSample),
	}
}

func (c *PrometheusWorkGroupCollector) ObserveWorkGroup(summary WorkGroupSummary) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := prometheusKey{WorkGroupID: string(summary.WorkGroupID), Mode: modeLabel(summary.Mode), Async: summary.Async}
	sample, ok := c.samples[key]
	if !ok {
		sample = &prometheusSample{}
		if buckets := c.options.DurationBuckets; len(buckets) > 0 {
			sample.buckets = make([]float64, len(buckets))
		}
		c.samples[key] = sample
	}
	durSeconds := summary.Duration.Seconds()
	sample.durationSum += durSeconds
	sample.durationCount++
	for i := range sample.buckets {
		if durSeconds <= c.options.DurationBuckets[i].Seconds() {
			sample.buckets[i]++
		}
	}
	sample.executed += float64(summary.SystemsExecuted)
	sample.skipped += float64(summary.SystemsSkipped)
	if summary.Error != nil {
		sample.errors++
	}

	if writer := c.options.Writer; writer != nil {
		_ = c.writeMetricsLocked(writer)
	}
}

func (c *PrometheusWorkGroupCollector) WriteMetrics(w io.Writer) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeMetricsLocked(w)
}

func (c *PrometheusWorkGroupCollector) writeMetricsLocked(w io.Writer) error {
	if w == nil {
		return nil
	}
	var buf bytes.Buffer
	buf.WriteString("# HELP ecs_work_group_duration_seconds Work group execution duration.\n")
	buf.WriteString("# TYPE ecs_work_group_duration_seconds summary\n")
	keys := make([]prometheusKey, 0, len(c.samples))
	for key := range c.samples {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].WorkGroupID == keys[j].WorkGroupID {
			if keys[i].Mode == keys[j].Mode {
				return !keys[i].Async && keys[j].Async
			}
			return keys[i].Mode < keys[j].Mode
		}
		return keys[i].WorkGroupID < keys[j].WorkGroupID
	})

	for _, key := range keys {
		sample := c.samples[key]
		labels := fmt.Sprintf("work_group_id=\"%s\",mode=\"%s\",async=\"%t\"", key.WorkGroupID, key.Mode, key.Async)
		buf.WriteString(fmt.Sprintf("ecs_work_group_duration_seconds_sum{%s} %f\n", labels, sample.durationSum))
		buf.WriteString(fmt.Sprintf("ecs_work_group_duration_seconds_count{%s} %f\n", labels, sample.durationCount))
		if len(sample.buckets) > 0 {
			for i, bucket := range sample.buckets {
				le := c.options.DurationBuckets[i].Seconds()
				buf.WriteString(fmt.Sprintf("ecs_work_group_duration_seconds_bucket{%s,le=\"%.6f\"} %f\n", labels, le, bucket))
			}
		}
	}

	buf.WriteString("# HELP ecs_work_group_systems_executed_total Systems executed per work group.\n")
	buf.WriteString("# TYPE ecs_work_group_systems_executed_total counter\n")
	for _, key := range keys {
		sample := c.samples[key]
		labels := fmt.Sprintf("work_group_id=\"%s\",mode=\"%s\",async=\"%t\"", key.WorkGroupID, key.Mode, key.Async)
		buf.WriteString(fmt.Sprintf("ecs_work_group_systems_executed_total{%s} %f\n", labels, sample.executed))
	}

	buf.WriteString("# HELP ecs_work_group_systems_skipped_total Systems skipped per work group.\n")
	buf.WriteString("# TYPE ecs_work_group_systems_skipped_total counter\n")
	for _, key := range keys {
		sample := c.samples[key]
		labels := fmt.Sprintf("work_group_id=\"%s\",mode=\"%s\",async=\"%t\"", key.WorkGroupID, key.Mode, key.Async)
		buf.WriteString(fmt.Sprintf("ecs_work_group_systems_skipped_total{%s} %f\n", labels, sample.skipped))
	}

	buf.WriteString("# HELP ecs_work_group_errors_total Work group error count.\n")
	buf.WriteString("# TYPE ecs_work_group_errors_total counter\n")
	for _, key := range keys {
		sample := c.samples[key]
		labels := fmt.Sprintf("work_group_id=\"%s\",mode=\"%s\",async=\"%t\"", key.WorkGroupID, key.Mode, key.Async)
		buf.WriteString(fmt.Sprintf("ecs_work_group_errors_total{%s} %f\n", labels, sample.errors))
	}

	_, err := w.Write(buf.Bytes())
	return err
}

type SigNozSpanExporter struct {
	opts *SigNozOptions
	mu   sync.Mutex
}

func NewSigNozSpanExporter(opts *SigNozOptions) SigNozExporter {
	if opts == nil {
		opts = &SigNozOptions{}
	}
	if opts.ServiceName == "" {
		opts.ServiceName = "ecs-scheduler"
	}
	return &SigNozSpanExporter{opts: opts}
}

func (e *SigNozSpanExporter) ExportWorkGroup(summary WorkGroupSummary) {
	if e.opts.Writer == nil {
		return
	}
	span := map[string]any{
		"service_name": e.opts.ServiceName,
		"name":         fmt.Sprintf("workgroup:%s", summary.WorkGroupID),
		"timestamp":    time.Now().UnixNano(),
		"duration_ms":  float64(summary.Duration) / float64(time.Millisecond),
		"attributes": map[string]any{
			"work_group_id":    summary.WorkGroupID,
			"mode":             modeLabel(summary.Mode),
			"async":            summary.Async,
			"tick":             summary.Tick,
			"systems_total":    summary.SystemsTotal,
			"systems_executed": summary.SystemsExecuted,
			"systems_skipped":  summary.SystemsSkipped,
			"component_reads":  summary.ComponentReads,
			"component_writes": summary.ComponentWrites,
			"resource_reads":   summary.ResourceReads,
			"resource_writes":  summary.ResourceWrites,
		},
	}
	if summary.Error != nil {
		span["error"] = summary.Error.Error()
	}
	payload, err := json.Marshal(span)
	if err != nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	_, _ = e.opts.Writer.Write(append(payload, '\n'))
}

func modeLabel(mode WorkGroupMode) string {
	if mode == WorkGroupModeAsync {
		return "async"
	}
	return "sync"
}
