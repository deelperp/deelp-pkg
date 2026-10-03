package assinatura

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/deelperp/deelp-pkg/internal/ttlcache"
)

// TTLCache é quanto tempo a situação do contrato fica válida em memória.
//
// Curto o bastante para que um pagamento libere o acesso em pouco tempo, longo
// o bastante para não fazer uma chamada HTTP por requisição de escrita.
const TTLCache = 2 * time.Minute

// TTLCacheBloqueado é bem menor de propósito. O cache é assimétrico porque o
// erro é assimétrico: guardar "liberado" por 2 minutos custa uma janela curta
// de uso indevido, enquanto guardar "bloqueado" pelo mesmo tempo deixa quem
// acabou de contratar tomando 402 depois de ver "plano ativado" na tela.
const TTLCacheBloqueado = 10 * time.Second

// CapacidadeCache limita quantas empresas ficam em memória por processo.
const CapacidadeCache = 10_000

type entradaCache struct {
	ativo  bool
	motivo string
}

// HTTPChecker consulta a situação do contrato no cliente-service repassando o
// Bearer do request original, mesmo padrão de pkg/authz.HTTPChecker. O cache é
// por processo (cada réplica mantém o seu); não usa Redis de propósito.
type HTTPChecker struct {
	baseURL    string
	httpClient *http.Client

	cache *ttlcache.Cache[string, entradaCache]
	agora func() time.Time
}

func NewHTTPChecker(baseURL string) *HTTPChecker {
	c := &HTTPChecker{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		agora:      time.Now,
	}
	c.cache = ttlcache.New[string, entradaCache](CapacidadeCache, func() time.Time { return c.agora() })
	return c
}

func (c *HTTPChecker) ContratoAtivo(ctx context.Context, bearer, empresaId string) (bool, string, error) {
	if c.baseURL == "" || bearer == "" {
		return false, "", fmt.Errorf("assinatura: checker não configurado")
	}

	entrada, err := c.cache.Load(ctx, empresaId, func(ctx context.Context) (entradaCache, time.Duration, error) {
		ativo, motivo, err := c.consultar(ctx, bearer)
		if err != nil {
			return entradaCache{}, 0, err
		}
		ttl := TTLCache
		if !ativo {
			ttl = TTLCacheBloqueado
		}
		return entradaCache{ativo: ativo, motivo: motivo}, ttl, nil
	})
	if err != nil {
		return false, "", err
	}
	return entrada.ativo, entrada.motivo, nil
}

func (c *HTTPChecker) consultar(ctx context.Context, bearer string) (bool, string, error) {
	endpoint := fmt.Sprintf("%s/cliente-service/v1/assinatura/contrato-situacao", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, "", err
	}
	req.Header.Set("Authorization", bearer)

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
	c.cache.Delete(empresaId)
}
