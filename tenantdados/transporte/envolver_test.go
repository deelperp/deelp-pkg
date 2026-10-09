package transporte

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deelperp/deelp-pkg/internalauth"
	"github.com/deelperp/deelp-pkg/tenantdados/core"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestEnvolverInstrumentaRotasInternasEPassaOResto(t *testing.T) {
	gravador := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(gravador)))

	proximoChamado := false
	proximo := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { proximoChamado = true })
	h := Envolver(core.Servico{}, "/tarefa-service/v1", internalauth.NewVerificador("chave"), proximo)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tarefa-service/v1"+CaminhoExportacao, nil))
	if proximoChamado {
		t.Fatal("rota interna de exportação não pode cair no roteador do serviço")
	}
	if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusForbidden {
		t.Fatalf("sem chave interna deveria recusar, veio %d", rec.Code)
	}
	spans := gravador.Ended()
	if len(spans) != 1 || spans[0].Name() != "GET /tarefa-service/v1"+CaminhoExportacao {
		t.Fatalf("export deveria gerar span com a rota exata: %+v", spans)
	}

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/tarefa-service/v1/tarefas", nil))
	if !proximoChamado {
		t.Fatal("rotas comuns devem seguir para o roteador do serviço")
	}
}
