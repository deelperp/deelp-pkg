package observabilidade

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestSpanDeClienteNaoGuardaTokenNemQuery(t *testing.T) {
	memoria := tracetest.NewInMemoryExporter()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSyncer(RedigirExportador(memoria))))
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	for _, tr := range []http.RoundTripper{Transporte(nil), TransporteExterno(nil)} {
		resp, err := (&http.Client{Transport: tr}).Get(srv.URL + "/public/orcamentos/Zk3j9Qw2LmN8pR4tVx7yB1cD5eFgH/x?token=SEGREDO")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	spans := memoria.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("esperado 2 spans de cliente, veio %d", len(spans))
	}
	for _, s := range spans {
		for _, kv := range s.Attributes {
			v := kv.Value.Emit()
			if strings.Contains(v, "Zk3j9Qw2") || strings.Contains(v, "SEGREDO") {
				t.Fatalf("vazou em %s=%s", kv.Key, v)
			}
		}
	}
}

func TestExportadorRedigeMensagemDeExcecao(t *testing.T) {
	memoria := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(RedigirExportador(memoria)))
	_, span := tp.Tracer("t").Start(context.Background(), "op")
	span.RecordError(errString("falha para maria@empresa.com.br cpf 123.456.789-09"))
	span.End()
	for _, ev := range memoria.GetSpans()[0].Events {
		for _, kv := range ev.Attributes {
			if kv.Key == "exception.message" && (strings.Contains(kv.Value.AsString(), "maria@") || strings.Contains(kv.Value.AsString(), "789-09")) {
				t.Fatalf("exception.message sem redação: %s", kv.Value.AsString())
			}
		}
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestRotasAninhadasMaisInternaVence(t *testing.T) {
	gravador := prepararTracer(t)
	interno := http.NewServeMux()
	interno.HandleFunc("GET /v1/ordens/{id}", func(http.ResponseWriter, *http.Request) {})
	externo := http.NewServeMux()
	externo.Handle("/svc/", http.StripPrefix("/svc", Rotas(interno)))
	srv := httptest.NewServer(MiddlewareHTTP("teste")(Rotas(externo)))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/svc/v1/ordens/1")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got := gravador.Ended()[0].Name(); got != "GET /v1/ordens/{id}" {
		t.Fatalf("a rota mais interna deveria vencer, veio %q", got)
	}
}

func TestPanicNoHandlerEncerraSpanComErro(t *testing.T) {
	gravador := prepararTracer(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /boom", func(http.ResponseWriter, *http.Request) { panic("boom") })
	srv := httptest.NewUnstartedServer(MiddlewareHTTP("teste")(Rotas(mux)))
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	srv.Start()
	defer srv.Close()
	if resp, err := http.Get(srv.URL + "/boom"); err == nil {
		resp.Body.Close()
	}
	spans := gravador.Ended()
	if len(spans) == 0 {
		t.Fatal("span deveria ser encerrado mesmo com panic")
	}
	if spans[0].Name() != "GET /boom" || spans[0].Status().Code.String() != "Error" {
		t.Fatalf("span de panic incorreto: %s %s", spans[0].Name(), spans[0].Status().Code)
	}
}

func TestMedirEtapaComPanicEncerraSpan(t *testing.T) {
	gravador := prepararTracer(t)
	func() {
		defer func() { _ = recover() }()
		_ = MedirEtapa(t.Context(), "f", "e", func(context.Context) error { panic("x") })
	}()
	if n := len(gravador.Ended()); n != 1 {
		t.Fatalf("span deveria ser encerrado, veio %d", n)
	}
}

func TestRedacaoCasosDaRevisao(t *testing.T) {
	manter := []string{
		"dial tcp 192.168.100.200:5432: connect: refused",
		"em 2026-10-09 12:30:45 falhou",
		"violates constraint idx_orcamentos_compra_decisoes_v2_situacao",
	}
	for _, m := range manter {
		if got := redigirTexto(m); got != m {
			t.Errorf("não deveria redigir:\n %s\n %s", m, got)
		}
	}
	remover := map[string]string{
		"telefone (11) 98765-4321 inválido":              "[numero]",
		"1198765432":                                     "[numero]",
		"postgres://app:s3cr3t@db:5432/x":                "[credenciais]",
		"senha em amqp://user:p4ss@broker:5672 recusada": "[credenciais]",
		"cpf 123.456.789-09 duplicado":                   "[numero]",
	}
	for entrada, marca := range remover {
		if got := redigirTexto(entrada); !strings.Contains(got, marca) || strings.Contains(got, "s3cr3t") || strings.Contains(got, "p4ss") || strings.Contains(got, "98765") {
			t.Errorf("deveria redigir %q → %q", entrada, got)
		}
	}
	for entrada, esperado := range map[string]string{
		"/relatorios/relatorio-mensal-de-vendas-2026-janeiro": "/relatorios/relatorio-mensal-de-vendas-2026-janeiro",
		"/pessoas/123.456.789-09":                             ":id",
		"/pessoas/1198765432":                                 ":id",
		"/x/eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.abcDEF123":   ":id",
	} {
		got := sanitizarPath(entrada)
		if esperado == ":id" {
			if !strings.Contains(got, ":id") {
				t.Errorf("path %q deveria ser redigido: %q", entrada, got)
			}
		} else if got != esperado {
			t.Errorf("path %q não deveria mudar: %q", entrada, got)
		}
	}
	_ = attribute.Key("")
}
