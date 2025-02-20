package cotel

import (
	"context"
	"log/slog"
	"os"
	"sync/atomic"

	"github.com/chenjie199234/Corelib/util/host"
	"github.com/chenjie199234/Corelib/util/name"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	otrace "go.opentelemetry.io/otel/trace"
	// "go.opentelemetry.io/otel/sdk/metric"
)

var logtrace bool

func init() {
	if str := os.Getenv("LOG_TRACE"); str != "" && str != "<LOG_TRACE>" && str != "0" && str != "1" {
		panic("[cotel] os env LOG_TRACE error,must in [0,1]")
	} else {
		logtrace = str == "1"
	}
}
func LogTrace() bool {
	return logtrace
}

func Init() error {
	if e := name.HasSelfFullName(); e != nil {
		return e
	}
	//trace
	otel.SetTextMapPropagator(propagation.TraceContext{})
	var sampler trace.Sampler
	if logtrace {
		sampler = trace.AlwaysSample()
	} else {
		sampler = trace.NeverSample()
	}
	tp := trace.NewTracerProvider(
		//when use the log as the exporter,we can use syncer
		//when use other exporter,we should use batcher(more effective)
		trace.WithSyncer(&slogTraceExporter{}),
		trace.WithResource(resource.NewSchemaless(
			attribute.String("service.name", name.GetSelfFullName()),
			attribute.String("host.id", host.Hostname),
			attribute.String("host.ip", host.Hostip))),
		trace.WithSampler(sampler),
	)
	otel.SetTracerProvider(tp)
	//metric
	// metric.NewMeterProvider()
	return nil
}
func Stop() {
	otel.GetTracerProvider().(*trace.TracerProvider).ForceFlush(context.Background())
	otel.GetTracerProvider().(*trace.TracerProvider).Shutdown(context.Background())
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
