package cli

import (
	"context"
	"crypto/rand"
	"log/slog"
	"time"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/logging"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/metrics"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/otlp"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/tracing"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/version"
)

// annotationNoExport marks a command that exports no spans: one that only
// shows the configuration, so that it works whatever the otlp keys hold.
const annotationNoExport = "nbpdns/no-otlp-export"

// A session is what a command that reads the configuration runs with.
type session struct {
	cfg      *config.Config
	settings []config.Setting
	log      *slog.Logger
	tracer   trace.Tracer
	// metrics, if set, count the clients' requests. Only `nbpdns serve`
	// has them.
	metrics *metrics.Metrics
}

// run loads the configuration, sets up logging and tracing, and runs fn in
// the command's root span. Every line logged with fn's context carries the
// span's trace_id and span_id, and the run's request_id. If otlp.endpoint is
// set, the spans are exported, and the last of them sent before run returns.
func (a *app) run(cmd *cobra.Command, fn func(ctx context.Context, s *session) error) error {
	cfg, settings, err := a.loader.Load()
	if err != nil {
		return err
	}
	log, err := logging.New(a.stderr, cfg.Log.Format, cfg.Log.Level)
	if err != nil {
		return err
	}
	info := version.Get()
	tp := tracing.NewProvider(info.Version)
	s := &session{cfg: cfg, settings: settings, log: log, tracer: tracing.Tracer(tp)}
	ctx := logging.WithRequestID(cmd.Context(), rand.Text())
	ctx, span := s.tracer.Start(ctx, cmd.CommandPath())
	// The root span ends before the provider shuts down, so that it's among
	// the spans sent.
	defer func() {
		span.End()
		sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.OTLP.Timeout)
		defer cancel()
		if err := tp.Shutdown(sctx); err != nil {
			log.WarnContext(sctx, "couldn't send the last spans to the OpenTelemetry collector", "err", err)
		}
	}()
	// The exporter is made in the root span, so that its warnings carry the
	// command's IDs. A span goes to the processors registered when it ends,
	// so the root span is exported too.
	if cmd.Annotations[annotationNoExport] == "" {
		exp, err := otlp.NewExporter(ctx, cfg.OTLP, log)
		if err != nil {
			return err
		}
		if exp != nil {
			tp.RegisterSpanProcessor(sdktrace.NewBatchSpanProcessor(exp, sdktrace.WithExportTimeout(cfg.OTLP.Timeout)))
			// The batcher reports a failed export here, from its own
			// goroutine, not to its caller.
			otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
				log.WarnContext(ctx, "couldn't export spans to the OpenTelemetry collector", "err", err)
			}))
		}
	}

	start := time.Now()
	log.DebugContext(ctx, "command started", "command", cmd.CommandPath(), "version", info.Version)
	err = fn(ctx, s)
	outcome := "success"
	if err != nil {
		outcome = "failure"
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	log.DebugContext(ctx, "command finished", "outcome", outcome, "duration_seconds", time.Since(start).Seconds())
	return err
}
