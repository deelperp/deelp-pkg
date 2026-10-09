package observabilidade

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestMetricasHTTPUsamRotaRegistradaESemDuplicata(t *testing.T) {
	prepararTracer(t)
	leitor := sdkmetric.NewManualReader()
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(leitor), sdkmetric.WithView(visoesPadrao()...)))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ordens/{ordemId}", func(w http.ResponseWriter, r *http.Request) {})
	srv := httptest.NewServer(MiddlewareHTTP("teste")(Rotas(mux)))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/ordens/3f2a9c")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	resp, err = ClienteHTTP(0, nil).Get(srv.URL + "/ordens/1")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	var rm metricdata.ResourceMetrics
	if err := leitor.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	if len(rm.ScopeMetrics) == 0 {
		t.Skip("instrumentos globais já ligados a outro MeterProvider por teste anterior; o arquivo a_ existe para rodar primeiro")
	}
	nomes := map[string]metricdata.Metrics{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			nomes[m.Name] = m
		}
	}
	for n := range nomes {
		if strings.HasPrefix(n, "http.") {
			t.Fatalf("métrica nativa do otelhttp não deveria existir: %s", n)
		}
	}
	h, ok := nomes["deelp_http_request_duration_seconds"].Data.(metricdata.Histogram[float64])
	if !ok || len(h.DataPoints) == 0 {
		t.Fatalf("histograma do servidor ausente: %+v", nomes)
	}
	rota, _ := h.DataPoints[0].Attributes.Value("rota")
	if rota.AsString() != "GET /ordens/{ordemId}" {
		t.Fatalf("rota deveria ser o padrão registrado, veio %q", rota.AsString())
	}
	if _, ok := nomes["deelp_http_client_duration_seconds"]; !ok {
		t.Fatal("histograma do cliente ausente")
	}
}
