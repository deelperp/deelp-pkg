package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const secret = "test-secret-key-suficientemente-grande"

func gerarToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("falha gerando token: %v", err)
	}
	return s
}

func handlerOK() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestAutenticacao_TokenAusente_Retorna401(t *testing.T) {
	h := Autenticacao(Config{SecretKey: secret})(handlerOK())
	req := httptest.NewRequest(http.MethodGet, "/qualquer", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401, obtido %d", rec.Code)
	}
}

func TestAutenticacao_SecretVazio_Retorna500(t *testing.T) {
	h := Autenticacao(Config{SecretKey: ""})(handlerOK())
	req := httptest.NewRequest(http.MethodGet, "/qualquer", nil)
	req.Header.Set("Authorization", "Bearer xxx")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("esperado 500, obtido %d", rec.Code)
	}
}

func TestAutenticacao_TokenInvalido_Retorna401(t *testing.T) {
	h := Autenticacao(Config{SecretKey: secret})(handlerOK())
	req := httptest.NewRequest(http.MethodGet, "/qualquer", nil)
	req.Header.Set("Authorization", "Bearer token.invalido.aqui")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401, obtido %d", rec.Code)
	}
}

func TestAutenticacao_TokenExpirado_Retorna401(t *testing.T) {
	tok := gerarToken(t, jwt.MapClaims{
		"usuarioId": "user-1",
		"exp":       time.Now().Add(-time.Minute).Unix(),
	})
	h := Autenticacao(Config{SecretKey: secret})(handlerOK())
	req := httptest.NewRequest(http.MethodGet, "/qualquer", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401, obtido %d", rec.Code)
	}
}

func TestAutenticacao_SemUsuarioId_Retorna401(t *testing.T) {
	tok := gerarToken(t, jwt.MapClaims{
		"email": "x@y.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
	})
	h := Autenticacao(Config{SecretKey: secret})(handlerOK())
	req := httptest.NewRequest(http.MethodGet, "/qualquer", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401, obtido %d", rec.Code)
	}
}

func TestAutenticacao_TokenValido_PropagaClaims(t *testing.T) {
	tok := gerarToken(t, jwt.MapClaims{
		"usuarioId":      "user-1",
		"empresaId":      "emp-1",
		"departamentoId": "dep-1",
		"exp":            time.Now().Add(time.Hour).Unix(),
	})

	chamado := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chamado = true
		c, ok := ClaimsDoContexto(r.Context())
		if !ok || c.UsuarioId != "user-1" || c.EmpresaId != "emp-1" || c.DepartamentoId != "dep-1" {
			t.Errorf("claims não propagadas corretamente: %+v ok=%v", c, ok)
		}
		w.WriteHeader(http.StatusOK)
	})

	h := Autenticacao(Config{SecretKey: secret})(next)
	req := httptest.NewRequest(http.MethodGet, "/qualquer", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !chamado {
		t.Fatal("next não foi chamado")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d", rec.Code)
	}
}

func TestTenantGuard_SemPathEmpresaId_DeixaPassar(t *testing.T) {
	mux := http.NewServeMux()
	chamado := false
	mux.HandleFunc("GET /sem-empresa", func(w http.ResponseWriter, _ *http.Request) {
		chamado = true
		w.WriteHeader(http.StatusOK)
	})
	cfg := Config{SecretKey: secret}
	h := Autenticacao(cfg)(TenantGuard(cfg)(mux))

	tok := gerarToken(t, jwt.MapClaims{
		"usuarioId": "user-1",
		"empresaId": "emp-1",
		"exp":       time.Now().Add(time.Hour).Unix(),
	})
	req := httptest.NewRequest(http.MethodGet, "/sem-empresa", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !chamado {
		t.Fatal("handler não foi chamado")
	}
}

func TestTenantGuard_EmpresaPathDivergente_Retorna403(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /servico/{empresaId}/coisa", func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler não deveria ter sido chamado")
		w.WriteHeader(http.StatusOK)
	})
	cfg := Config{SecretKey: secret}
	h := Autenticacao(cfg)(TenantGuard(cfg)(mux))

	tok := gerarToken(t, jwt.MapClaims{
		"usuarioId": "user-1",
		"empresaId": "emp-1",
		"exp":       time.Now().Add(time.Hour).Unix(),
	})
	req := httptest.NewRequest(http.MethodGet, "/servico/outra-empresa/coisa", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("esperado 403, obtido %d", rec.Code)
	}
}

func TestTenantGuard_EmpresaPathBate_DeixaPassar(t *testing.T) {
	mux := http.NewServeMux()
	chamado := false
	mux.HandleFunc("GET /servico/{empresaId}/coisa", func(w http.ResponseWriter, _ *http.Request) {
		chamado = true
		w.WriteHeader(http.StatusOK)
	})
	cfg := Config{SecretKey: secret}
	h := Autenticacao(cfg)(TenantGuard(cfg)(mux))

	tok := gerarToken(t, jwt.MapClaims{
		"usuarioId": "user-1",
		"empresaId": "emp-1",
		"exp":       time.Now().Add(time.Hour).Unix(),
	})
	req := httptest.NewRequest(http.MethodGet, "/servico/emp-1/coisa", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !chamado || rec.Code != http.StatusOK {
		t.Fatalf("esperado 200 e chamado=true, obtido %d chamado=%v", rec.Code, chamado)
	}
}

func TestResponder_Customizado_EhUsado(t *testing.T) {
	chamou := 0
	custom := func(w http.ResponseWriter, status int, msg string) {
		chamou++
		w.WriteHeader(status)
	}
	cfg := Config{SecretKey: secret, Responder: custom}
	h := Autenticacao(cfg)(handlerOK())
	req := httptest.NewRequest(http.MethodGet, "/qualquer", nil) // sem Authorization
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if chamou != 1 {
		t.Fatalf("responder customizado não foi chamado: %d", chamou)
	}
}
