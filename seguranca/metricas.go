package seguranca

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const meterNome = "deelp.seguranca"

var (
	metricasOnce          sync.Once
	contadorEventos       metric.Int64Counter
	contadorLoginTentativa metric.Int64Counter
	contadorIPBlocks      metric.Int64Counter
	contadorTenantGuard   metric.Int64Counter
	contadorCSRFFalha     metric.Int64Counter
	contadorRefreshFalha  metric.Int64Counter
)

func inicializarMetricas() {
	metricasOnce.Do(func() {
		meter := otel.GetMeterProvider().Meter(meterNome)
		contadorEventos, _ = meter.Int64Counter("deelp_security_events_total",
			metric.WithDescription("Eventos de seguranca emitidos pelo pacote pkg/seguranca"))
		contadorLoginTentativa, _ = meter.Int64Counter("deelp_login_attempts_total",
			metric.WithDescription("Tentativas de login agregadas por resultado"))
		contadorIPBlocks, _ = meter.Int64Counter("deelp_ip_blocks_total",
			metric.WithDescription("Bloqueios efetivos do IPBlocker"))
		contadorTenantGuard, _ = meter.Int64Counter("deelp_tenant_guard_blocks_total",
			metric.WithDescription("Requisicoes bloqueadas pelo TenantGuard"))
		contadorCSRFFalha, _ = meter.Int64Counter("deelp_csrf_failures_total",
			metric.WithDescription("Falhas de validacao CSRF"))
		contadorRefreshFalha, _ = meter.Int64Counter("deelp_refresh_token_failures_total",
			metric.WithDescription("Falhas na troca de refresh token"))
	})
}

func incrementar(ctx context.Context, c metric.Int64Counter, attrs ...attribute.KeyValue) {
	if c == nil {
		return
	}
	c.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// MetricaEvento incrementa o counter de eventos de seguranca.
func MetricaEvento(ctx context.Context, evento, servico string) {
	inicializarMetricas()
	incrementar(ctx, contadorEventos,
		attribute.String("evento", evento),
		attribute.String("servico", servico),
	)
}

// MetricaLoginTentativa contabiliza tentativas de login.
func MetricaLoginTentativa(ctx context.Context, resultado string) {
	inicializarMetricas()
	incrementar(ctx, contadorLoginTentativa, attribute.String("resultado", resultado))
}

// MetricaIPBloqueado conta bloqueios efetivos.
func MetricaIPBloqueado(ctx context.Context, servico, motivo string) {
	inicializarMetricas()
	incrementar(ctx, contadorIPBlocks,
		attribute.String("servico", servico),
		attribute.String("motivo", motivo),
	)
}

// MetricaTenantGuardBloqueio conta requisicoes barradas pelo TenantGuard.
func MetricaTenantGuardBloqueio(ctx context.Context, servico, motivo string) {
	inicializarMetricas()
	incrementar(ctx, contadorTenantGuard,
		attribute.String("servico", servico),
		attribute.String("motivo", motivo),
	)
}

// MetricaCSRFFalha conta falhas de validacao CSRF.
func MetricaCSRFFalha(ctx context.Context, servico, motivo string) {
	inicializarMetricas()
	incrementar(ctx, contadorCSRFFalha,
		attribute.String("servico", servico),
		attribute.String("motivo", motivo),
	)
}

// MetricaRefreshTokenFalha conta falhas na troca de refresh.
func MetricaRefreshTokenFalha(ctx context.Context, motivo string) {
	inicializarMetricas()
	incrementar(ctx, contadorRefreshFalha, attribute.String("motivo", motivo))
}
