package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func claimsUsuario() Claims {
	return Claims{
		UsuarioId:         "11111111-1111-1111-1111-111111111111",
		Email:             "u@x.com",
		EmpresaId:         "22222222-2222-2222-2222-222222222222",
		ColaboracaoId:     "33333333-3333-3333-3333-333333333333",
		CargoId:           "44444444-4444-4444-4444-444444444444",
		IsPlatformAdmin:   true,
		PlataformaModulos: []string{"contas"},
		ExpiraEm:          time.Now().Add(time.Hour).Unix(),
	}
}

func tokenAssistente(t *testing.T) string {
	t.Helper()
	tok, _, err := DerivarTokenAssistente(claimsUsuario(), secret, time.Now())
	if err != nil {
		t.Fatalf("derivar: %v", err)
	}
	return tok
}

func requisicaoCom(metodo, caminho, tok string) *http.Request {
	req := httptest.NewRequest(metodo, caminho, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	return req
}

func TestCanalAssistente_LeituraPassaComClaimsRecortadas(t *testing.T) {
	var recebidas Claims
	h := Autenticacao(Config{SecretKey: secret})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recebidas, _ = ClaimsDoContexto(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, requisicaoCom(http.MethodGet, "/ordem-service/v1/pedidos/grade", tokenAssistente(t)))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d", rec.Code)
	}
	if recebidas.Canal != CanalAssistente || recebidas.EmpresaId != claimsUsuario().EmpresaId || recebidas.CargoId != claimsUsuario().CargoId {
		t.Fatalf("claims inesperadas: %+v", recebidas)
	}
	if recebidas.IsPlatformAdmin || len(recebidas.PlataformaModulos) > 0 {
		t.Fatal("token do assistente não pode carregar acesso de plataforma")
	}
}

func TestCanalAssistente_PostDeConsultaPassa(t *testing.T) {
	h := Autenticacao(Config{SecretKey: secret})(handlerOK())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, requisicaoCom(http.MethodPost, "/cliente-service/v1/clientes/pesquisar", tokenAssistente(t)))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d", rec.Code)
	}
}

func TestCanalAssistente_EscritaRecusada(t *testing.T) {
	casos := []struct{ metodo, caminho string }{
		{http.MethodPost, "/nfe-service/v1/nfe/abc/enviar-sefaz"},
		{http.MethodPost, "/nfe-service/v1/nfe"},
		{http.MethodPut, "/ordem-service/v1/pedidos/abc"},
		{http.MethodDelete, "/cliente-service/v1/clientes/abc"},
		{http.MethodPatch, "/financeiro-service/v1/contas-pagar/abc"},
		{http.MethodPost, "/notificacao-service/v1"},
		{http.MethodPost, "/relatorio-service/v1/relatorios/vendas"},
		{http.MethodPost, "/financeiro-service/v1/orcamentos/x/gerar-pdf-solicitacao"},
		{http.MethodPost, "/nfe-service/v1/nfe/listar"},
	}
	h := Autenticacao(Config{SecretKey: secret})(handlerOK())
	for _, c := range casos {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, requisicaoCom(c.metodo, c.caminho, tokenAssistente(t)))
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s: esperado 403, obtido %d", c.metodo, c.caminho, rec.Code)
		}
	}
}

func TestCanalDesconhecido_Recusado(t *testing.T) {
	tok := gerarToken(t, jwt.MapClaims{
		"usuarioId": "u", "empresaId": "e", "canal": "integracao",
		"exp": time.Now().Add(time.Minute).Unix(),
	})
	h := Autenticacao(Config{SecretKey: secret})(handlerOK())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, requisicaoCom(http.MethodGet, "/qualquer", tok))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("esperado 403, obtido %d", rec.Code)
	}
}

func TestCanalAssistente_ComClaimDeSuporteRecusado(t *testing.T) {
	tok := gerarToken(t, jwt.MapClaims{
		"usuarioId": "u", "empresaId": "e", "canal": CanalAssistente,
		"suporteEmpresaId": "e", "sessaoSuporteId": "s",
		"exp": time.Now().Add(time.Minute).Unix(),
	})
	h := Autenticacao(Config{SecretKey: secret})(handlerOK())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, requisicaoCom(http.MethodGet, "/qualquer", tok))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("esperado 403, obtido %d", rec.Code)
	}
}

func TestValidarToken_RecusaTokenDeCanal(t *testing.T) {
	if _, err := ValidarToken(tokenAssistente(t), secret); !errors.Is(err, ErrTokenDeCanal) {
		t.Fatalf("esperado ErrTokenDeCanal, obtido %v", err)
	}
}

func TestDerivarTokenAssistente_Recusas(t *testing.T) {
	agora := time.Now()
	casos := map[string]Claims{
		"já é canal":     func() Claims { c := claimsUsuario(); c.Canal = CanalAssistente; return c }(),
		"suporte":        func() Claims { c := claimsUsuario(); c.SuporteEmpresaId = c.EmpresaId; return c }(),
		"sessao suporte": func() Claims { c := claimsUsuario(); c.SessaoSuporteId = "s"; return c }(),
		"sem empresa":    func() Claims { c := claimsUsuario(); c.EmpresaId = ""; return c }(),
		"sem usuario":    func() Claims { c := claimsUsuario(); c.UsuarioId = ""; return c }(),
		"origem vencida": func() Claims { c := claimsUsuario(); c.ExpiraEm = agora.Add(-time.Second).Unix(); return c }(),
	}
	for nome, c := range casos {
		if _, _, err := DerivarTokenAssistente(c, secret, agora); err == nil {
			t.Errorf("%s: derivação deveria ser recusada", nome)
		}
	}
	if _, _, err := DerivarTokenAssistente(claimsUsuario(), "", agora); err == nil {
		t.Error("secret vazio deveria ser recusado")
	}
}

func TestDerivarTokenAssistente_NaoUltrapassaOrigem(t *testing.T) {
	agora := time.Now()
	c := claimsUsuario()
	c.ExpiraEm = agora.Add(time.Minute).Unix()
	_, expira, err := DerivarTokenAssistente(c, secret, agora)
	if err != nil {
		t.Fatal(err)
	}
	if expira.Unix() != c.ExpiraEm {
		t.Fatalf("validade deveria ser a da origem: %v", expira)
	}
	c.ExpiraEm = agora.Add(time.Hour).Unix()
	_, expira, _ = DerivarTokenAssistente(c, secret, agora)
	if expira.After(agora.Add(DuracaoTokenAssistente)) {
		t.Fatalf("validade passou do teto: %v", expira)
	}
}

func TestRequerPermissao_CanalSoVisualizar(t *testing.T) {
	canal := Claims{UsuarioId: "u", EmpresaId: "e", Canal: CanalAssistente}
	checker := &checkerFake{permitido: true}
	if rec := executar(RequerPermissao(Config{}, checker, "financeiro", "visualizar"), &canal); rec.Code != http.StatusOK {
		t.Fatalf("visualizar deveria passar, obtido %d", rec.Code)
	}
	if rec := executar(RequerPermissao(Config{}, checker, "financeiro", "excluir"), &canal); rec.Code != http.StatusForbidden {
		t.Fatalf("excluir deveria ser recusado, obtido %d", rec.Code)
	}
}

func TestRequerPermissaoRemota_CanalSoVisualizar(t *testing.T) {
	canal := Claims{UsuarioId: "u", EmpresaId: "e", Canal: CanalAssistente}
	checker := &checkerRemotoFake{permitido: true}
	if rec := executarRemoto(RequerPermissaoRemota(Config{}, checker, "fiscal", "exportar"), &canal, "Bearer x"); rec.Code != http.StatusForbidden {
		t.Fatalf("exportar deveria ser recusado, obtido %d", rec.Code)
	}
	if rec := executarRemoto(RequerPermissaoRemota(Config{}, checker, "fiscal", "visualizar"), &canal, "Bearer x"); rec.Code != http.StatusOK {
		t.Fatalf("visualizar deveria passar, obtido %d", rec.Code)
	}
}

func TestRequerQualquerPermissaoRemota_CanalIgnoraParesDeEscrita(t *testing.T) {
	canal := Claims{UsuarioId: "u", EmpresaId: "e", Canal: CanalAssistente}
	mw := RequerQualquerPermissaoRemota(Config{}, &checkerRemotoFake{permitido: true}, ModuloAcao{Modulo: "configuracoes", Acao: "configurar"})
	if rec := executarRemoto(mw, &canal, "Bearer x"); rec.Code != http.StatusForbidden {
		t.Fatalf("par de escrita não pode liberar o canal, obtido %d", rec.Code)
	}
}

func TestEhCanalAssistente(t *testing.T) {
	ctx := ComClaims(context.Background(), Claims{Canal: CanalAssistente})
	if !EhCanalAssistente(ctx) || EhCanalAssistente(context.Background()) {
		t.Fatal("detecção do canal incorreta")
	}
}
