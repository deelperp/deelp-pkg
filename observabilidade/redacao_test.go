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

func TestRedigirTexto(t *testing.T) {
	casos := map[string]string{
		`duplicate key value violates unique constraint "uq_cnpj" Key (cnpj)=(12.345.678/0001-90) already exists`: `duplicate key value violates unique constraint "uq_cnpj" Key (cnpj)=([numero]) already exists`,
		`falha ao enviar para maria.silva@empresa.com.br`:                                                         `falha ao enviar para [email]`,
		`chave 35261012345678000190550010000001231000001234 rejeitada`:                                            `chave [numero] rejeitada`,
		`Authorization: Bearer abc.def.ghi falhou`:                                                                `Authorization: [redigido] falhou`,
		`token Zk3j9Qw2LmN8pR4tVx7yB1cD5eFgH inválido`:                                                            `token [token] inválido`,
		`conexão recusada: dial tcp 10.0.0.5:5432`:                                                                `conexão recusada: dial tcp 10.0.0.5:5432`,
	}
	for entrada, esperado := range casos {
		if got := redigirTexto(entrada); got != esperado {
			t.Errorf("\n entrada  %s\n esperado %s\n veio     %s", entrada, esperado, got)
		}
	}
}

func TestTransporteExternoNaoInjetaTraceparent(t *testing.T) {
	prepararTracer(t)
	var recebido string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { recebido = r.Header.Get("traceparent") }))
	defer srv.Close()
	ctx, span := Span(t.Context(), "teste", "pai")
	defer span.End()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := (&http.Client{Transport: TransporteExterno(nil)}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if recebido != "" {
		t.Fatalf("traceparent não pode sair para terceiro: %q", recebido)
	}
}
