package resposta

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deelperp/deelp-pkg/auth"
	"github.com/google/uuid"
)

func TestEscreverErro_EnvelopeCanonico(t *testing.T) {
	rec := httptest.NewRecorder()
	EscreverErro(rec, http.StatusUnauthorized, "Não autorizado")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q", ct)
	}

	var corpo Corpo
	if err := json.Unmarshal(rec.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("json: %v", err)
	}
	if corpo.Sucesso || corpo.Mensagem != "Não autorizado" || corpo.Conteudo != nil {
		t.Errorf("corpo = %+v", corpo)
	}
}

func TestEscreverSucesso_OmiteMensagem(t *testing.T) {
	rec := httptest.NewRecorder()
	EscreverSucesso(rec, map[string]int{"quantidade": 7})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var corpo Corpo
	if err := json.Unmarshal(rec.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("json: %v", err)
	}
	if !corpo.Sucesso || corpo.Mensagem != "" {
		t.Errorf("corpo = %+v", corpo)
	}
}

func TestEscreverResultado_InfereNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	EscreverResultado(rec, false, "NFS-e não encontrada", nil)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, esperado 404", rec.Code)
	}
}

func TestEscreverSaida_FalhaUsa400(t *testing.T) {
	rec := httptest.NewRecorder()
	EscreverSaida(rec, false, Corpo{Sucesso: false, Mensagem: "inválido"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestEscreverCriado_SucessoUsa201(t *testing.T) {
	rec := httptest.NewRecorder()
	EscreverCriado(rec, true, Corpo{Sucesso: true})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestEscreverResultado_SucessoComMensagem(t *testing.T) {
	rec := httptest.NewRecorder()
	EscreverResultado(rec, true, "já cancelada", map[string]string{"uid": "1"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestStatusDoErro(t *testing.T) {
	casos := map[string]int{
		"NFS-e não encontrada":            http.StatusNotFound,
		"nota nao encontrada":             http.StatusNotFound,
		"não pode ser editada":            http.StatusConflict,
		"Já existe NFS-e com este número": http.StatusConflict,
		"conflito de versão":              http.StatusConflict,
		"corpo inválido":                  http.StatusBadRequest,
	}
	for msg, esperado := range casos {
		if obtido := StatusDoErro(msg); obtido != esperado {
			t.Errorf("StatusDoErro(%q) = %d, esperado %d", msg, obtido, esperado)
		}
	}
}

func TestEmpresaIdDoToken_SemClaimResponde403(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	id, ok := EmpresaIdDoToken(rec, req)
	if ok || id != uuid.Nil {
		t.Fatal("sem claim deveria recusar")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, esperado 403", rec.Code)
	}
}

func TestEmpresaIdDoToken_LeDoContexto(t *testing.T) {
	empresaId := uuid.New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(auth.ComClaims(req.Context(), auth.Claims{EmpresaId: empresaId.String()}))

	id, ok := EmpresaIdDoToken(rec, req)
	if !ok || id != empresaId {
		t.Fatalf("id = %v ok = %v", id, ok)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("não deveria escrever resposta: %s", rec.Body.String())
	}
}

func TestUsuarioIdDoToken_SemClaimResponde403(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	if _, ok := UsuarioIdDoToken(rec, req); ok {
		t.Fatal("sem claim deveria recusar")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, esperado 403", rec.Code)
	}
}
