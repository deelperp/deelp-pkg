package observabilidade

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestSanitizarURL(t *testing.T) {
	casos := map[string]string{
		"/financeiro-service/v1/public/orcamentos/Zk3j9Qw2LmN8pR4tVx7yB1cD5eFgH/approve":  "/financeiro-service/v1/public/orcamentos/:id/approve",
		"/nfe-service/v1/nfe/35261012345678000190550010000001231000001234":                "/nfe-service/v1/nfe/:id",
		"/ordem-service/v1/pedidos/3f2a9c10-1111-4222-8333-444455556666/anexos":           "/ordem-service/v1/pedidos/:id/anexos",
		"/financeiro-service/v1/interno/orcamentos-compra/pedidos-em-aberto-por-material": "/financeiro-service/v1/interno/orcamentos-compra/pedidos-em-aberto-por-material",
		"https://user:senha@api.exemplo.com/v1/cobranca/abc?token=segredo#x":              "https://api.exemplo.com/v1/cobranca/abc",
	}
	for entrada, esperado := range casos {
		if got := sanitizarURL(entrada); got != esperado {
			t.Errorf("%s\n esperado %s\n veio     %s", entrada, esperado, got)
		}
	}
}

func TestSpanDoServidorNaoGuardaTokenDoPath(t *testing.T) {
	gravador := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(processadorTenant{}), sdktrace.WithSpanProcessor(gravador)))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /public/orcamentos/{token}", func(http.ResponseWriter, *http.Request) {})
	srv := httptest.NewServer(MiddlewareHTTP("teste")(Rotas(mux)))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/public/orcamentos/Zk3j9Qw2LmN8pR4tVx7yB1cD5eFgH")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	for _, s := range gravador.Ended() {
		for _, kv := range s.Attributes() {
			if kv.Value.Type() == attribute.STRING && strings.Contains(kv.Value.AsString(), "Zk3j9Qw2") {
				t.Fatalf("token vazou no atributo %s=%s", kv.Key, kv.Value.AsString())
			}
		}
	}
}
