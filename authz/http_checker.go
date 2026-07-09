// Package authz fornece um verificador de permissões que consulta o
// autenticacao-service (fonte de verdade) via REST, repassando o token do
// usuário. Serve a qualquer microserviço que não possua a tabela de
// permissões localmente. Satisfaz auth.PermissaoCheckerRemoto.
package authz

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type HTTPChecker struct {
	baseURL string
	http    *http.Client
}

// NewHTTPChecker cria o verificador. baseURL é a URL base do
// autenticacao-service (env AUTENTICACAO_SERVICE_URL).
func NewHTTPChecker(baseURL string) *HTTPChecker {
	return &HTTPChecker{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// TemPermissao consulta as permissões do usuário (empresa vem do token
// repassado) e verifica modulo:acao. Erros de rede/parse são propagados para o
// chamador tratar como fail-closed.
func (c *HTTPChecker) TemPermissao(ctx context.Context, bearer, usuarioId, modulo, acao string) (bool, error) {
	if c.baseURL == "" {
		return false, fmt.Errorf("AUTENTICACAO_SERVICE_URL não configurada")
	}
	url := fmt.Sprintf("%s/autenticacao-service/v1/usuarios/%s/permissoes", c.baseURL, usuarioId)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	if bearer != "" {
		if strings.HasPrefix(strings.ToLower(bearer), "bearer ") {
			req.Header.Set("Authorization", bearer)
		} else {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("autenticacao-service permissoes: status %d: %s", resp.StatusCode, string(body))
	}
	var parsed struct {
		Sucesso  bool                `json:"sucesso"`
		Conteudo map[string][]string `json:"conteudo"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return false, fmt.Errorf("resposta inválida do autenticacao-service: %w", err)
	}
	if !parsed.Sucesso {
		return false, nil
	}
	for _, a := range parsed.Conteudo[modulo] {
		if a == acao {
			return true, nil
		}
	}
	return false, nil
}
