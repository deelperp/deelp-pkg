package observabilidade

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	duracaoTicker   = HistogramaSegundos("deelp_ticker_rodada_duracao_seconds", "Duração de cada rodada de job periódico")
	ultimaExecucao  = Medidor("deelp_ticker_ultima_execucao_timestamp", "Unix time do fim da última rodada do job")
	intervaloTicker = Medidor("deelp_ticker_intervalo_seconds", "Intervalo esperado entre rodadas do job")
	falhasTicker    = Contador("deelp_ticker_falhas_total", "Rodadas de job periódico que terminaram com erro")
)

// RodadaTicker executa uma rodada de job periódico com span, duração, resultado e
// timestamp da última execução (base do alerta de job parado).
func RodadaTicker(ctx context.Context, job string, intervalo time.Duration, fn func(context.Context) error) error {
	ctx, span := Span(ctx, "ticker", job)
	inicio := time.Now()
	jobAttr := metric.WithAttributes(attribute.String("rotina", job))
	falhasTicker.Add(ctx, 0, jobAttr)
	registrar := func(err error) {
		duracaoTicker.Record(ctx, time.Since(inicio).Seconds(), metric.WithAttributes(
			attribute.String("rotina", job), attribute.String("resultado", ResultadoDe(err))))
		ultimaExecucao.Record(ctx, float64(time.Now().Unix()), jobAttr)
		intervaloTicker.Record(ctx, intervalo.Seconds(), jobAttr)
		if err != nil {
			falhasTicker.Add(ctx, 1, jobAttr)
		}
	}
	defer func() {
		if p := recover(); p != nil {
			erro := fmt.Errorf("panic: %v", p)
			registrar(erro)
			FinalizarSpanErr(span, erro)
			panic(p)
		}
	}()
	err := fn(ctx)
	registrar(err)
	FinalizarSpanErr(span, err)
	return err
}

// Rodada é RodadaTicker para jobs cujo corpo registra o próprio erro em log.
func Rodada(ctx context.Context, job string, intervalo time.Duration, fn func(context.Context)) {
	_ = RodadaTicker(ctx, job, intervalo, func(c context.Context) error {
		fn(c)
		return nil
	})
}
