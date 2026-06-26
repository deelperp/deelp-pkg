package observabilidade

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Span abre um span filho a partir do TracerProvider global configurado por
// Iniciar. escopo nomeia o tracer (ex.: "financeiro-service/centro_custo") e
// operacao nomeia o span (ex.: "Criar"). O chamador encerra o span com
// defer span.End() ou via FinalizarSpanErr.
func Span(ctx context.Context, escopo, operacao string) (context.Context, trace.Span) {
	return otel.Tracer(escopo).Start(ctx, operacao)
}

// FinalizarSpanErr encerra o span, registrando o erro e marcando o status como
// Error quando err for não-nil. Pensado para operações de banco e integrações
// que retornam error.
func FinalizarSpanErr(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}
