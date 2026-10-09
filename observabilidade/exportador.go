package observabilidade

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type exportadorRedigido struct {
	sdktrace.SpanExporter
}

// RedigirExportador aplica a redação quando o span já está completo. O otelhttp.Transport
// grava url.full depois de Tracer.Start, então só na saída todos os atributos existem.
func RedigirExportador(e sdktrace.SpanExporter) sdktrace.SpanExporter {
	return exportadorRedigido{SpanExporter: e}
}

func (e exportadorRedigido) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	saida := make([]sdktrace.ReadOnlySpan, len(spans))
	for i, s := range spans {
		saida[i] = spanRedigido{ReadOnlySpan: s}
	}
	return e.SpanExporter.ExportSpans(ctx, saida)
}

type spanRedigido struct {
	sdktrace.ReadOnlySpan
}

var atributosDePessoa = map[attribute.Key]bool{
	"client.address": true, "user_agent.original": true, "network.peer.address": true,
	"network.peer.port": true, "http.request.method_original": true,
}

func (s spanRedigido) Attributes() []attribute.KeyValue {
	original := s.ReadOnlySpan.Attributes()
	copia := make([]attribute.KeyValue, 0, len(original))
	for _, kv := range original {
		if atributosDePessoa[kv.Key] {
			continue
		}
		if atributosDeURL[kv.Key] && kv.Value.Type() == attribute.STRING {
			kv = attribute.String(string(kv.Key), sanitizarURL(kv.Value.AsString()))
		}
		copia = append(copia, kv)
	}
	return copia
}

func (s spanRedigido) Events() []sdktrace.Event {
	original := s.ReadOnlySpan.Events()
	copia := make([]sdktrace.Event, len(original))
	for i, ev := range original {
		attrs := make([]attribute.KeyValue, len(ev.Attributes))
		for j, kv := range ev.Attributes {
			if kv.Key == "exception.message" && kv.Value.Type() == attribute.STRING {
				kv = attribute.String("exception.message", redigirTexto(kv.Value.AsString()))
			}
			attrs[j] = kv
		}
		ev.Attributes = attrs
		copia[i] = ev
	}
	return copia
}

func (s spanRedigido) Status() sdktrace.Status {
	st := s.ReadOnlySpan.Status()
	st.Description = redigirTexto(st.Description)
	return st
}
