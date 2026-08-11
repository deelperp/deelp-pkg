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
	"sync"
	"time"
)

// TTLCache é por quanto tempo o mapa de permissões do usuário fica válido em
// memória.
//
// Sem cache, CADA requisição protegida de CADA serviço fazia uma ida e volta
// HTTP ao autenticacao-service. Carregar uma tela com várias chamadas
// protegidas gerava dezenas de consultas, estourava o rate limiter (100
// req/min por IP, compartilhado com /entrar) e derrubava o próprio login.
//
// 2 minutos é curto o bastante para uma revogação de permissão surtir efeito
// rápido e longo o bastante para colapsar a rajada de uma navegação inteira em
// uma consulta só.
const TTLCache = 2 * time.Minute

type entradaCache struct {
	permissoes map[string][]string
	expiraEm   time.Time
}

type HTTPChecker struct {
	baseURL string
	http    *http.Client

	mu    sync.RWMutex
	cache map[string]entradaCache
	agora func() time.Time
}

// NewHTTPChecker cria o verificador. baseURL é a URL base do
// autenticacao-service (env AUTENTICACAO_SERVICE_URL).
func NewHTTPChecker(baseURL string) *HTTPChecker {
	return &HTTPChecker{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		http:    &http.Client{Timeout: 10 * time.Second},
		cache:   map[string]entradaCache{},
		agora:   time.Now,
	}
}

func (c *HTTPChecker) doCache(usuarioId string) (map[string][]string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entrada, ok := c.cache[usuarioId]
	if !ok || c.agora().After(entrada.expiraEm) {
		return nil, false
	}
	return entrada.permissoes, true
}

func (c *HTTPChecker) guardar(usuarioId string, permissoes map[string][]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[usuarioId] = entradaCache{permissoes: permissoes, expiraEm: c.agora().Add(TTLCache)}
}

// Invalidar descarta as permissões em cache do usuário. Usar quando o próprio
// processo sabe que o cargo mudou.
func (c *HTTPChecker) Invalidar(usuarioId string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cache, usuarioId)
}

func contem(acoes []string, acao string) bool {
	for _, a := range acoes {
		if a == acao {
			return true
		}
	}
	return false
}

// TemPermissao consulta as permissões do usuário (empresa vem do token
// repassado) e verifica modulo:acao. Erros de rede/parse são propagados para o
// chamador tratar como fail-closed.
func (c *HTTPChecker) TemPermissao(ctx context.Context, bearer, usuarioId, modulo, acao string) (bool, error) {
	if c.baseURL == "" {
		return false, fmt.Errorf("AUTENTICACAO_SERVICE_URL não configurada")
	}
	if permissoes, ok := c.doCache(usuarioId); ok {
		return contem(permissoes[modulo], acao), nil
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
		// Resposta sem sucesso não é cacheada: pode ser estado transitório, e
		// gravar "sem permissão" por 2 minutos negaria acesso legítimo.
		return false, nil
	}

	c.guardar(usuarioId, parsed.Conteudo)
	return contem(parsed.Conteudo[modulo], acao), nil
}
