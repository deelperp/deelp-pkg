package internalauth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func requisicao(chave string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/servico/v1/internal/consumo", nil)
	if chave != "" {
		r.Header.Set(Header, chave)
	}
	return r
}

func TestValida_AceitaChaveCerta(t *testing.T) {
	if !NewVerificador("dedicada").Valida(requisicao("dedicada")) {
		t.Fatal("esperava aceitar a chave configurada")
	}
}

func TestValida_RejeitaChaveErrada(t *testing.T) {
	if NewVerificador("dedicada").Valida(requisicao("outra")) {
		t.Fatal("não esperava aceitar chave desconhecida")
	}
}

func TestValida_RejeitaHeaderAusente(t *testing.T) {
	if NewVerificador("dedicada").Valida(requisicao("")) {
		t.Fatal("não esperava aceitar requisição sem o header")
	}
}

// Sem chave configurada nega tudo: rota interna aberta por configuração
// faltando exporia dado de todos os tenants.
func TestValida_SemChaveConfiguradaNegaTudo(t *testing.T) {
	v := NewVerificador("")
	if v.Valida(requisicao("qualquer")) {
		t.Fatal("sem chave configurada, nada pode ser aceito")
	}
	if v.Valida(requisicao("")) {
		t.Fatal("header vazio não pode casar com chave vazia")
	}
}

func TestValida_VerificadorNilNega(t *testing.T) {
	var v *Verificador
	if v.Valida(requisicao("qualquer")) {
		t.Fatal("verificador nil deve negar")
	}
}

func TestMiddleware_BloqueiaComChaveErrada(t *testing.T) {
	alcancado := false
	destino := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { alcancado = true })

	rec := httptest.NewRecorder()
	NewVerificador("dedicada").Middleware(destino).ServeHTTP(rec, requisicao("errada"))

	if alcancado {
		t.Fatal("handler não deveria ser alcançado")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, esperado 403", rec.Code)
	}
}

func TestMiddleware_PassaComChaveCerta(t *testing.T) {
	alcancado := false
	destino := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { alcancado = true })

	rec := httptest.NewRecorder()
	NewVerificador("dedicada").Middleware(destino).ServeHTTP(rec, requisicao("dedicada"))

	if !alcancado {
		t.Fatalf("handler deveria ser alcançado; status = %d", rec.Code)
	}
}
