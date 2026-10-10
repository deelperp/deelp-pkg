package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const CanalAssistente = "ia"

const DuracaoTokenAssistente = 5 * time.Minute

const AcaoDeLeitura = "visualizar"

var ErrDerivacaoRecusada = errors.New("auth: token não pode originar acesso do assistente")

func RecusaDoCanal(c Claims, r *http.Request) string {
	if c.Canal == "" {
		return ""
	}
	if c.Canal != CanalAssistente {
		return "Canal de acesso desconhecido"
	}
	if c.SuporteEmpresaId != "" || c.SessaoSuporteId != "" {
		return "Assistente de IA indisponível em sessão de suporte"
	}
	if !canalPodeChamar(r) {
		return "O assistente de IA tem acesso somente leitura"
	}
	return ""
}

func canalPodeChamar(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	case http.MethodPost:
		return strings.HasSuffix(strings.TrimRight(r.URL.Path, "/"), "/pesquisar")
	}
	return false
}

func CanalPermiteAcao(c Claims, acao string) bool {
	if c.Canal == "" {
		return true
	}
	return c.Canal == CanalAssistente && acao == AcaoDeLeitura
}

func EhCanalAssistente(ctx context.Context) bool {
	c, ok := ClaimsDoContexto(ctx)
	return ok && c.Canal == CanalAssistente
}

func DerivarTokenAssistente(c Claims, secret string, agora time.Time) (string, time.Time, error) {
	if strings.TrimSpace(secret) == "" {
		return "", time.Time{}, ErrSecretAusente
	}
	if c.Canal != "" || c.SuporteEmpresaId != "" || c.SessaoSuporteId != "" {
		return "", time.Time{}, ErrDerivacaoRecusada
	}
	if c.UsuarioId == "" || c.EmpresaId == "" {
		return "", time.Time{}, ErrDerivacaoRecusada
	}
	expira := agora.Add(DuracaoTokenAssistente)
	if c.ExpiraEm > 0 {
		if origem := time.Unix(c.ExpiraEm, 0); origem.Before(expira) {
			expira = origem
		}
	}
	if !expira.After(agora) {
		return "", time.Time{}, ErrDerivacaoRecusada
	}
	claims := jwt.MapClaims{
		"usuarioId": c.UsuarioId,
		"email":     c.Email,
		"nome":      c.Nome,
		"sobrenome": c.Sobrenome,
		"empresaId": c.EmpresaId,
		"canal":     CanalAssistente,
		"iat":       agora.Unix(),
		"exp":       expira.Unix(),
		"jti":       uuid.NewString(),
	}
	for chave, valor := range map[string]string{
		"colaboracaoId":  c.ColaboracaoId,
		"departamentoId": c.DepartamentoId,
		"cargoId":        c.CargoId,
	} {
		if valor != "" {
			claims[chave] = valor
		}
	}
	assinado, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		return "", time.Time{}, err
	}
	return assinado, expira, nil
}
