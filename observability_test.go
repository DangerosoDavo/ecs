package ecs

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestPrometheusWorkGroupCollectorWritesMetrics(t *testing.T) {
	collector := NewPrometheusWorkGroupCollector(&PrometheusCollectorOptions{})
	cimpl, ok := collector.(*PrometheusWorkGroupCollector)
	if !ok {
		t.Fatalf("expected PrometheusWorkGroupCollector implementation")
	}

	summary := WorkGroupSummary{
		WorkGroupID:     "wg",
		Mode:            WorkGroupModeSynchronized,
		Async:           false,
		Tick:            42,
		Duration:        5 * time.Millisecond,
		SystemsTotal:    2,
		SystemsExecuted: 2,
		SystemsSkipped:  0,
	}

	collector.ObserveWorkGroup(summary)

	var buf bytes.Buffer
	if err := cimpl.WriteMetrics(&buf); err != nil {
		t.Fatalf("write metrics: %v", err)
	}
	metrics := buf.String()
	if !strings.Contains(metrics, "ecs_work_group_duration_seconds_sum") {
		t.Fatalf("expected duration metric in %q", metrics)
	}
	if !strings.Contains(metrics, "ecs_work_group_systems_executed_total") {
		t.Fatalf("expected executed metric in %q", metrics)
	}
}

func TestSigNozSpanExporterWritesJSON(t *testing.T) {
	var buf bytes.Buffer
	exporter := NewSigNozSpanExporter(&SigNozOptions{Writer: &buf, ServiceName: "ecs-test"})

	summary := WorkGroupSummary{
		WorkGroupID:     "wg",
		Mode:            WorkGroupModeAsync,
		Async:           true,
		Tick:            13,
		Duration:        10 * time.Millisecond,
		SystemsTotal:    1,
		SystemsExecuted: 1,
		ResourceReads:   []string{"clock"},
	}

	exporter.ExportWorkGroup(summary)

	if buf.Len() == 0 {
		t.Fatalf("expected exporter to write output")
	}

	var payload map[string]any
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	attrs, ok := payload["attributes"].(map[string]any)
	if !ok {
		t.Fatalf("attributes missing in payload: %v", payload)
	}
	if attrs["work_group_id"] != "wg" {
		t.Fatalf("unexpected work_group_id: %v", attrs["work_group_id"])
	}
}
