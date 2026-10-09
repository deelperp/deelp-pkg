package observabilidade

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

type handlerComTrace struct {
	slog.Handler
}

// LogComTrace acrescenta trace_id e span_id a todo registro emitido com contexto
// (slog.InfoContext etc.), o que liga log e trace no Loki e no Jaeger.
func LogComTrace(base slog.Handler) slog.Handler {
	return handlerComTrace{Handler: base}
}

func (h handlerComTrace) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
	}
	return h.Handler.Handle(ctx, r)
}

func (h handlerComTrace) WithAttrs(attrs []slog.Attr) slog.Handler {
	return handlerComTrace{Handler: h.Handler.WithAttrs(attrs)}
}

func (h handlerComTrace) WithGroup(nome string) slog.Handler {
	return handlerComTrace{Handler: h.Handler.WithGroup(nome)}
}
