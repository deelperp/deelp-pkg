package authz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deelperp/deelp-pkg/auth"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestPermissaoRemota_SuporteRevalidaNaProximaRequisicao(t *testing.T) {
	casos := []struct {
		nome   string
		metodo string
		acao   string
		status int
		corpo  string
	}{
		{"sessao encerrada", http.MethodGet, "visualizar", http.StatusOK, `{"sucesso":true,"conteudo":{}}`},
		{"escrita revogada", http.MethodPut, "atualizar", http.StatusOK, `{"sucesso":true,"conteudo":{"cadastros":["visualizar"]}}`},
		{"autorizacao indisponivel", http.MethodPut, "atualizar", http.StatusServiceUnavailable, `{"sucesso":false}`},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			var chamadas atomic.Int32
			servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if chamadas.Add(1) == 1 {
					_, _ = w.Write([]byte(`{"sucesso":true,"conteudo":{"cadastros":["visualizar","atualizar"]}}`))
					return
				}
				w.WriteHeader(caso.status)
				_, _ = w.Write([]byte(caso.corpo))
			}))
			defer servidor.Close()

			const secret = "segredo-apenas-para-teste-de-suporte"
			empresaId := uuid.NewString()
			token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"usuarioId":         uuid.NewString(),
				"empresaId":         empresaId,
				"suporteEmpresaId":  empresaId,
				"sessaoSuporteId":   uuid.NewString(),
				"suporteEscritaAte": time.Now().Add(time.Minute).Unix(),
				"exp":               time.Now().Add(time.Minute).Unix(),
			}).SignedString([]byte(secret))
			if err != nil {
				t.Fatal(err)
			}
			checker := NewHTTPChecker(servidor.URL)
			cfg := auth.Config{SecretKey: secret}
			execucoes := 0
			final := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				execucoes++
				w.WriteHeader(http.StatusNoContent)
			})
			handler := auth.Autenticacao(cfg)(auth.TenantGuard(cfg)(
				auth.RequerPermissaoRemota(cfg, checker, "cadastros", caso.acao)(final),
			))
			for i, status := range []int{http.StatusNoContent, http.StatusForbidden} {
				req := httptest.NewRequest(caso.metodo, "/cliente-service/v1/clientes", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				res := httptest.NewRecorder()
				handler.ServeHTTP(res, req)
				if res.Code != status {
					t.Fatalf("requisição %d: esperado %d, recebido %d: %s", i+1, status, res.Code, res.Body.String())
				}
			}
			if chamadas.Load() != 2 || execucoes != 1 {
				t.Fatalf("consultas=%d, execuções=%d; esperado 2 consultas e 1 execução", chamadas.Load(), execucoes)
			}
			if checker.cache.Len() != 0 {
				t.Fatal("permissões de suporte não devem ser gravadas no cache")
			}
		})
	}
}

func TestTemPermissao_SuporteIgnoraEntradaJaCacheada(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"sucesso":true,"conteudo":{}}`))
	}))
	defer servidor.Close()
	checker := NewHTTPChecker(servidor.URL)
	checker.guardar(chaveCache("operador", "Bearer suporte"), "operador", map[string][]string{"cadastros": {"atualizar"}})
	ctx := auth.ComClaims(context.Background(), auth.Claims{
		UsuarioId: "operador", EmpresaId: "empresa", SuporteEmpresaId: "empresa", SessaoSuporteId: "sessao",
	})
	permitido, err := checker.TemPermissao(ctx, "Bearer suporte", "operador", "cadastros", "atualizar")
	if err != nil || permitido {
		t.Fatalf("cache de suporte não pode conceder acesso: permitido=%v, erro=%v", permitido, err)
	}
}
