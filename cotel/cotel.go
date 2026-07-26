package cotel

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/chenjie199234/Corelib/internal/version"
	"github.com/chenjie199234/Corelib/util/host"
	"github.com/chenjie199234/Corelib/util/name"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	oprometheus "go.opentelemetry.io/otel/exporters/prometheus"
	ometric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	otrace "go.opentelemetry.io/otel/trace"
)

var status atomic.Bool
var tp *trace.TracerProvider
var mp *metric.MeterProvider
var needmetric bool
var promRegister *prometheus.Registry

func Init() error {
	if e := name.HasSelfFullName(); e != nil {
		return e
	}
	if status.Swap(true) {
		return nil
	}
	traceenv := strings.TrimSpace(strings.ToLower(os.Getenv("TRACE")))
	if traceenv == "<TRACE>" {
		traceenv = ""
	}
	if traceenv != "" && traceenv != "log" && traceenv != "otlphttp" && traceenv != "otlpgrpc" {
		panic("[cotel] os env TRACE error,must in [\"\",\"log\",\"otlphttp\",\"otlpgrpc\"]")
	}
	metricenv := strings.TrimSpace(strings.ToLower(os.Getenv("METRIC")))
	if metricenv == "<METRIC>" {
		metricenv = ""
	}
	if metricenv != "" && metricenv != "log" && metricenv != "otlphttp" && metricenv != "otlpgrpc" && metricenv != "prometheus" {
		panic("[cotel] os env METRIC error,must in [\"\",\"log\",\"otlphttp\",\"otlpgrpc\",\"prometheus\"]")
	}
	resources := resource.NewSchemaless(
		attribute.String("service.name", name.GetSelfFullName()),
		attribute.String("host.id", host.Hostname),
		attribute.String("host.ip", host.Hostip))
	//trace
	otel.SetTextMapPropagator(propagation.TraceContext{})
	topts := make([]trace.TracerProviderOption, 0, 3)
	topts = append(topts, trace.WithResource(resources))
	topts = append(topts, trace.WithSampler(trace.AlwaysSample()))
	switch traceenv {
	case "":
		topts[len(topts)-1] = trace.WithSampler(trace.NeverSample())
	case "log":
		topts = append(topts, trace.WithSyncer(&slogTraceExporter{}))
	case "otlphttp":
		str1 := strings.TrimSpace(strings.ToLower(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")))
		str2 := strings.TrimSpace(strings.ToLower(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")))
		if (str1 == "" || str1 == "<OTEL_EXPORTER_OTLP_TRACES_ENDPOINT>") && (str2 == "" || str2 == "<OTEL_EXPORTER_OTLP_ENDPOINT>") {
			panic("[cotel] os env OTEL_EXPORTER_OTLP_TRACES_ENDPOINT or OTEL_EXPORTER_OTLP_ENDPOINT missing,when os env TRACE is otlphttp")
		}
		exporter, e := otlptrace.New(context.Background(), otlptracehttp.NewClient())
		if e != nil {
			panic("[cotel] create trace otlphttp exporter failed,error: " + e.Error())
		}
		topts = append(topts, trace.WithBatcher(exporter))
	case "otlpgrpc":
		str1 := strings.TrimSpace(strings.ToLower(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")))
		str2 := strings.TrimSpace(strings.ToLower(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")))
		if (str1 == "" || str1 == "<OTEL_EXPORTER_OTLP_TRACES_ENDPOINT>") && (str2 == "" || str2 == "<OTEL_EXPORTER_OTLP_ENDPOINT>") {
			panic("[cotel] os env OTEL_EXPORTER_OTLP_TRACES_ENDPOINT or OTEL_EXPORTER_OTLP_ENDPOINT missing,when os env TRACE is otlpgrpc")
		}
		exporter, e := otlptrace.New(context.Background(), otlptracegrpc.NewClient())
		if e != nil {
			panic("[cotel] create trace otlpgrpc exporter failed,error: " + e.Error())
		}
		topts = append(topts, trace.WithBatcher(exporter))
	}
	tp = trace.NewTracerProvider(topts...)
	otel.SetTracerProvider(tp)
	//metric
	mopts := make([]metric.Option, 0, 2)
	mopts = append(mopts, metric.WithResource(resources))
	switch metricenv {
	case "log":
		mopts = append(mopts, metric.WithReader(metric.NewPeriodicReader(&slogMetricExporter{})))
		needmetric = true
	case "otlphttp":
		str1 := strings.TrimSpace(strings.ToLower(os.Getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT")))
		str2 := strings.TrimSpace(strings.ToLower(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")))
		if (str1 == "" || str1 == "<OTEL_EXPORTER_OTLP_METRICS_ENDPOINT>") && (str2 == "" || str2 == "<OTEL_EXPORTER_OTLP_ENDPOINT>") {
			panic("[cotel] os env OTEL_EXPORTER_OTLP_METRICS_ENDPOINT or OTEL_EXPORTER_OTLP_ENDPOINT missing,when os env METRIC is otlphttp")
		}
		exporter, e := otlpmetrichttp.New(context.Background())
		if e != nil {
			panic("[cotel] create metric otlphttp exporter failed,error: " + e.Error())
		}
		mopts = append(mopts, metric.WithReader(metric.NewPeriodicReader(exporter)))
		needmetric = true
	case "otlpgrpc":
		str1 := strings.TrimSpace(strings.ToLower(os.Getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT")))
		str2 := strings.TrimSpace(strings.ToLower(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")))
		if (str1 == "" || str1 == "<OTEL_EXPORTER_OTLP_METRICS_ENDPOINT>") && (str2 == "" || str2 == "<OTEL_EXPORTER_OTLP_ENDPOINT>") {
			panic("[cotel] os env OTEL_EXPORTER_OTLP_METRICS_ENDPOINT or OTEL_EXPORTER_OTLP_ENDPOINT missing,when os env METRIC is otlpgrpc")
		}
		exporter, e := otlpmetricgrpc.New(context.Background())
		if e != nil {
			panic("[cotel] create metric otlpgrpc exporter failed,error: " + e.Error())
		}
		mopts = append(mopts, metric.WithReader(metric.NewPeriodicReader(exporter)))
		needmetric = true
	case "prometheus":
		promRegister = prometheus.NewRegistry()
		exporter, e := oprometheus.New(oprometheus.WithRegisterer(promRegister))
		if e != nil {
			panic("[cotel] create metric prometheus exporter failed,error: " + e.Error())
		}
		mopts = append(mopts, metric.WithReader(exporter))
		needmetric = true
	}
	if needmetric {
		mp = metric.NewMeterProvider(mopts...)
		otel.SetMeterProvider(mp)
		gc, _ := otel.Meter("Corelib.host", ometric.WithInstrumentationVersion(version.String())).Int64ObservableCounter("gc", ometric.WithUnit("ns"))
		goroutine, _ := otel.Meter("Corelib.host", ometric.WithInstrumentationVersion(version.String())).Int64ObservableGauge("goroutine", ometric.WithUnit("1"))
		thread, _ := otel.Meter("Corelib.host", ometric.WithInstrumentationVersion(version.String())).Int64ObservableGauge("thread", ometric.WithUnit("1"))
		cpu, _ := otel.Meter("Corelib.host", ometric.WithInstrumentationVersion(version.String())).Float64ObservableGauge("cpu_usage", ometric.WithUnit("1"))
		mem, _ := otel.Meter("Corelib.host", ometric.WithInstrumentationVersion(version.String())).Float64ObservableGauge("mem_usage", ometric.WithUnit("1"))
		otel.Meter("Corelib.host", ometric.WithInstrumentationVersion(version.String())).RegisterCallback(func(ctx context.Context, s ometric.Observer) error {
			_, cpuusage, _, _, memusage, _ := GetCpuMemUsage()
			s.ObserveFloat64(cpu, cpuusage)
			s.ObserveFloat64(mem, memusage)
			gcinfo := &debug.GCStats{}
			debug.ReadGCStats(gcinfo)
			s.ObserveInt64(gc, gcinfo.PauseTotal.Nanoseconds())
			s.ObserveInt64(goroutine, int64(runtime.NumGoroutine()))
			threadnum, _ := runtime.ThreadCreateProfile(nil)
			s.ObserveInt64(thread, int64(threadnum))
			return nil
		}, cpu, mem, gc, goroutine, thread)
	}
	return nil
}

func Stop() {
	if needmetric {
		wg := sync.WaitGroup{}
		wg.Go(func() {
			tp.Shutdown(context.Background())
		})
		wg.Go(func() {
			mp.Shutdown(context.Background())
		})
		wg.Wait()
	} else {
		tp.Shutdown(context.Background())
	}
}

func NeedMetric() bool {
	return needmetric
}

func GetPrometheusHandler() http.Handler {
	if promRegister == nil {
		return nil
	}
	return promhttp.HandlerFor(promRegister, promhttp.HandlerOpts{ErrorLog: slog.NewLogLogger(slog.Default().Handler(), slog.LevelInfo)})
}

func TraceIDFromContext(ctx context.Context) string {
	span := otrace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		return span.SpanContext().TraceID().String()
	}
	return ""
}

// copy the trace info from ctx to a new Context(without deadline)
func CloneTrace(ctx context.Context) context.Context {
	return otrace.ContextWithSpan(context.Background(), otrace.SpanFromContext(ctx))
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
	for _, ro := range spans {
		if ro == nil {
			continue
		}
		slog.Info("trace",
			slog.String("Name", ro.Name()),
			slog.Any("SpanContext", ro.SpanContext()),
			slog.Any("Parent", ro.Parent()),
			slog.Int("SpanKind", int(ro.SpanKind())),
			slog.Int("ChildSpanCount", ro.ChildSpanCount()),
			slog.Time("StartTime", ro.StartTime()),
			slog.Time("EndTime", ro.EndTime()),
			slog.Any("Resource", ro.Resource()),
			slog.Any("Attributes", ro.Attributes()),
			slog.Any("Status", ro.Status()))
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
