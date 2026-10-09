package observabilidade

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func prepararTracer(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	gravador := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(gravador)))
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return gravador
}

func TestMiddlewareHTTPNomeiaSpanPelaRotaRegistrada(t *testing.T) {
	gravador := prepararTracer(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ordens/{ordemId}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	copiaRequest := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), struct{}{}, 1)))
		})
	}
	srv := httptest.NewServer(MiddlewareHTTP("teste")(copiaRequest(Rotas(mux))))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/ordens/3f2a")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	spans := gravador.Ended()
	if len(spans) != 1 {
		t.Fatalf("esperado 1 span, veio %d", len(spans))
	}
	if spans[0].Name() != "GET /ordens/{ordemId}" {
		t.Fatalf("nome de span inesperado: %s", spans[0].Name())
	}
	if spans[0].Status().Code.String() != "Error" {
		t.Fatalf("5xx deve marcar span como erro, veio %s", spans[0].Status().Code)
	}
}

func TestMiddlewareHTTPIgnoraHealth(t *testing.T) {
	gravador := prepararTracer(t)
	srv := httptest.NewServer(MiddlewareHTTP("teste")(http.NewServeMux()))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if n := len(gravador.Ended()); n != 0 {
		t.Fatalf("health não deve gerar span, gerou %d", n)
	}
}

func TestTransportePropagaTraceparent(t *testing.T) {
	prepararTracer(t)
	var recebido string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recebido = r.Header.Get("traceparent")
	}))
	defer srv.Close()

	ctx, span := Span(t.Context(), "teste", "pai")
	defer span.End()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := ClienteHTTP(0, nil).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if recebido == "" {
		t.Fatal("traceparent não foi propagado")
	}
}

func TestAmostradorPadraoMantemTudo(t *testing.T) {
	t.Setenv(envAmostragem, "")
	if d := amostrador(0).Description(); d != sdktrace.ParentBased(sdktrace.AlwaysSample()).Description() {
		t.Fatalf("sem configuração deve manter tudo, veio %s", d)
	}
	if d := amostrador(0.1).Description(); d == sdktrace.ParentBased(sdktrace.AlwaysSample()).Description() {
		t.Fatal("fração 0.1 deveria amostrar")
	}
}

func TestMedirEtapaPropagaErroEMarcaSpan(t *testing.T) {
	gravador := prepararTracer(t)
	falha := errors.New("sefaz fora")
	err := MedirEtapa(t.Context(), "nfe_emissao", "enviar_sefaz", func(context.Context) error { return falha })
	if !errors.Is(err, falha) {
		t.Fatalf("erro deve voltar intacto, veio %v", err)
	}
	spans := gravador.Ended()
	if len(spans) != 1 || spans[0].Name() != "enviar_sefaz" || spans[0].Status().Code.String() != "Error" {
		t.Fatalf("span inesperado: %+v", spans)
	}
}
