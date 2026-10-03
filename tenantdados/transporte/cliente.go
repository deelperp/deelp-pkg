package transporte

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/deelperp/deelp-pkg/internalauth"
	"github.com/deelperp/deelp-pkg/tenantdados/core"
	"github.com/google/uuid"
)

// Cliente é o lado do orquestrador: fala com um serviço participante.
// Prefixo é o caminho do serviço (ex.: "/estoque-service/v1").
type Cliente struct {
	Servico     string
	BaseURL     string
	Prefixo     string
	InternalKey string
	HTTP        *http.Client
}

func NovoCliente(servico, baseURL, prefixo, internalKey string) *Cliente {
	return &Cliente{
		Servico: servico, BaseURL: strings.TrimRight(baseURL, "/"), Prefixo: prefixo, InternalKey: internalKey,
		HTTP: &http.Client{Timeout: 10 * time.Minute},
	}
}

func (c *Cliente) requisicao(ctx context.Context, metodo, caminho string, empresaID uuid.UUID) (*http.Response, error) {
	if c.BaseURL == "" {
		return nil, fmt.Errorf("URL do %s não configurada", c.Servico)
	}
	req, err := http.NewRequestWithContext(ctx, metodo, c.BaseURL+c.Prefixo+caminho, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(internalauth.Header, c.InternalKey)
	req.Header.Set(HeaderEmpresa, empresaID.String())
	return c.HTTP.Do(req)
}

func (c *Cliente) Exportar(ctx context.Context, empresaID uuid.UUID) ([]byte, error) {
	resp, err := c.requisicao(ctx, http.MethodGet, CaminhoExportacao, empresaID)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	corpo, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s respondeu %d: %s", c.Servico, resp.StatusCode, bytes.TrimSpace(corpo))
	}
	return corpo, nil
}

func (c *Cliente) Expurgar(ctx context.Context, empresaID uuid.UUID, fase int, simular bool) (core.RelatorioExpurgo, error) {
	caminho := fmt.Sprintf("%s?fase=%d&simular=%t", CaminhoExpurgo, fase, simular)
	resp, err := c.requisicao(ctx, http.MethodPost, caminho, empresaID)
	if err != nil {
		return core.RelatorioExpurgo{Servico: c.Servico, Fase: fase, Simulado: simular}, err
	}
	defer resp.Body.Close()
	var envelope struct {
		Sucesso  bool                  `json:"sucesso"`
		Mensagem string                `json:"mensagem"`
		Conteudo core.RelatorioExpurgo `json:"conteudo"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return core.RelatorioExpurgo{Servico: c.Servico, Fase: fase, Simulado: simular}, fmt.Errorf("%s respondeu %d sem relatório", c.Servico, resp.StatusCode)
	}
	envelope.Conteudo.Servico = c.Servico
	if !envelope.Sucesso || resp.StatusCode != http.StatusOK {
		return envelope.Conteudo, fmt.Errorf("%s: %s", c.Servico, envelope.Mensagem)
	}
	return envelope.Conteudo, nil
}
