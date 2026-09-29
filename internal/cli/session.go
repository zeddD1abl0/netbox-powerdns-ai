package cli

import (
	"context"
	"crypto/rand"
	"log/slog"
	"time"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/logging"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/tracing"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/version"
)

// A session is what a command that reads the configuration runs with.
type session struct {
	cfg      *config.Config
	settings []config.Setting
	log      *slog.Logger
	tracer   trace.Tracer
}

// run loads the configuration, sets up logging and tracing, and runs fn in
// the command's root span. Every line logged with fn's context carries the
// span's trace_id and span_id, and the run's request_id.
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
	// Nothing is exported yet, so shutting down only releases the provider.
	defer func() { _ = tp.Shutdown(context.WithoutCancel(cmd.Context())) }()
	s := &session{cfg: cfg, settings: settings, log: log, tracer: tracing.Tracer(tp)}

	ctx := logging.WithRequestID(cmd.Context(), rand.Text())
	ctx, span := s.tracer.Start(ctx, cmd.CommandPath())
	defer span.End()

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
