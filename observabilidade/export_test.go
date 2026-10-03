package observabilidade

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	collectormetric "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/proto"
)

func TestIniciar_ExportaHTTP(t *testing.T) {
	for _, tc := range []struct {
		name       string
		tls        bool
		withoutURL bool
	}{
		{name: "http_com_prefixo"},
		{name: "https_com_prefixo", tls: true},
		{name: "tls_config_sem_scheme", tls: true, withoutURL: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearOTLPEnvironment(t)
			traces := make(chan *collectortrace.ExportTraceServiceRequest, 4)
			metrics := make(chan *collectormetric.ExportMetricsServiceRequest, 4)
			prefix := "/collector"
			if tc.withoutURL {
				prefix = ""
			}
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/x-protobuf" {
					t.Errorf("requisição OTLP inválida: %s %s", r.Method, r.Header.Get("Content-Type"))
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if (r.TLS != nil) != tc.tls {
					t.Errorf("TLS = %v, esperado %v", r.TLS != nil, tc.tls)
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("ler payload: %v", err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				switch r.URL.Path {
				case prefix + "/v1/traces":
					request := new(collectortrace.ExportTraceServiceRequest)
					if err := proto.Unmarshal(body, request); err != nil {
						t.Errorf("decodificar traces: %v", err)
					}
					traces <- request
				case prefix + "/v1/metrics":
					request := new(collectormetric.ExportMetricsServiceRequest)
					if err := proto.Unmarshal(body, request); err != nil {
						t.Errorf("decodificar métricas: %v", err)
					}
					metrics <- request
				default:
					t.Errorf("path OTLP inesperado: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "application/x-protobuf")
			}))
			if tc.tls {
				server.StartTLS()
			} else {
				server.Start()
			}
			t.Cleanup(server.Close)
			cfg := Config{NomeServico: "otlp-test", Endpoint: server.URL + prefix, Protocolo: ProtocoloHTTP}
			if tc.tls {
				cfg.TLSConfig = trustServer(server)
			}
			if tc.withoutURL {
				cfg.Endpoint = server.Listener.Addr().String()
			}
			startOTLP(t, cfg)
			traceErr, metricErr := flushOTLP(t)
			if traceErr != nil || metricErr != nil {
				t.Fatalf("exportação: traces=%v, métricas=%v", traceErr, metricErr)
			}
			assertOTLPPayload(t, traces, metrics)
		})
	}
}

func TestIniciar_ExportaGRPC(t *testing.T) {
	for _, tc := range []struct {
		name   string
		tls    bool
		scheme string
	}{
		{name: "host_legado"},
		{name: "http_explicito", scheme: "http://"},
		{name: "tls_config_sem_scheme", tls: true},
		{name: "https_explicito", tls: true, scheme: "https://"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearOTLPEnvironment(t)
			traces := make(chan *collectortrace.ExportTraceServiceRequest, 4)
			metrics := make(chan *collectormetric.ExportMetricsServiceRequest, 4)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = listener.Close() })
			cfg := Config{NomeServico: "otlp-test", Endpoint: tc.scheme + listener.Addr().String()}
			if tc.scheme != "" {
				cfg.Protocolo = ProtocoloGRPC
			}
			var opts []grpc.ServerOption
			if tc.tls {
				t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "true")
				certificateServer := httptest.NewTLSServer(http.NotFoundHandler())
				cfg.TLSConfig = trustServer(certificateServer)
				opts = append(opts, grpc.Creds(credentials.NewTLS(certificateServer.TLS.Clone())))
				certificateServer.Close()
			}
			server := grpc.NewServer(opts...)
			collectortrace.RegisterTraceServiceServer(server, &traceCollector{requests: traces})
			collectormetric.RegisterMetricsServiceServer(server, &metricCollector{requests: metrics})
			go func() { _ = server.Serve(listener) }()
			t.Cleanup(server.Stop)
			startOTLP(t, cfg)
			traceErr, metricErr := flushOTLP(t)
			if traceErr != nil || metricErr != nil {
				t.Fatalf("exportação: traces=%v, métricas=%v", traceErr, metricErr)
			}
			assertOTLPPayload(t, traces, metrics)
		})
	}
}

func TestIniciar_HTTPSRejeitaCertificadoNaoConfiavel(t *testing.T) {
	clearOTLPEnvironment(t)
	var requests atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
	}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	t.Cleanup(server.Close)
	startOTLP(t, Config{NomeServico: "otlp-test", Endpoint: server.URL})
	traceErr, metricErr := flushOTLP(t)
	if traceErr == nil || metricErr == nil {
		t.Errorf("certificado não confiável deve falhar: traces=%v, métricas=%v", traceErr, metricErr)
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("collector recebeu %d requisições com certificado não confiável", got)
	}
}

func TestIniciar_GRPCRejeitaCertificadoNaoConfiavel(t *testing.T) {
	clearOTLPEnvironment(t)
	traces := make(chan *collectortrace.ExportTraceServiceRequest, 4)
	metrics := make(chan *collectormetric.ExportMetricsServiceRequest, 4)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	certificateServer := httptest.NewTLSServer(http.NotFoundHandler())
	serverTLS := certificateServer.TLS.Clone()
	certificateServer.Close()
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTLS)))
	collectortrace.RegisterTraceServiceServer(server, &traceCollector{requests: traces})
	collectormetric.RegisterMetricsServiceServer(server, &metricCollector{requests: metrics})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	startOTLP(t, Config{NomeServico: "otlp-test", Endpoint: "https://" + listener.Addr().String(), Protocolo: ProtocoloGRPC})
	traceErr, metricErr := flushOTLP(t)
	if traceErr == nil || metricErr == nil {
		t.Errorf("certificado não confiável deve falhar: traces=%v, métricas=%v", traceErr, metricErr)
	}
	if len(traces) != 0 || len(metrics) != 0 {
		t.Errorf("collector gRPC recebeu %d traces e %d métricas com certificado não confiável", len(traces), len(metrics))
	}
}

func TestIniciar_HTTPSNaoPermiteDowngradePorAmbiente(t *testing.T) {
	for _, env := range []string{
		"OTEL_EXPORTER_OTLP_INSECURE",
		"OTEL_EXPORTER_OTLP_TRACES_INSECURE",
		"OTEL_EXPORTER_OTLP_METRICS_INSECURE",
	} {
		t.Run(env, func(t *testing.T) {
			clearOTLPEnvironment(t)
			t.Setenv(env, "true")
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "application/x-protobuf")
			}))
			t.Cleanup(server.Close)
			startOTLP(t, Config{NomeServico: "otlp-test", Endpoint: strings.Replace(server.URL, "http://", "https://", 1)})
			traceErr, metricErr := flushOTLP(t)
			if traceErr == nil || metricErr == nil {
				t.Errorf("HTTPS contra collector plaintext deve falhar: traces=%v, métricas=%v", traceErr, metricErr)
			}
			if got := requests.Load(); got != 0 {
				t.Errorf("%s permitiu %d requisições plaintext", env, got)
			}
		})
	}
}

func TestIniciar_HTTPSNaoSegueRedirecionamentoPlaintext(t *testing.T) {
	clearOTLPEnvironment(t)
	var plaintextRequests atomic.Int32
	plaintext := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		plaintextRequests.Add(1)
		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	t.Cleanup(plaintext.Close)
	var secureRequests atomic.Int32
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secureRequests.Add(1)
		http.Redirect(w, r, plaintext.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(secure.Close)
	startOTLP(t, Config{NomeServico: "otlp-test", Endpoint: secure.URL, TLSConfig: trustServer(secure)})
	traceErr, metricErr := flushOTLP(t)
	if traceErr == nil || metricErr == nil {
		t.Errorf("redirect para plaintext deve falhar: traces=%v, métricas=%v", traceErr, metricErr)
	}
	if secureRequests.Load() < 2 {
		t.Error("collector HTTPS não recebeu traces e métricas para redirecionar")
	}
	if got := plaintextRequests.Load(); got != 0 {
		t.Errorf("redirect permitiu %d requisições plaintext", got)
	}
}

func clearOTLPEnvironment(t *testing.T) {
	t.Helper()
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "OTEL_") {
			t.Setenv(key, "")
		}
	}
	t.Setenv("OTEL_METRIC_EXPORT_INTERVAL", "3600000")
	t.Setenv("OTEL_BSP_SCHEDULE_DELAY", "3600000")
}

func trustServer(server *httptest.Server) *tls.Config {
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	return &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
}

func startOTLP(t *testing.T, cfg Config) {
	t.Helper()
	oldTrace, oldMetric, oldPropagator := otel.GetTracerProvider(), otel.GetMeterProvider(), otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(oldTrace)
		otel.SetMeterProvider(oldMetric)
		otel.SetTextMapPropagator(oldPropagator)
	})
	shutdown, err := Iniciar(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = shutdown(ctx)
	})
	_, span := otel.Tracer("otlp-test").Start(context.Background(), "export-test")
	span.End()
	counter, err := otel.Meter("otlp-test").Int64Counter("export_test_total")
	if err != nil {
		t.Fatal(err)
	}
	counter.Add(context.Background(), 3)
}

func flushOTLP(t *testing.T) (error, error) {
	t.Helper()
	traceContext, cancelTrace := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelTrace()
	traceErr := otel.GetTracerProvider().(*sdktrace.TracerProvider).ForceFlush(traceContext)
	metricContext, cancelMetric := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelMetric()
	metricErr := otel.GetMeterProvider().(*sdkmetric.MeterProvider).ForceFlush(metricContext)
	return traceErr, metricErr
}

func assertOTLPPayload(t *testing.T, traces <-chan *collectortrace.ExportTraceServiceRequest, metrics <-chan *collectormetric.ExportMetricsServiceRequest) {
	t.Helper()
	select {
	case request := <-traces:
		found := false
		for _, resource := range request.ResourceSpans {
			for _, scope := range resource.ScopeSpans {
				for _, span := range scope.Spans {
					found = found || span.Name == "export-test"
				}
			}
		}
		if !found {
			t.Error("payload OTLP sem span export-test")
		}
	default:
		t.Error("collector não recebeu traces")
	}
	select {
	case request := <-metrics:
		found := false
		for _, resource := range request.ResourceMetrics {
			for _, scope := range resource.ScopeMetrics {
				for _, metric := range scope.Metrics {
					for _, point := range metric.GetSum().GetDataPoints() {
						found = found || (metric.Name == "export_test_total" && point.GetAsInt() == 3)
					}
				}
			}
		}
		if !found {
			t.Error("payload OTLP sem contador export_test_total=3")
		}
	default:
		t.Error("collector não recebeu métricas")
	}
}

type traceCollector struct {
	collectortrace.UnimplementedTraceServiceServer
	requests chan<- *collectortrace.ExportTraceServiceRequest
}

func (c *traceCollector) Export(_ context.Context, request *collectortrace.ExportTraceServiceRequest) (*collectortrace.ExportTraceServiceResponse, error) {
	c.requests <- request
	return &collectortrace.ExportTraceServiceResponse{}, nil
}

type metricCollector struct {
	collectormetric.UnimplementedMetricsServiceServer
	requests chan<- *collectormetric.ExportMetricsServiceRequest
}

func (c *metricCollector) Export(_ context.Context, request *collectormetric.ExportMetricsServiceRequest) (*collectormetric.ExportMetricsServiceResponse, error) {
	c.requests <- request
	return &collectormetric.ExportMetricsServiceResponse{}, nil
}
