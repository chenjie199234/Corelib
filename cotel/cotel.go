package cotel

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/chenjie199234/Corelib/util/host"
	"github.com/chenjie199234/Corelib/util/name"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	ometric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	otrace "go.opentelemetry.io/otel/trace"
)

var traceexporter string
var metricexporter string

func init() {
	traceenv := strings.ToLower(os.Getenv("TRACE"))
	if traceenv != "" && traceenv != "<TRACE>" && traceenv != "log" && traceenv != "oltp" && traceenv != "zipkin" {
		panic("[cotel] os env TRACE error,must in [\"\",\"log\",\"oltp\",\"zipkin\"]")
	} else {
		traceexporter = traceenv
	}
	metricenv := strings.ToLower(os.Getenv("METRIC"))
	if metricenv != "" && metricenv != "<METRIC>" && metricenv != "log" && metricenv != "oltp" && metricenv != "prometheus" {
		panic("[cotel] os env METRIC error,must in [\"\",\"log\",\"oltp\",\"prometheus\"]")
	} else {
		metricexporter = metricenv
	}
}
func Init() error {
	if e := name.HasSelfFullName(); e != nil {
		return e
	}
	resources := resource.NewSchemaless(
		attribute.String("service.name", name.GetSelfFullName()),
		attribute.String("host.id", host.Hostname),
		attribute.String("host.ip", host.Hostip))
	//trace
	otel.SetTextMapPropagator(propagation.TraceContext{})
	topts := make([]trace.TracerProviderOption, 0, 3)
	topts = append(topts, trace.WithResource(resources))
	if traceexporter != "" {
		topts = append(topts, trace.WithSampler(trace.AlwaysSample()))
	} else {
		topts = append(topts, trace.WithSampler(trace.NeverSample()))
	}
	switch traceexporter {
	case "log":
		topts = append(topts, trace.WithSyncer(&slogTraceExporter{}))
	case "oltp":
		panic("[cotel] trace not support oltp now")
	case "zipkin":
		panic("[cotel] trace not support zipkin now")
	}
	otel.SetTracerProvider(trace.NewTracerProvider(topts...))
	//metric
	mopts := make([]metric.Option, 0, 2)
	mopts = append(mopts, metric.WithResource(resources))
	switch metricexporter {
	case "log":
		mopts = append(mopts, metric.WithReader(metric.NewPeriodicReader(&slogMetricExporter{})))
	case "oltp":
		panic("[cotel] metric not support oltp now")
	case "prometheus":
		panic("[cotel] metric not support prometheus now")
	}
	otel.SetMeterProvider(metric.NewMeterProvider(mopts...))
	cpuc, _ := otel.Meter("Host").Float64ObservableGauge("cpu_cur_usage", ometric.WithUnit("%"))
	cpum, _ := otel.Meter("Host").Float64ObservableGauge("cpu_max_usage", ometric.WithUnit("%"))
	cpua, _ := otel.Meter("Host").Float64ObservableGauge("cpu_avg_usage", ometric.WithUnit("%"))
	memc, _ := otel.Meter("Host").Float64ObservableGauge("mem_cur_usage", ometric.WithUnit("%"))
	memm, _ := otel.Meter("Host").Float64ObservableGauge("mem_max_usage", ometric.WithUnit("%"))
	gc, _ := otel.Meter("Host").Int64ObservableGauge("gc", ometric.WithUnit("ns"))
	goroutine, _ := otel.Meter("Host").Int64ObservableGauge("goroutine", ometric.WithUnit("1"))
	thread, _ := otel.Meter("Host").Int64ObservableGauge("thread", ometric.WithUnit("1"))
	otel.Meter("Host").RegisterCallback(func(ctx context.Context, s ometric.Observer) error {
		lastcpu, maxcpu, avgcpu := collectCPU()
		totalmem, lastmem, maxmem := collectMEM()
		goroutinenum, threadnum, gctime := getGo()
		s.ObserveFloat64(cpuc, lastcpu*100.0)
		s.ObserveFloat64(cpum, maxcpu*100.0)
		s.ObserveFloat64(cpua, avgcpu*100.0)
		s.ObserveFloat64(memc, float64(lastmem)/float64(totalmem)*100.0)
		s.ObserveFloat64(memm, float64(maxmem)/float64(totalmem)*100.0)
		s.ObserveInt64(gc, int64(gctime))
		s.ObserveInt64(goroutine, int64(goroutinenum))
		s.ObserveInt64(thread, int64(threadnum))
		return nil
	}, cpuc, cpum, cpua, memc, memm, gc, goroutine, thread)
	return nil
}

func Stop() {
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		otel.GetTracerProvider().(*trace.TracerProvider).Shutdown(context.Background())
		wg.Done()
	}()
	go func() {
		otel.GetMeterProvider().(*metric.MeterProvider).Shutdown(context.Background())
		wg.Done()
	}()
	wg.Wait()
}

func TraceIDFromContext(ctx context.Context) string {
	span := otrace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		return span.SpanContext().TraceID().String()
	}
	return ""
}

type slogTraceExporter struct {
	stopped atomic.Bool
}

func (s *slogTraceExporter) ExportSpans(ctx context.Context, spans []trace.ReadOnlySpan) error {
	if s.stopped.Load() {
		return nil
	}
	if len(spans) == 0 {
		return nil
	}
	stubs := tracetest.SpanStubsFromReadOnlySpans(spans)
	for _, stub := range stubs {
		slog.Info("trace",
			slog.String("Name", stub.Name),
			slog.Any("SpanContext", stub.SpanContext),
			slog.Any("Parent", stub.Parent),
			slog.Int("SpanKind", int(stub.SpanKind)),
			slog.Int("ChildSpanCount", stub.ChildSpanCount),
			slog.Time("StartTime", stub.StartTime),
			slog.Time("EndTime", stub.EndTime),
			slog.Any("Resource", stub.Resource),
			slog.Any("Attributes", stub.Attributes),
			slog.Any("Status", stub.Status))
	}
	return nil
}

func (s *slogTraceExporter) Shutdown(ctx context.Context) error {
	s.stopped.Store(true)
	return nil
}

type slogMetricExporter struct {
	stopped atomic.Bool
}

func (s *slogMetricExporter) Temporality(p metric.InstrumentKind) metricdata.Temporality {
	return metric.DefaultTemporalitySelector(p)
}
func (s *slogMetricExporter) Aggregation(p metric.InstrumentKind) metric.Aggregation {
	return metric.DefaultAggregationSelector(p)
}
func (s *slogMetricExporter) Export(ctx context.Context, metrics *metricdata.ResourceMetrics) error {
	if s.stopped.Load() {
		return nil
	}
	attrs := make([]any, 0, 10)
	attrs = append(attrs, slog.Any("Resource", metrics.Resource))
	for _, m := range metrics.ScopeMetrics {
		gattrs := make([]any, 0, len(m.Metrics))
		for _, mm := range m.Metrics {
			gattrs = append(gattrs, slog.Any(mm.Name+"("+mm.Unit+")", mm.Data))
		}
		attrs = append(attrs, slog.Group(m.Scope.Name, gattrs...))
	}
	slog.Info("metric", attrs...)
	return nil
}
func (s *slogMetricExporter) ForceFlush(context.Context) error {
	return nil
}
func (s *slogMetricExporter) Shutdown(context.Context) error {
	s.stopped.Store(true)
	return nil
}
