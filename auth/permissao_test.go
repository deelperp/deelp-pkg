package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type checkerFake struct {
	permitido bool
	err       error
	chamado   bool
	modulo    string
	acao      string
}

func (c *checkerFake) TemPermissao(_ context.Context, _, _, modulo, acao string) (bool, error) {
	c.chamado = true
	c.modulo = modulo
	c.acao = acao
	return c.permitido, c.err
}

func executar(mw func(http.Handler) http.Handler, claims *Claims) *httptest.ResponseRecorder {
	final := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	if claims != nil {
		req = req.WithContext(ComClaims(req.Context(), *claims))
	}
	rec := httptest.NewRecorder()
	mw(final).ServeHTTP(rec, req)
	return rec
}

func TestRequerPermissao(t *testing.T) {
	cfg := Config{}
	claimsCompletas := &Claims{UsuarioId: "u1", EmpresaId: "e1"}

	t.Run("permite quando checker retorna true", func(t *testing.T) {
		ck := &checkerFake{permitido: true}
		rec := executar(RequerPermissao(cfg, ck, "colaboradores", "configurar"), claimsCompletas)
		if rec.Code != http.StatusOK {
			t.Fatalf("esperado 200, obtido %d", rec.Code)
		}
		if !ck.chamado || ck.modulo != "colaboradores" || ck.acao != "configurar" {
			t.Fatalf("checker chamado com args errados: %+v", ck)
		}
	})

	t.Run("nega com 403 quando checker retorna false", func(t *testing.T) {
		rec := executar(RequerPermissao(cfg, &checkerFake{permitido: false}, "colaboradores", "configurar"), claimsCompletas)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("esperado 403, obtido %d", rec.Code)
		}
	})

	t.Run("401 quando nao ha claims", func(t *testing.T) {
		rec := executar(RequerPermissao(cfg, &checkerFake{permitido: true}, "m", "a"), nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("esperado 401, obtido %d", rec.Code)
		}
	})

	t.Run("403 quando claim sem empresa", func(t *testing.T) {
		rec := executar(RequerPermissao(cfg, &checkerFake{permitido: true}, "m", "a"), &Claims{UsuarioId: "u1"})
		if rec.Code != http.StatusForbidden {
			t.Fatalf("esperado 403, obtido %d", rec.Code)
		}
	})

	t.Run("403 fail-closed quando checker erra", func(t *testing.T) {
		rec := executar(RequerPermissao(cfg, &checkerFake{err: errors.New("db down")}, "m", "a"), claimsCompletas)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("esperado 403, obtido %d", rec.Code)
		}
	})

	t.Run("403 quando checker nil", func(t *testing.T) {
		rec := executar(RequerPermissao(cfg, nil, "m", "a"), claimsCompletas)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("esperado 403, obtido %d", rec.Code)
		}
	})
}

type checkerRemotoFake struct {
	permitido   bool
	err         error
	bearerVisto string
}

func (c *checkerRemotoFake) TemPermissao(_ context.Context, bearer, _, _, _ string) (bool, error) {
	c.bearerVisto = bearer
	return c.permitido, c.err
}

func executarRemoto(mw func(http.Handler) http.Handler, claims *Claims, authHeader string) *httptest.ResponseRecorder {
	final := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if claims != nil {
		req = req.WithContext(ComClaims(req.Context(), *claims))
	}
	rec := httptest.NewRecorder()
	mw(final).ServeHTTP(rec, req)
	return rec
}

func TestRequerPermissaoRemota(t *testing.T) {
	cfg := Config{}
	claims := &Claims{UsuarioId: "u1", EmpresaId: "e1"}

	t.Run("permite e repassa o bearer", func(t *testing.T) {
		ck := &checkerRemotoFake{permitido: true}
		rec := executarRemoto(RequerPermissaoRemota(cfg, ck, "colaboradores", "configurar"), claims, "Bearer tok123")
		if rec.Code != http.StatusOK {
			t.Fatalf("esperado 200, obtido %d", rec.Code)
		}
		if ck.bearerVisto != "Bearer tok123" {
			t.Fatalf("bearer não repassado: %q", ck.bearerVisto)
		}
	})

	t.Run("nega com 403", func(t *testing.T) {
		rec := executarRemoto(RequerPermissaoRemota(cfg, &checkerRemotoFake{permitido: false}, "m", "a"), claims, "Bearer t")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("esperado 403, obtido %d", rec.Code)
		}
	})

	t.Run("403 fail-closed em erro", func(t *testing.T) {
		rec := executarRemoto(RequerPermissaoRemota(cfg, &checkerRemotoFake{err: errCheckerRemoto}, "m", "a"), claims, "Bearer t")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("esperado 403, obtido %d", rec.Code)
		}
	})

	t.Run("401 sem claims", func(t *testing.T) {
		rec := executarRemoto(RequerPermissaoRemota(cfg, &checkerRemotoFake{permitido: true}, "m", "a"), nil, "Bearer t")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("esperado 401, obtido %d", rec.Code)
		}
	})
}

var errCheckerRemoto = errors.New("remoto indisponível")
