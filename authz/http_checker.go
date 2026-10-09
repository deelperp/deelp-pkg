// Package authz fornece um verificador de permissões que consulta o
// autenticacao-service (fonte de verdade) via REST, repassando o token do
// usuário. Serve a qualquer microserviço que não possua a tabela de
// permissões localmente. Satisfaz auth.PermissaoCheckerRemoto.
package authz

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/deelperp/deelp-pkg/observabilidade"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/deelperp/deelp-pkg/auth"
	"github.com/deelperp/deelp-pkg/internal/ttlcache"
)

// TTLCache é por quanto tempo o mapa de permissões do usuário fica válido em
// memória.
//
// Sem cache, CADA requisição protegida de CADA serviço fazia uma ida e volta
// HTTP ao autenticacao-service. Carregar uma tela com várias chamadas
// protegidas gerava dezenas de consultas, estourava o rate limiter (400
// req/min por IP, compartilhado com /entrar) e derrubava o próprio login.
//
// 2 minutos é curto o bastante para uma revogação de permissão surtir efeito
// rápido e longo o bastante para colapsar a rajada de uma navegação inteira em
// uma consulta só.
const TTLCache = 2 * time.Minute

// CapacidadeCache limita quantos pares usuário×token ficam em memória. Cada
// login gera um token novo; sem teto, um processo longevo acumula entradas.
const CapacidadeCache = 10_000

type permissoesEmCache struct {
	usuarioId  string
	permissoes map[string][]string
}

type HTTPChecker struct {
	baseURL string
	http    *http.Client

	cache *ttlcache.Cache[string, permissoesEmCache]
	agora func() time.Time
}

// NewHTTPChecker cria o verificador. baseURL é a URL base do
// autenticacao-service (env AUTENTICACAO_SERVICE_URL).
func NewHTTPChecker(baseURL string) *HTTPChecker {
	c := &HTTPChecker{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		http:    &http.Client{Timeout: 10 * time.Second, Transport: observabilidade.Transporte(nil)},
		agora:   time.Now,
	}
	c.cache = ttlcache.New[string, permissoesEmCache](CapacidadeCache, func() time.Time { return c.agora() })
	return c
}

func chaveCache(usuarioId, bearer string) string {
	soma := sha256.Sum256([]byte(usuarioId + "\x00" + bearer))
	return hex.EncodeToString(soma[:])
}

func (c *HTTPChecker) guardar(chave, usuarioId string, permissoes map[string][]string) {
	c.cache.Set(chave, permissoesEmCache{usuarioId: usuarioId, permissoes: permissoes}, TTLCache)
}

// Invalidar descarta as permissões em cache do usuário. Usar quando o próprio
// processo sabe que o cargo mudou.
func (c *HTTPChecker) Invalidar(usuarioId string) {
	c.cache.DeleteFunc(func(_ string, v permissoesEmCache) bool { return v.usuarioId == usuarioId })
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
// Sessões de suporte são revalidadas em toda chamada, sem cache.
func (c *HTTPChecker) TemPermissao(ctx context.Context, bearer, usuarioId, modulo, acao string) (bool, error) {
	if c.baseURL == "" {
		return false, fmt.Errorf("AUTENTICACAO_SERVICE_URL não configurada")
	}
	if auth.EhSessaoSuporte(ctx) {
		permissoes, _, err := c.consultar(ctx, bearer, usuarioId)
		if err != nil {
			return false, err
		}
		return contem(permissoes[modulo], acao), nil
	}
	encontrado, err := c.cache.Load(ctx, chaveCache(usuarioId, bearer), func(ctx context.Context) (permissoesEmCache, time.Duration, error) {
		permissoes, sucesso, err := c.consultar(ctx, bearer, usuarioId)
		if err != nil || !sucesso {
			// Resposta sem sucesso não é cacheada: pode ser estado transitório, e
			// gravar "sem permissão" por 2 minutos negaria acesso legítimo.
			return permissoesEmCache{}, 0, err
		}
		return permissoesEmCache{usuarioId: usuarioId, permissoes: permissoes}, TTLCache, nil
	})
	if err != nil {
		return false, err
	}
	return contem(encontrado.permissoes[modulo], acao), nil
}

func (c *HTTPChecker) consultar(ctx context.Context, bearer, usuarioId string) (map[string][]string, bool, error) {
	url := fmt.Sprintf("%s/autenticacao-service/v1/usuarios/%s/permissoes", c.baseURL, usuarioId)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
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
		return nil, false, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, false, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, false, fmt.Errorf("autenticacao-service permissoes: status %d", resp.StatusCode)
	}
	var parsed struct {
		Sucesso  bool                `json:"sucesso"`
		Conteudo map[string][]string `json:"conteudo"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, false, fmt.Errorf("resposta inválida do autenticacao-service: %w", err)
	}
	if !parsed.Sucesso {
		return nil, false, nil
	}
	return parsed.Conteudo, true, nil
}
