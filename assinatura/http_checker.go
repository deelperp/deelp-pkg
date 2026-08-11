package assinatura

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// TTLCache é quanto tempo a situação do contrato fica válida em memória.
//
// Curto o bastante para que um pagamento libere o acesso em pouco tempo, longo
// o bastante para não fazer uma chamada HTTP por requisição de escrita.
const TTLCache = 2 * time.Minute

type entradaCache struct {
	ativo    bool
	motivo   string
	expiraEm time.Time
}

// HTTPChecker consulta a situação do contrato no cliente-service.
//
// O cache é por processo (cada réplica mantém o seu). Não usa Redis de
// propósito: a consequência de uma réplica ter dado 2 minutos mais velho é
// irrelevante aqui, e uma dependência a mais no caminho de escrita seria mais
// um ponto de falha.
type HTTPChecker struct {
	baseURL     string
	internalKey string
	httpClient  *http.Client

	mu    sync.RWMutex
	cache map[string]entradaCache
	agora func() time.Time
}

func NovoHTTPChecker(baseURL, internalKey string) *HTTPChecker {
	return &HTTPChecker{
		baseURL:     baseURL,
		internalKey: internalKey,
		httpClient:  &http.Client{Timeout: 5 * time.Second},
		cache:       map[string]entradaCache{},
		agora:       time.Now,
	}
}

func (c *HTTPChecker) ContratoAtivo(ctx context.Context, empresaId string) (bool, string, error) {
	if c.baseURL == "" || c.internalKey == "" {
		return false, "", fmt.Errorf("assinatura: checker não configurado")
	}

	if entrada, ok := c.doCache(empresaId); ok {
		return entrada.ativo, entrada.motivo, nil
	}

	ativo, motivo, err := c.consultar(ctx, empresaId)
	if err != nil {
		return false, "", err
	}

	c.guardar(empresaId, entradaCache{ativo: ativo, motivo: motivo, expiraEm: c.agora().Add(TTLCache)})
	return ativo, motivo, nil
}

func (c *HTTPChecker) doCache(empresaId string) (entradaCache, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entrada, ok := c.cache[empresaId]
	if !ok || c.agora().After(entrada.expiraEm) {
		return entradaCache{}, false
	}
	return entrada, true
}

func (c *HTTPChecker) guardar(empresaId string, entrada entradaCache) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[empresaId] = entrada
}

func (c *HTTPChecker) consultar(ctx context.Context, empresaId string) (bool, string, error) {
	endpoint := fmt.Sprintf("%s/cliente-service/v1/assinatura/interno/%s", c.baseURL, empresaId)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, "", err
	}
	req.Header.Set("X-Internal-Key", c.internalKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()

	corpo, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return false, "", err
	}
	if resp.StatusCode != http.StatusOK {
		return false, "", fmt.Errorf("assinatura: cliente-service devolveu status %d", resp.StatusCode)
	}

	var envelope struct {
		Sucesso  bool `json:"sucesso"`
		Conteudo struct {
			ContratoAtivo    bool   `json:"contratoAtivo"`
			ContratoSituacao string `json:"contratoSituacao"`
		} `json:"conteudo"`
	}
	if err := json.Unmarshal(corpo, &envelope); err != nil {
		return false, "", fmt.Errorf("assinatura: corpo ilegível: %w", err)
	}
	if !envelope.Sucesso {
		return false, "", fmt.Errorf("assinatura: consulta sem sucesso")
	}

	return envelope.Conteudo.ContratoAtivo, envelope.Conteudo.ContratoSituacao, nil
}

// Invalidar descarta a entrada em cache do tenant. Usado quando o próprio
// processo sabe que a situação mudou (ex.: contrato recém-ativado).
func (c *HTTPChecker) Invalidar(empresaId string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cache, empresaId)
}
