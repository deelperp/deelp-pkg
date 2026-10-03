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
	"crypto/tls"
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
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
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
	// TLSConfig habilita TLS e permite configurar a CA e certificados de cliente.
	// HTTPS usa TLS mesmo quando este campo é nil; host:porta mantém o modo sem TLS.
	TLSConfig *tls.Config
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
	switch c.Protocolo {
	case "", ProtocoloAuto, ProtocoloGRPC, ProtocoloHTTP:
	default:
		return fmt.Errorf("observabilidade: protocolo desconhecido %q", c.Protocolo)
	}
	return nil
}

func (c Config) protocoloEfetivo() Protocolo {
	switch c.Protocolo {
	case ProtocoloGRPC, ProtocoloHTTP:
		return c.Protocolo
	}
	ep := strings.TrimSpace(c.Endpoint)
	if strings.HasPrefix(ep, "http://") || strings.HasPrefix(ep, "https://") {
		return ProtocoloHTTP
	}
	return ProtocoloGRPC
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
	endpoint, err := cfg.resolveEndpoint()
	if err != nil {
		return nil, err
	}
	tlsConfig := &tls.Config{}
	if cfg.TLSConfig != nil {
		tlsConfig = cfg.TLSConfig.Clone()
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
		traceOptions := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(endpoint.host),
			otlptracehttp.WithURLPath(endpoint.path + "/v1/traces"),
		}
		metricOptions := []otlpmetrichttp.Option{
			otlpmetrichttp.WithEndpoint(endpoint.host),
			otlpmetrichttp.WithURLPath(endpoint.path + "/v1/metrics"),
		}
		if endpoint.secure {
			traceOptions = append(traceOptions, otlptracehttp.WithTLSClientConfig(tlsConfig), otlptracehttp.WithProxy(secureProxy))
			metricOptions = append(metricOptions, otlpmetrichttp.WithTLSClientConfig(tlsConfig), otlpmetrichttp.WithProxy(secureProxy))
		} else {
			traceOptions = append(traceOptions, otlptracehttp.WithInsecure())
			metricOptions = append(metricOptions, otlpmetrichttp.WithInsecure())
		}
		traceExporter, terr := otlptracehttp.New(ctx, traceOptions...)
		if terr != nil {
			return nil, fmt.Errorf("observabilidade: criar trace exporter HTTP: %w", terr)
		}
		metricExporter, merr := otlpmetrichttp.New(ctx, metricOptions...)
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
		var transportCredentials credentials.TransportCredentials = insecure.NewCredentials()
		if endpoint.secure {
			transportCredentials = credentials.NewTLS(tlsConfig)
		}
		traceExporter, terr := otlptracegrpc.New(ctx,
			otlptracegrpc.WithTLSCredentials(transportCredentials),
			otlptracegrpc.WithEndpoint(endpoint.host),
		)
		if terr != nil {
			return nil, fmt.Errorf("observabilidade: criar trace exporter gRPC: %w", terr)
		}
		metricExporter, merr := otlpmetricgrpc.New(ctx,
			otlpmetricgrpc.WithTLSCredentials(transportCredentials),
			otlpmetricgrpc.WithEndpoint(endpoint.host),
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
