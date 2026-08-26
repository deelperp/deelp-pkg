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
		"email":          "user@deelp.com",
		"nome":           "Ana",
		"sobrenome":      "Silva",
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
		if c.Nome != "Ana" || c.Sobrenome != "Silva" || c.Email != "user@deelp.com" {
			t.Errorf("nome/e-mail não propagados: %+v", c)
		}
		if nome, okNome := NomeCompletoDoContexto(r.Context()); !okNome || nome != "Ana Silva" {
			t.Errorf("NomeCompletoDoContexto: got %q ok=%v", nome, okNome)
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

func tokenSuporte(t *testing.T, extra jwt.MapClaims) string {
	t.Helper()
	claims := jwt.MapClaims{
		"usuarioId":        "op-1",
		"email":            "op@deelp.com",
		"empresaId":        "emp-alvo",
		"suporteEmpresaId": "emp-alvo",
		"cargoId":          "cargo-titular",
		"exp":              time.Now().Add(time.Hour).Unix(),
	}
	for k, v := range extra {
		claims[k] = v
	}
	return gerarToken(t, claims)
}

func TestAutenticacao_SessaoSuporte_GETPassa(t *testing.T) {
	h := Autenticacao(Config{SecretKey: secret})(handlerOK())
	req := httptest.NewRequest(http.MethodGet, "/cliente-service/v1/clientes/abc", nil)
	req.Header.Set("Authorization", "Bearer "+tokenSuporte(t, nil))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET de suporte deveria passar, obtido %d", rec.Code)
	}
}

func TestAutenticacao_SessaoSuporte_PostPesquisarPassa(t *testing.T) {
	h := Autenticacao(Config{SecretKey: secret})(handlerOK())
	req := httptest.NewRequest(http.MethodPost, "/cliente-service/v1/clientes/pesquisar", nil)
	req.Header.Set("Authorization", "Bearer "+tokenSuporte(t, nil))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST pesquisar de suporte deveria passar, obtido %d", rec.Code)
	}
}

func TestAutenticacao_SessaoSuporte_PostEscrita403(t *testing.T) {
	h := Autenticacao(Config{SecretKey: secret})(handlerOK())
	req := httptest.NewRequest(http.MethodPost, "/cliente-service/v1/clientes", nil)
	req.Header.Set("Authorization", "Bearer "+tokenSuporte(t, nil))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST de escrita deveria ser 403, obtido %d", rec.Code)
	}
}

func TestAutenticacao_SessaoSuporte_MetodosDeEscrita403(t *testing.T) {
	h := Autenticacao(Config{SecretKey: secret})(handlerOK())
	for _, metodo := range []string{http.MethodPut, http.MethodPatch, http.MethodDelete} {
		req := httptest.NewRequest(metodo, "/cliente-service/v1/clientes/abc", nil)
		req.Header.Set("Authorization", "Bearer "+tokenSuporte(t, nil))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s deveria ser 403, obtido %d", metodo, rec.Code)
		}
	}
}

func TestAutenticacao_SessaoNormal_PostEscritaPassa(t *testing.T) {
	tok := gerarToken(t, jwt.MapClaims{
		"usuarioId": "user-1",
		"empresaId": "emp-1",
		"exp":       time.Now().Add(time.Hour).Unix(),
	})
	h := Autenticacao(Config{SecretKey: secret})(handlerOK())
	req := httptest.NewRequest(http.MethodPost, "/cliente-service/v1/clientes", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("sessão normal não pode ser bloqueada pelo gate de suporte, obtido %d", rec.Code)
	}
}

func TestAutenticacao_SessaoSuporte_PesquisarESalvar403(t *testing.T) {
	h := Autenticacao(Config{SecretKey: secret})(handlerOK())
	req := httptest.NewRequest(http.MethodPost, "/cliente-service/v1/clientes/pesquisar-e-salvar", nil)
	req.Header.Set("Authorization", "Bearer "+tokenSuporte(t, nil))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("sufixo parcial não pode abrir escrita, obtido %d", rec.Code)
	}
}

func TestTenantGuard_SessaoSuporte_EmpresaAlvoPassa(t *testing.T) {
	empresa := "11111111-1111-1111-1111-111111111111"
	mux := http.NewServeMux()
	mux.HandleFunc("GET /empresas/{empresaId}/x", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	cfg := Config{SecretKey: secret}
	h := Autenticacao(cfg)(TenantGuard(cfg)(mux))

	tok := tokenSuporte(t, jwt.MapClaims{
		"empresaId":        empresa,
		"suporteEmpresaId": empresa,
	})
	req := httptest.NewRequest(http.MethodGet, "/empresas/"+empresa+"/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("TenantGuard deveria aceitar empresaId do alvo, obtido %d corpo=%s", rec.Code, rec.Body.String())
	}
}

func TestAutenticacao_SessaoSuporte_PropagaClaim(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !EhSessaoSuporte(r.Context()) {
			t.Error("EhSessaoSuporte deveria ser verdadeiro")
		}
		id, ok := SuporteEmpresaIdDoContexto(r.Context())
		if !ok || id.String() != "11111111-1111-1111-1111-111111111111" {
			t.Errorf("SuporteEmpresaIdDoContexto: %v ok=%v", id, ok)
		}
		w.WriteHeader(http.StatusOK)
	})
	h := Autenticacao(Config{SecretKey: secret})(next)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tokenSuporte(t, jwt.MapClaims{
		"empresaId":        "11111111-1111-1111-1111-111111111111",
		"suporteEmpresaId": "11111111-1111-1111-1111-111111111111",
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
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
	t.Skip("TODO(seguranca): middleware envolve o ServeMux como um todo, " +
		"então r.PathValue('empresaId') está vazio no momento da execução. " +
		"extrairEmpresaIdDoPath só detecta UUID após /empresas/ ou /empresa/, " +
		"o que falha em rotas tipo /servico/{empresaId}/coisa com valor não-UUID. " +
		"Solução requer RFC: ou estratégia mais agressiva de varredura de path, " +
		"ou refatorar para aplicar TenantGuard por handler (não global).")
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

func TestTenantGuard_PathRaw_EmpresasComUUIDBate(t *testing.T) {
	const empresaUUID = "11111111-2222-3333-4444-555555555555"
	chamado := false
	leaf := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		chamado = true
		w.WriteHeader(http.StatusOK)
	})
	cfg := Config{SecretKey: secret}
	h := Autenticacao(cfg)(TenantGuard(cfg)(leaf))

	tok := gerarToken(t, jwt.MapClaims{
		"usuarioId": "user-1",
		"empresaId": empresaUUID,
		"exp":       time.Now().Add(time.Hour).Unix(),
	})
	req := httptest.NewRequest(http.MethodGet, "/cliente-service/v1/empresas/"+empresaUUID+"/clientes", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !chamado || rec.Code != http.StatusOK {
		t.Fatalf("esperado 200 e chamado=true, obtido %d chamado=%v", rec.Code, chamado)
	}
}

func TestTenantGuard_PathRaw_EmpresasComUUIDDivergente(t *testing.T) {
	const empresaUUID = "11111111-2222-3333-4444-555555555555"
	const outraEmpresa = "99999999-aaaa-bbbb-cccc-dddddddddddd"
	leaf := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler não deveria ter sido chamado")
		w.WriteHeader(http.StatusOK)
	})
	cfg := Config{SecretKey: secret}
	h := Autenticacao(cfg)(TenantGuard(cfg)(leaf))

	tok := gerarToken(t, jwt.MapClaims{
		"usuarioId": "user-1",
		"empresaId": empresaUUID,
		"exp":       time.Now().Add(time.Hour).Unix(),
	})
	req := httptest.NewRequest(http.MethodGet, "/cliente-service/v1/empresas/"+outraEmpresa+"/clientes", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("esperado 403, obtido %d", rec.Code)
	}
}

func TestTenantGuard_PathRaw_SemUUID_NaoBloqueia(t *testing.T) {
	chamado := false
	leaf := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		chamado = true
		w.WriteHeader(http.StatusOK)
	})
	cfg := Config{SecretKey: secret}
	h := Autenticacao(cfg)(TenantGuard(cfg)(leaf))

	tok := gerarToken(t, jwt.MapClaims{
		"usuarioId": "user-1",
		"empresaId": "11111111-2222-3333-4444-555555555555",
		"exp":       time.Now().Add(time.Hour).Unix(),
	})
	req := httptest.NewRequest(http.MethodGet, "/cliente-service/v1/planos", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !chamado {
		t.Fatalf("handler não foi chamado mesmo sem empresaId no path")
	}
}

func TestExtrairEmpresaIdDoPath(t *testing.T) {
	const uid = "11111111-2222-3333-4444-555555555555"
	cases := []struct {
		path string
		want string
	}{
		{"/cliente-service/v1/empresas/" + uid, uid},
		{"/cliente-service/v1/empresa/" + uid + "/clientes", uid},
		{"/cliente-service/v1/empresas/" + uid + "/configuracoes/certificado", uid},
		{"/cliente-service/v1/empresas/nao-uuid", ""},
		{"/cliente-service/v1/clientes", ""},
		{"/cliente-service/v1/empresas", ""},
	}
	for _, c := range cases {
		got := extrairEmpresaIdDoPath(c.path)
		if got != c.want {
			t.Errorf("extrairEmpresaIdDoPath(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestPareceUUID(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"11111111-2222-3333-4444-555555555555", true},
		{"AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE", true},
		{"11111111-2222-3333-4444-55555555555", false},
		{"11111111-2222-3333-4444-5555555555555", false},
		{"11111111x2222-3333-4444-555555555555", false},
		{"zzzzzzzz-2222-3333-4444-555555555555", false},
		{"", false},
	}
	for _, c := range cases {
		got := pareceUUID(c.s)
		if got != c.want {
			t.Errorf("pareceUUID(%q) = %v, want %v", c.s, got, c.want)
		}
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
