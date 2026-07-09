package auth

import (
	"context"
	"net/http"
	"strings"
)

// PermissaoChecker verifica se o usuário autenticado, no contexto da empresa
// ativa, possui a permissão (modulo:acao).
//
// A implementação é injetada por cada serviço, mantendo o pacote auth livre de
// dependências de domínio:
//   - autenticacao-service consulta o banco de permissões diretamente;
//   - demais serviços consultam via broker HTTP para o autenticacao-service.
//
// Diferente das claims do JWT, a checagem é feita ao vivo — permissões não
// trafegam no token (decisão documentada em token_claims.go), então alterações
// de cargo têm efeito imediato, sem depender de refresh de token.
type PermissaoChecker interface {
	TemPermissao(ctx context.Context, usuarioId, empresaId, modulo, acao string) (bool, error)
}

// RequerPermissao exige que o usuário autenticado possua a permissão
// (modulo:acao) antes de seguir para o próximo handler.
//
// Deve ser registrado DEPOIS de Autenticacao na cadeia — depende das claims
// injetadas no contexto. Política fail-closed:
//   - claim ausente          → 401
//   - claim sem empresa       → 403
//   - checker nil ou com erro → 403
//   - sem a permissão         → 403
func RequerPermissao(cfg Config, checker PermissaoChecker, modulo, acao string) func(http.Handler) http.Handler {
	resp := cfg.responder()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsDoContexto(r.Context())
			if !ok || claims.UsuarioId == "" {
				resp(w, http.StatusUnauthorized, "Você precisa estar logado para realizar esta ação")
				return
			}
			if claims.EmpresaId == "" {
				resp(w, http.StatusForbidden, "Token sem vínculo de empresa. Selecione uma colaboração novamente.")
				return
			}
			if checker == nil {
				cfg.log("auth.RequerPermissao: checker nil", "modulo", modulo, "acao", acao)
				resp(w, http.StatusForbidden, "Autorização indisponível")
				return
			}
			permitido, err := checker.TemPermissao(r.Context(), claims.UsuarioId, claims.EmpresaId, modulo, acao)
			if err != nil {
				cfg.log("auth.RequerPermissao: erro no checker", "erro", err, "modulo", modulo, "acao", acao)
				resp(w, http.StatusForbidden, "Não foi possível validar sua permissão")
				return
			}
			if !permitido {
				resp(w, http.StatusForbidden, "Você não tem permissão para realizar esta ação")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// PermissaoCheckerRemoto verifica a permissão consultando outro serviço
// (tipicamente o autenticacao-service), repassando o token do request. Usado
// por serviços que não possuem a tabela de permissões localmente.
type PermissaoCheckerRemoto interface {
	TemPermissao(ctx context.Context, bearer, usuarioId, modulo, acao string) (bool, error)
}

// RequerPermissaoRemota é a variante de RequerPermissao para serviços que
// autorizam via chamada remota: extrai o Bearer do request (header ou cookie
// cfg.CookieName) e o repassa ao checker. Mesma política fail-closed.
// Deve rodar após Autenticacao.
func RequerPermissaoRemota(cfg Config, checker PermissaoCheckerRemoto, modulo, acao string) func(http.Handler) http.Handler {
	resp := cfg.responder()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsDoContexto(r.Context())
			if !ok || claims.UsuarioId == "" {
				resp(w, http.StatusUnauthorized, "Você precisa estar logado para realizar esta ação")
				return
			}
			if claims.EmpresaId == "" {
				resp(w, http.StatusForbidden, "Token sem vínculo de empresa. Selecione uma colaboração novamente.")
				return
			}
			if checker == nil {
				cfg.log("auth.RequerPermissaoRemota: checker nil", "modulo", modulo, "acao", acao)
				resp(w, http.StatusForbidden, "Autorização indisponível")
				return
			}
			permitido, err := checker.TemPermissao(r.Context(), bearerDoRequest(r, cfg.CookieName), claims.UsuarioId, modulo, acao)
			if err != nil {
				cfg.log("auth.RequerPermissaoRemota: erro no checker", "erro", err, "modulo", modulo, "acao", acao)
				resp(w, http.StatusForbidden, "Não foi possível validar sua permissão")
				return
			}
			if !permitido {
				resp(w, http.StatusForbidden, "Você não tem permissão para realizar esta ação")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func bearerDoRequest(r *http.Request, cookieName string) string {
	raw := strings.TrimSpace(r.Header.Get("Authorization"))
	if raw != "" {
		return raw
	}
	if cookieName != "" {
		if c, err := r.Cookie(cookieName); err == nil {
			return strings.TrimSpace(c.Value)
		}
	}
	return ""
}
