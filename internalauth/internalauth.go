// Package internalauth autentica chamadas service-to-service pelo header
// X-Internal-Key.
//
// A chave é dedicada (INTERNAL_API_KEY) e nunca o SECRET_KEY: o segredo que
// assina os JWT não pode trafegar em header nem ser exposto fora do processo —
// quem o captura forja token de qualquer usuário da plataforma.
package internalauth

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

const Header = "X-Internal-Key"

type Verificador struct {
	chave string
}

func NewVerificador(chave string) *Verificador {
	return &Verificador{chave: strings.TrimSpace(chave)}
}

// Valida é fail-closed: verificador nil, chave não configurada ou header
// ausente negam. Rota interna aberta por engano expõe dado de todos os tenants.
func (v *Verificador) Valida(r *http.Request) bool {
	if v == nil || r == nil || v.chave == "" {
		return false
	}
	enviada := r.Header.Get(Header)
	if enviada == "" {
		return false
	}
	// Tempo constante: `!=` vaza pelo tempo de resposta quantos bytes iniciais
	// bateram, o bastante para descobrir a chave byte a byte.
	return subtle.ConstantTimeCompare([]byte(enviada), []byte(v.chave)) == 1
}

// Middleware responde 403 quando a chave não confere. Serviços que precisam de
// corpo de erro próprio usam Valida direto.
func (v *Verificador) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !v.Valida(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"sucesso":false,"mensagem":"Acesso interno não autorizado"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
