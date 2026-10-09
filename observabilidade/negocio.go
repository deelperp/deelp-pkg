package observabilidade

import (
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	duracaoEtapa   = HistogramaSegundos("deelp_etapa_duracao_seconds", "Duração das etapas dos fluxos de negócio")
	eventosNegocio = Contador("deelp_negocio_eventos_total", "Eventos de negócio por fluxo e resultado")
)

// MedirEtapa abre um span "fluxo.etapa" e mede a duração. Rótulos (fluxo, etapa) são
// valores fixos do código; nunca passar id, chave ou nome de tenant.
func MedirEtapa(ctx context.Context, fluxo, etapa string, fn func(context.Context) error) error {
	ctx, span := Span(ctx, fluxo, etapa)
	inicio := time.Now()
	err := fn(ctx)
	resultado := "ok"
	if err != nil {
		resultado = "erro"
	}
	duracaoEtapa.Record(ctx, time.Since(inicio).Seconds(), metric.WithAttributes(
		attribute.String("fluxo", fluxo),
		attribute.String("etapa", etapa),
		attribute.String("resultado", resultado),
	))
	FinalizarSpanErr(span, err)
	return err
}

// ContarEvento soma 1 em deelp_negocio_eventos_total. Os atributos extras precisam ter
// cardinalidade baixa e fechada (provedor, tipo, c_stat agrupado).
func ContarEvento(ctx context.Context, fluxo, evento, resultado string, extras ...attribute.KeyValue) {
	attrs := append([]attribute.KeyValue{
		attribute.String("fluxo", fluxo),
		attribute.String("evento", evento),
		attribute.String("resultado", resultado),
	}, extras...)
	eventosNegocio.Add(ctx, 1, metric.WithAttributes(attrs...))
}

func ResultadoDe(err error) string {
	if err != nil {
		return "erro"
	}
	return "ok"
}

const limiteMensagemSpan = 200

// ErroDeMensagem transforma a mensagem de um resultado de negócio em erro para o
// status do span, truncada para não levar texto longo (ou dado do usuário) ao Jaeger.
func ErroDeMensagem(mensagem string) error {
	if r := []rune(mensagem); len(r) > limiteMensagemSpan {
		mensagem = string(r[:limiteMensagemSpan]) + "…"
	}
	return errors.New(mensagem)
}

// PreRegistrarEvento cria a série com valor 0 antes do primeiro evento. Sem isso,
// increase()/rate() do Prometheus ignoram a primeira ocorrência depois de cada deploy,
// e é justamente o evento raro (falha, rejeição) que os alertas precisam enxergar.
func PreRegistrarEvento(fluxo, evento string, resultados ...string) {
	for _, r := range resultados {
		eventosNegocio.Add(context.Background(), 0, metric.WithAttributes(
			attribute.String("fluxo", fluxo),
			attribute.String("evento", evento),
			attribute.String("resultado", r),
		))
	}
}
