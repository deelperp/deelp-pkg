// Package observabilidade inicializa o pipeline OpenTelemetry (traces +
// metrics) exportando via OTLP. Compartilhado entre todos os microserviços
// da plataforma Deelp.
//
// Quem usa chama Iniciar(ctx, cfg) no startup e defer no shutdown:
//
//	desligar, err := observabilidade.Iniciar(ctx, observabilidade.Config{
//	    NomeServico:    "ordem-service",
//	    VersaoServico:  "1.4.2",
//	    Ambiente:       "producao",
//	    Endpoint:       "otel-collector.internal:4317",
//	    Protocolo:      observabilidade.ProtocoloGRPC,
//	})
//	if err != nil { log.Fatal(err) }
//	defer desligar(context.Background())
package observabilidade

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
)

// Protocolo determina como o cliente OTLP fala com o Collector.
type Protocolo string

const (
	ProtocoloAuto Protocolo = "auto"
	ProtocoloGRPC Protocolo = "grpc"
	ProtocoloHTTP Protocolo = "http"
)

// Config configura o pipeline de observabilidade.
//
// Campos obrigatórios: NomeServico, Endpoint.
//
// VersaoServico e Ambiente são enviados como resource attributes para
// permitir filtrar dashboards/alertas. Logger é opcional — usado para
// reportar o modo escolhido e erros não-fatais durante o setup.
type Config struct {
	NomeServico   string
	VersaoServico string
	Ambiente      string
	Endpoint      string
	Protocolo     Protocolo
	Logger        *slog.Logger
}

// Desligar é a função retornada por Iniciar; chame-a no shutdown do
// processo para garantir flush dos spans/métricas em buffer.
type Desligar func(context.Context) error

func (c Config) validar() error {
	if strings.TrimSpace(c.NomeServico) == "" {
		return errors.New("observabilidade: NomeServico obrigatório")
	}
	if strings.TrimSpace(c.Endpoint) == "" {
		return errors.New("observabilidade: Endpoint obrigatório")
	}
	return nil
}

func (c Config) protocoloEfetivo() Protocolo {
	switch c.Protocolo {
	case ProtocoloGRPC, ProtocoloHTTP:
		return c.Protocolo
	}
	ep := c.Endpoint
	if strings.HasPrefix(ep, "http://") || strings.HasPrefix(ep, "https://") {
		return ProtocoloHTTP
	}
	return ProtocoloGRPC
}

// hostDoEndpoint remove scheme e path do endpoint HTTP, deixando só host:porta.
// Necessário porque otlptracehttp.WithEndpoint espera só o host.
func hostDoEndpoint(ep string) string {
	host := strings.TrimPrefix(ep, "https://")
	host = strings.TrimPrefix(host, "http://")
	if idx := strings.Index(host, "/"); idx != -1 {
		host = host[:idx]
	}
	return host
}

func (c Config) log(msg string, args ...any) {
	if c.Logger == nil {
		return
	}
	c.Logger.Info(msg, args...)
}

// Iniciar configura tracer/meter providers globais e propagators, e devolve
// uma função de shutdown que flush-a tudo em buffer antes de fechar.
func Iniciar(ctx context.Context, cfg Config) (Desligar, error) {
	if err := cfg.validar(); err != nil {
		return nil, err
	}

	versao := cfg.VersaoServico
	if versao == "" {
		versao = "0.0.0"
	}
	ambiente := cfg.Ambiente
	if ambiente == "" {
		ambiente = "unknown"
	}

	// resource.New com detectors (WithProcess/WithHost/WithTelemetrySDK)
	// dispara "conflicting Schema URL" quando dependências transitivas trazem
	// semconv de versões diferentes (1.25 vs 1.27 vs 1.34 etc) — comum quando
	// otel direto e otel/sdk indireto estão em versões diferentes no go.mod.
	//
	// Solução: criar o resource só com os atributos que controlamos diretamente,
	// usando o semconv pinado em /v1.27.0. Atributos auxiliares (host.name,
	// process.pid, etc) são úteis mas não essenciais — perdê-los é aceitável
	// e evita 100% o erro de schema URL conflict.
	res := resource.NewWithAttributes(semconv.SchemaURL,
		semconv.ServiceNameKey.String(cfg.NomeServico),
		semconv.ServiceVersionKey.String(versao),
		semconv.DeploymentEnvironmentNameKey.String(ambiente),
	)

	proto := cfg.protocoloEfetivo()
	cfg.log("observabilidade.Iniciar", "servico", cfg.NomeServico, "protocolo", string(proto), "endpoint", cfg.Endpoint)

	var (
		tp *sdktrace.TracerProvider
		mt *sdkmetric.MeterProvider
	)

	switch proto {
	case ProtocoloHTTP:
		host := hostDoEndpoint(cfg.Endpoint)
		traceExporter, terr := otlptracehttp.New(ctx,
			otlptracehttp.WithEndpoint(host),
			otlptracehttp.WithURLPath("/v1/traces"),
			otlptracehttp.WithInsecure(),
		)
		if terr != nil {
			return nil, fmt.Errorf("observabilidade: criar trace exporter HTTP: %w", terr)
		}
		metricExporter, merr := otlpmetrichttp.New(ctx,
			otlpmetrichttp.WithEndpoint(host),
			otlpmetrichttp.WithURLPath("/v1/metrics"),
			otlpmetrichttp.WithInsecure(),
		)
		if merr != nil {
			_ = traceExporter.Shutdown(ctx)
			return nil, fmt.Errorf("observabilidade: criar metric exporter HTTP: %w", merr)
		}
		tp = sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(traceExporter),
			sdktrace.WithResource(res),
		)
		mt = sdkmetric.NewMeterProvider(
			sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
			sdkmetric.WithResource(res),
		)

	case ProtocoloGRPC:
		traceExporter, terr := otlptracegrpc.New(ctx,
			otlptracegrpc.WithInsecure(),
			otlptracegrpc.WithEndpoint(cfg.Endpoint),
		)
		if terr != nil {
			return nil, fmt.Errorf("observabilidade: criar trace exporter gRPC: %w", terr)
		}
		metricExporter, merr := otlpmetricgrpc.New(ctx,
			otlpmetricgrpc.WithInsecure(),
			otlpmetricgrpc.WithEndpoint(cfg.Endpoint),
		)
		if merr != nil {
			_ = traceExporter.Shutdown(ctx)
			return nil, fmt.Errorf("observabilidade: criar metric exporter gRPC: %w", merr)
		}
		tp = sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(traceExporter),
			sdktrace.WithResource(res),
		)
		mt = sdkmetric.NewMeterProvider(
			sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
			sdkmetric.WithResource(res),
		)

	default:
		return nil, fmt.Errorf("observabilidade: protocolo desconhecido %q", proto)
	}

	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mt)
	// Sem propagator W3C, requests entre serviços perdem o trace parent —
	// cada microservice cria um trace novo. Setar aqui resolve para todos
	// os consumidores deste pacote.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return func(c context.Context) error {
		errTP := tp.Shutdown(c)
		errMT := mt.Shutdown(c)
		return errors.Join(errTP, errMT)
	}, nil
}
