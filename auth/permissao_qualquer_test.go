package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type checkerQualquer struct {
	permitidos map[string]bool
	erro       error
}

func (c checkerQualquer) TemPermissao(_ context.Context, _, _, modulo, acao string) (bool, error) {
	if c.erro != nil {
		return false, c.erro
	}
	return c.permitidos[modulo+":"+acao], nil
}

func executarQualquer(t *testing.T, checker PermissaoCheckerRemoto, pares ...ModuloAcao) int {
	t.Helper()
	chamou := false
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req = req.WithContext(ComClaims(req.Context(), Claims{UsuarioId: "u1", EmpresaId: "e1"}))

	rec := httptest.NewRecorder()
	RequerQualquerPermissaoRemota(Config{}, checker, pares...)(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { chamou = true }),
	).ServeHTTP(rec, req)

	if rec.Code == http.StatusOK && !chamou {
		t.Fatal("respondeu 200 sem chamar o handler")
	}
	return rec.Code
}

// Recurso lido por telas de módulos diferentes: basta uma das permissões.
func TestRequerQualquerPermissao_BastaUmaPermissao(t *testing.T) {
	checker := checkerQualquer{permitidos: map[string]bool{"fiscal:visualizar": true}}

	if got := executarQualquer(t, checker,
		ModuloAcao{Modulo: "configuracoes", Acao: "visualizar"},
		ModuloAcao{Modulo: "fiscal", Acao: "visualizar"},
	); got != http.StatusOK {
		t.Fatalf("esperava 200, veio %d", got)
	}
}

func TestRequerQualquerPermissao_NegaSemNenhuma(t *testing.T) {
	checker := checkerQualquer{permitidos: map[string]bool{}}

	if got := executarQualquer(t, checker,
		ModuloAcao{Modulo: "configuracoes", Acao: "visualizar"},
		ModuloAcao{Modulo: "fiscal", Acao: "visualizar"},
	); got != http.StatusForbidden {
		t.Fatalf("esperava 403, veio %d", got)
	}
}

// Fail-closed: erro na consulta nega, não libera.
func TestRequerQualquerPermissao_ErroNoCheckerNega(t *testing.T) {
	checker := checkerQualquer{erro: errors.New("rede fora")}

	if got := executarQualquer(t, checker, ModuloAcao{Modulo: "fiscal", Acao: "visualizar"}); got != http.StatusForbidden {
		t.Fatalf("esperava 403, veio %d", got)
	}
}

func TestRequerQualquerPermissao_ListaVaziaNega(t *testing.T) {
	if got := executarQualquer(t, checkerQualquer{permitidos: map[string]bool{"x:y": true}}); got != http.StatusForbidden {
		t.Fatalf("esperava 403, veio %d", got)
	}
}

func TestRequerQualquerPermissao_CheckerNilNega(t *testing.T) {
	if got := executarQualquer(t, nil, ModuloAcao{Modulo: "fiscal", Acao: "visualizar"}); got != http.StatusForbidden {
		t.Fatalf("esperava 403, veio %d", got)
	}
}
