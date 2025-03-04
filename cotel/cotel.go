package cotel

import (
	"context"
	"log/slog"
	"net/http"
	"os"
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
	"go.opentelemetry.io/otel/exporters/zipkin"
	ometric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	otrace "go.opentelemetry.io/otel/trace"
)

var needmetric bool
var promRegister *prometheus.Registry

func Init() error {
	if e := name.HasSelfFullName(); e != nil {
		return e
	}
	traceenv := strings.TrimSpace(strings.ToLower(os.Getenv("TRACE")))
	if traceenv != "" && traceenv != "<TRACE>" && traceenv != "log" && traceenv != "otlp" && traceenv != "zipkin" {
		panic("[cotel] os env TRACE error,must in [\"\",\"log\",\"otlphttp\",\"otlpgrpc\",\"zipkin\"]")
	}
	metricenv := strings.TrimSpace(strings.ToLower(os.Getenv("METRIC")))
	if metricenv != "" && metricenv != "<METRIC>" && metricenv != "log" && metricenv != "otlp" && metricenv != "prometheus" {
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
	if traceenv != "" {
		topts = append(topts, trace.WithSampler(trace.AlwaysSample()))
	} else {
		topts = append(topts, trace.WithSampler(trace.NeverSample()))
	}
	switch traceenv {
	case "log":
		topts = append(topts, trace.WithSyncer(&slogTraceExporter{}))
	case "otlphttp":
		str1 := strings.TrimSpace(strings.ToLower(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")))
		str2 := strings.TrimSpace(strings.ToLower(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")))
		if (str1 == "" || str1 == "<OTEL_EXPORTER_OTLP_TRACES_ENDPOINT>") && (str2 == "" || str2 == "<OTEL_EXPORTER_OTLP_ENDPOINT>") {
			panic("[cotel] os env OTEL_EXPORTER_OTLP_TRACES_ENDPOINT or OTEL_EXPORTER_OTLP_ENDPOINT missing,when os env TRACE is otlp...")
		}
		exporter, e := otlptrace.New(context.Background(), otlptracehttp.NewClient())
		if e != nil {
			panic("[cotel] os env OTEL_EXPORTER_OTLP_TRACES_ENDPOINT or OTEL_EXPORTER_OTLP_ENDPOINT error,when os env TRACE is otlp...")
		}
		topts = append(topts, trace.WithBatcher(exporter))
	case "otlpgrpc":
		str1 := strings.TrimSpace(strings.ToLower(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")))
		str2 := strings.TrimSpace(strings.ToLower(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")))
		if (str1 == "" || str1 == "<OTEL_EXPORTER_OTLP_TRACES_ENDPOINT>") && (str2 == "" || str2 == "<OTEL_EXPORTER_OTLP_ENDPOINT>") {
			panic("[cotel] os env OTEL_EXPORTER_OTLP_TRACES_ENDPOINT or OTEL_EXPORTER_OTLP_ENDPOINT missing,when os env TRACE is otlp...")
		}
		exporter, e := otlptrace.New(context.Background(), otlptracegrpc.NewClient())
		if e != nil {
			panic("[cotel] os env OTEL_EXPORTER_OTLP_TRACES_ENDPOINT or OTEL_EXPORTER_OTLP_ENDPOINT error,when os env TRACE is otlp...")
		}
		topts = append(topts, trace.WithBatcher(exporter))
	case "zipkin":
		str := strings.TrimSpace(strings.ToLower(os.Getenv("ZIPKIN_URL")))
		if str == "" || str == "<ZIPKIN_URL>" {
			panic("[cotel] os env ZIPKIN_URL missing,when os env TRACE is zipkin")
		}
		exporter, e := zipkin.New(str, zipkin.WithLogger(slog.NewLogLogger(slog.Default().Handler(), slog.LevelInfo)))
		if e != nil {
			panic("[cotel] os env ZIPKIN_URL error,when os env TRACE is zipkin")
		}
		topts = append(topts, trace.WithBatcher(exporter))
	}
	otel.SetTracerProvider(trace.NewTracerProvider(topts...))
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
			panic("[cotel] os env OTEL_EXPORTER_OTLP_METRICS_ENDPOINT or OTEL_EXPORTER_OTLP_ENDPOINT missing,when os env METRIC is otlp...")
		}
		exporter, e := otlpmetrichttp.New(context.Background())
		if e != nil {
			panic("[cotel] os env OTEL_EXPORTER_OTLP_METRICS_ENDPOINT or OTEL_EXPORTER_OTLP_ENDPOINT error,when os env METRIC is otlp...")
		}
		mopts = append(mopts, metric.WithReader(metric.NewPeriodicReader(exporter)))
		needmetric = true
	case "otlpgrpc":
		str1 := strings.TrimSpace(strings.ToLower(os.Getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT")))
		str2 := strings.TrimSpace(strings.ToLower(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")))
		if (str1 == "" || str1 == "<OTEL_EXPORTER_OTLP_METRICS_ENDPOINT>") && (str2 == "" || str2 == "<OTEL_EXPORTER_OTLP_ENDPOINT>") {
			panic("[cotel] os env OTEL_EXPORTER_OTLP_METRICS_ENDPOINT or OTEL_EXPORTER_OTLP_ENDPOINT missing,when os env METRIC is otlp...")
		}
		exporter, e := otlpmetricgrpc.New(context.Background())
		if e != nil {
			panic("[cotel] os env OTEL_EXPORTER_OTLP_METRICS_ENDPOINT or OTEL_EXPORTER_OTLP_ENDPOINT error,when os env METRIC is otlp...")
		}
		mopts = append(mopts, metric.WithReader(metric.NewPeriodicReader(exporter)))
		needmetric = true
	case "prometheus":
		promRegister = prometheus.NewRegistry()
		exporter, _ := oprometheus.New(oprometheus.WithoutUnits(), oprometheus.WithRegisterer(promRegister), oprometheus.WithoutCounterSuffixes())
		mopts = append(mopts, metric.WithReader(exporter))
		needmetric = true
	}
	if needmetric {
		otel.SetMeterProvider(metric.NewMeterProvider(mopts...))
		cpuc, _ := otel.Meter("Corelib.host", ometric.WithInstrumentationVersion(version.String())).Float64ObservableGauge("cpu_cur_usage", ometric.WithUnit("%"))
		cpum, _ := otel.Meter("Corelib.host", ometric.WithInstrumentationVersion(version.String())).Float64ObservableGauge("cpu_max_usage", ometric.WithUnit("%"))
		cpua, _ := otel.Meter("Corelib.host", ometric.WithInstrumentationVersion(version.String())).Float64ObservableGauge("cpu_avg_usage", ometric.WithUnit("%"))
		memc, _ := otel.Meter("Corelib.host", ometric.WithInstrumentationVersion(version.String())).Float64ObservableGauge("mem_cur_usage", ometric.WithUnit("%"))
		memm, _ := otel.Meter("Corelib.host", ometric.WithInstrumentationVersion(version.String())).Float64ObservableGauge("mem_max_usage", ometric.WithUnit("%"))
		gc, _ := otel.Meter("Corelib.host", ometric.WithInstrumentationVersion(version.String())).Int64ObservableGauge("gc", ometric.WithUnit("ns"))
		goroutine, _ := otel.Meter("Corelib.host", ometric.WithInstrumentationVersion(version.String())).Int64ObservableGauge("goroutine", ometric.WithUnit("1"))
		thread, _ := otel.Meter("Corelib.host", ometric.WithInstrumentationVersion(version.String())).Int64ObservableGauge("thread", ometric.WithUnit("1"))
		otel.Meter("Corelib.host", ometric.WithInstrumentationVersion(version.String())).RegisterCallback(func(ctx context.Context, s ometric.Observer) error {
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
	}
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
		if needmetric {
			otel.GetMeterProvider().(*metric.MeterProvider).Shutdown(context.Background())
		}
		wg.Done()
	}()
	wg.Wait()
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
