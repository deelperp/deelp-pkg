// Package tenantdados é a fachada de compatibilidade do contrato de exportação
// e expurgo dos dados de um tenant. Código novo importa direto o núcleo
// (tenantdados/core, sem drivers nem HTTP) e só o adapter que usa
// (tenantdados/postgres, tenantdados/mongo, tenantdados/transporte).
//
// O orquestrador (cliente-service) chama os serviços pela rota interna com
// X-Internal-Key e X-Empresa-Id. O expurgo roda em duas fases porque há FK
// cruzando serviços: fase 1 apaga movimento em todos os serviços, fase 2
// apaga cadastro-mestre.
package tenantdados

import (
	"context"
	"io"
	"net/http"

	"github.com/deelperp/deelp-pkg/internalauth"
	"github.com/deelperp/deelp-pkg/tenantdados/core"
	tdmongo "github.com/deelperp/deelp-pkg/tenantdados/mongo"
	tdpostgres "github.com/deelperp/deelp-pkg/tenantdados/postgres"
	"github.com/deelperp/deelp-pkg/tenantdados/transporte"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	FaseMovimento = core.FaseMovimento
	FaseCadastro  = core.FaseCadastro

	HeaderEmpresa     = transporte.HeaderEmpresa
	CaminhoExportacao = transporte.CaminhoExportacao
	CaminhoExpurgo    = transporte.CaminhoExpurgo
)

var ErrFaseInvalida = core.ErrFaseInvalida

type (
	Arquivo          = core.Arquivo
	Contagem         = core.Contagem
	RelatorioExpurgo = core.RelatorioExpurgo
	Fonte            = core.Fonte
	FonteFunc        = core.FonteFunc
	ArmazenamentoS3  = core.ArmazenamentoS3
	PrefixoS3        = core.PrefixoS3
	TabelaPG         = tdpostgres.TabelaPG
	ColecaoMongo     = tdmongo.ColecaoMongo
	Cliente          = transporte.Cliente
)

// Servico mantém os métodos HTTP que os serviços já chamam; o núcleo vive em
// core.Servico, que tem os mesmos campos.
type Servico struct {
	Nome   string
	Fontes []Fonte
}

func (s Servico) Nucleo() core.Servico { return core.Servico(s) }

func (s Servico) Exportar(ctx context.Context, empresaID uuid.UUID) ([]Arquivo, error) {
	return s.Nucleo().Exportar(ctx, empresaID)
}

func (s Servico) Expurgar(ctx context.Context, empresaID uuid.UUID, fase int, simular bool) (RelatorioExpurgo, error) {
	return s.Nucleo().Expurgar(ctx, empresaID, fase, simular)
}

func (s Servico) HandlerExportacao(v *internalauth.Verificador) http.HandlerFunc {
	return transporte.HandlerExportacao(s.Nucleo(), v)
}

func (s Servico) HandlerExpurgo(v *internalauth.Verificador) http.HandlerFunc {
	return transporte.HandlerExpurgo(s.Nucleo(), v)
}

func (s Servico) Envolver(prefixo string, v *internalauth.Verificador, proximo http.Handler) http.Handler {
	return transporte.Envolver(s.Nucleo(), prefixo, v, proximo)
}

func EscreverZip(w io.Writer, arquivos []Arquivo) error { return core.EscreverZip(w, arquivos) }

func NovoCliente(servico, baseURL, prefixo, internalKey string) *Cliente {
	return transporte.NovoCliente(servico, baseURL, prefixo, internalKey)
}

func IdentidadesUUID(id uuid.UUID) bson.A { return tdmongo.IdentidadesUUID(id) }

func FiltroPorCampo(campo string) func(uuid.UUID) bson.M { return tdmongo.FiltroPorCampo(campo) }
