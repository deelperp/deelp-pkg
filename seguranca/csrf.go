package seguranca

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
)

const (
	CSRFHeaderName = "X-CSRF-Token"
	CSRFCookieName = "deelp_csrf"
)

// CSRFConfig configura o CSRFGuard.
//
// O guard usa estrategia double-submit: o frontend recebe um cookie nao-httpOnly
// `CSRFCookieName` e ecoa o mesmo valor no header `CSRFHeaderName`. O backend
// rejeita quando os dois divergem.
//
// MetodosProtegidos define quais HTTP methods exigem CSRF. Padrao: POST/PUT/PATCH/DELETE.
//
// IgnorarSe e' um predicate opcional que permite isentar requisicoes
// (uso comum: endpoints chamados apenas pelo mobile, autenticados via header
// Authorization). Quando IgnorarSe retorna true, o middleware passa sem checar.
type CSRFConfig struct {
	MetodosProtegidos []string
	IgnorarSe         func(r *http.Request) bool
	ProducaoHTTPS     bool
}

func metodosPadrao() map[string]struct{} {
	return map[string]struct{}{
		http.MethodPost:   {},
		http.MethodPut:    {},
		http.MethodPatch:  {},
		http.MethodDelete: {},
	}
}

func (cfg CSRFConfig) metodos() map[string]struct{} {
	if len(cfg.MetodosProtegidos) == 0 {
		return metodosPadrao()
	}
	m := make(map[string]struct{}, len(cfg.MetodosProtegidos))
	for _, met := range cfg.MetodosProtegidos {
		m[strings.ToUpper(met)] = struct{}{}
	}
	return m
}

// CSRFGuard cria middleware que aplica double-submit cookie.
func CSRFGuard(cfg CSRFConfig) func(http.Handler) http.Handler {
	metodos := cfg.metodos()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, exige := metodos[r.Method]; !exige {
				next.ServeHTTP(w, r)
				return
			}
			if cfg.IgnorarSe != nil && cfg.IgnorarSe(r) {
				next.ServeHTTP(w, r)
				return
			}
			cookie, err := r.Cookie(CSRFCookieName)
			if err != nil || cookie == nil || cookie.Value == "" {
				MetricaCSRFFalha(r.Context(), "", "cookie_ausente")
				escreverFalhaCSRF(w, "CSRF cookie ausente")
				return
			}
			header := strings.TrimSpace(r.Header.Get(CSRFHeaderName))
			if header == "" {
				MetricaCSRFFalha(r.Context(), "", "header_ausente")
				escreverFalhaCSRF(w, "CSRF header ausente")
				return
			}
			if header != cookie.Value {
				MetricaCSRFFalha(r.Context(), "", "token_divergente")
				escreverFalhaCSRF(w, "CSRF token divergente")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CSRFTokenHandler emite um novo CSRF token, grava como cookie nao-httpOnly e
// devolve no corpo da resposta.
func CSRFTokenHandler(cfg CSRFConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := gerarToken()
		http.SetCookie(w, &http.Cookie{
			Name:     CSRFCookieName,
			Value:    token,
			Path:     "/",
			HttpOnly: false,
			Secure:   cfg.ProducaoHTTPS,
			SameSite: http.SameSiteStrictMode,
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"csrf_token":  token,
			"header_name": CSRFHeaderName,
		})
	}
}

// IgnorarCSRFSeAuthorizationBearer e' um predicate util: nao exige CSRF
// quando a requisicao tem Authorization: Bearer (mobile / API direta).
func IgnorarCSRFSeAuthorizationBearer(r *http.Request) bool {
	authz := strings.TrimSpace(r.Header.Get("Authorization"))
	return strings.HasPrefix(strings.ToLower(authz), "bearer ")
}

func gerarToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return base64.RawURLEncoding.EncodeToString([]byte("fallback-token-deterministico"))
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func escreverFalhaCSRF(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]any{"sucesso": false, "mensagem": msg})
}
