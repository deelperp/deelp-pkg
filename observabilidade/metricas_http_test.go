package observabilidade

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func instrumentosLocais(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()
	leitor := sdkmetric.NewManualReader()
	m := sdkmetric.NewMeterProvider(sdkmetric.WithReader(leitor), sdkmetric.WithView(visoesPadrao()...)).Meter("teste")
	antigoServidor, antigoAndamento, antigoCliente := duracaoServidor, emAndamento, duracaoCliente
	t.Cleanup(func() { duracaoServidor, emAndamento, duracaoCliente = antigoServidor, antigoAndamento, antigoCliente })
	var err error
	if duracaoServidor, err = m.Float64Histogram("deelp_http_request_duration_seconds", metric.WithUnit("s")); err != nil {
		t.Fatal(err)
	}
	if emAndamento, err = m.Int64UpDownCounter("deelp_http_requests_in_flight"); err != nil {
		t.Fatal(err)
	}
	if duracaoCliente, err = m.Float64Histogram("deelp_http_client_duration_seconds", metric.WithUnit("s")); err != nil {
		t.Fatal(err)
	}
	return leitor
}

func coletar(t *testing.T, leitor *sdkmetric.ManualReader) map[string]metricdata.Metrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := leitor.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	nomes := map[string]metricdata.Metrics{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			nomes[m.Name] = m
		}
	}
	return nomes
}

func TestMetricasHTTPUsamRotaRegistradaEMetodoFechado(t *testing.T) {
	prepararTracer(t)
	leitor := instrumentosLocais(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ordens/{ordemId}", func(w http.ResponseWriter, r *http.Request) {})
	srv := httptest.NewServer(MiddlewareHTTP("teste")(Rotas(mux)))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/ordens/3f2a9c")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	req, _ := http.NewRequest("METODOINVENTADO", srv.URL+"/qualquer", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	resp, err = ClienteHTTP(0, nil).Get(srv.URL + "/ordens/1")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	nomes := coletar(t, leitor)
	h, ok := nomes["deelp_http_request_duration_seconds"].Data.(metricdata.Histogram[float64])
	if !ok || len(h.DataPoints) == 0 {
		t.Fatalf("histograma do servidor ausente: %+v", nomes)
	}
	rotas, metodos := map[string]bool{}, map[string]bool{}
	for _, dp := range h.DataPoints {
		r, _ := dp.Attributes.Value("rota")
		m, _ := dp.Attributes.Value("metodo")
		rotas[r.AsString()] = true
		metodos[m.AsString()] = true
	}
	if !rotas["GET /ordens/{ordemId}"] {
		t.Fatalf("rota deveria ser o padrão registrado: %v", rotas)
	}
	if metodos["METODOINVENTADO"] || !metodos["_OTHER"] {
		t.Fatalf("método fora da lista deve virar _OTHER: %v", metodos)
	}
	if _, ok := nomes["deelp_http_client_duration_seconds"]; !ok {
		t.Fatal("histograma do cliente ausente")
	}
}
