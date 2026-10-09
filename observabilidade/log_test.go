package observabilidade

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestLogComTraceInjetaIdsDoSpanAtivo(t *testing.T) {
	prepararTracer(t)
	var buf bytes.Buffer
	logger := slog.New(LogComTrace(slog.NewJSONHandler(&buf, nil)))

	ctx, span := Span(t.Context(), "teste", "op")
	defer span.End()
	logger.InfoContext(ctx, "com span")
	logger.Info("sem contexto")

	linhas := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if !strings.Contains(linhas[0], span.SpanContext().TraceID().String()) {
		t.Fatalf("trace_id ausente: %s", linhas[0])
	}
	if strings.Contains(linhas[1], "trace_id") {
		t.Fatalf("log sem span não deve ter trace_id: %s", linhas[1])
	}
}
