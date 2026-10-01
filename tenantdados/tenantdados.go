// Package tenantdados implementa o contrato de exportação e expurgo dos dados
// de um tenant, repetido em cada serviço dono de dados. Cada serviço declara
// suas fontes num lugar só; exportar, contar (simulação) e apagar saem da
// mesma declaração — tabela nova esquecida aqui fica fora dos três, e o teste
// de inventário de cada serviço existe para pegar isso.
//
// O orquestrador (cliente-service) chama os serviços pela rota interna com
// X-Internal-Key e X-Empresa-Id. O expurgo roda em duas fases porque há FK
// cruzando serviços (título → cliente, nota de entrada → plano de contas,
// cotação → material): fase 1 apaga movimento em todos os serviços, fase 2
// apaga cadastro-mestre.
package tenantdados

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
)

const (
	FaseMovimento = 1
	FaseCadastro  = 2

	HeaderEmpresa = "X-Empresa-Id"
)

var ErrFaseInvalida = errors.New("fase de expurgo deve ser 1 (movimento) ou 2 (cadastro)")

// Arquivo é um item do ZIP do serviço. O orquestrador prefixa com o nome do
// serviço; Nome é relativo (ex.: "clientes.csv", "xml/nfe/2026-09/123.xml").
type Arquivo struct {
	Nome     string
	Conteudo []byte
}

// Contagem é uma linha do relatório de expurgo. Retido indica dado que fica
// por obrigação legal (ex.: documento fiscal dentro dos 5 anos).
type Contagem struct {
	Fonte      string `json:"fonte"`
	Quantidade int64  `json:"quantidade"`
	Retido     bool   `json:"retido,omitempty"`
	Motivo     string `json:"motivo,omitempty"`
}

type RelatorioExpurgo struct {
	Servico  string     `json:"servico"`
	Fase     int        `json:"fase"`
	Simulado bool       `json:"simulado"`
	Itens    []Contagem `json:"itens"`
}

// Fonte é qualquer origem de dados do tenant (tabela, coleção, prefixo S3).
type Fonte interface {
	Nome() string
	Fase() int
	Exportar(ctx context.Context, empresaID uuid.UUID) ([]Arquivo, error)
	Contar(ctx context.Context, empresaID uuid.UUID) (Contagem, error)
	Apagar(ctx context.Context, empresaID uuid.UUID) (Contagem, error)
}

// Servico agrupa as fontes de um serviço na ordem em que devem ser apagadas
// dentro de cada fase (filhos antes dos pais).
type Servico struct {
	Nome   string
	Fontes []Fonte
}

func (s Servico) Exportar(ctx context.Context, empresaID uuid.UUID) ([]Arquivo, error) {
	arquivos := make([]Arquivo, 0, len(s.Fontes))
	for _, f := range s.Fontes {
		lote, err := f.Exportar(ctx, empresaID)
		if err != nil {
			return nil, fmt.Errorf("exportar %s: %w", f.Nome(), err)
		}
		arquivos = append(arquivos, lote...)
	}
	sort.SliceStable(arquivos, func(i, j int) bool { return arquivos[i].Nome < arquivos[j].Nome })
	return arquivos, nil
}

// Expurgar apaga (ou só conta, quando simular) as fontes da fase pedida, na
// ordem declarada. Para no primeiro erro: apagar pela metade sem saber onde
// parou é pior que não apagar.
func (s Servico) Expurgar(ctx context.Context, empresaID uuid.UUID, fase int, simular bool) (RelatorioExpurgo, error) {
	if fase != FaseMovimento && fase != FaseCadastro {
		return RelatorioExpurgo{}, ErrFaseInvalida
	}
	relatorio := RelatorioExpurgo{Servico: s.Nome, Fase: fase, Simulado: simular, Itens: []Contagem{}}
	for _, f := range s.Fontes {
		if f.Fase() != fase {
			continue
		}
		var (
			c   Contagem
			err error
		)
		if simular {
			c, err = f.Contar(ctx, empresaID)
		} else {
			c, err = f.Apagar(ctx, empresaID)
		}
		if err != nil {
			return relatorio, fmt.Errorf("%s: %w", f.Nome(), err)
		}
		relatorio.Itens = append(relatorio.Itens, c)
	}
	return relatorio, nil
}

func nomeArquivo(fonte string) string {
	nome := strings.ReplaceAll(fonte, ".", "__")
	return nome + ".csv"
}
