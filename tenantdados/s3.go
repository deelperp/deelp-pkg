package tenantdados

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

type ArmazenamentoS3 interface {
	ListarPrefixo(ctx context.Context, prefixo string) ([]string, error)
	Download(ctx context.Context, key string) ([]byte, error)
	Excluir(ctx context.Context, key string) error
}

// PrefixoS3 declara os arquivos do tenant sob um prefixo do bucket. O bucket é
// único para todos os tenants: o prefixo precisa terminar no id da empresa
// (com "/"), senão "…/empresa/1" casaria "…/empresa/12".
type PrefixoS3 struct {
	S3             ArmazenamentoS3
	NomeFonte      string
	Prefixo        func(empresaID uuid.UUID) string
	PastaExportada string
	FaseExpurgo    int
	Retida         bool
	MotivoRetencao string
}

func (p PrefixoS3) Nome() string { return p.NomeFonte }
func (p PrefixoS3) Fase() int    { return p.FaseExpurgo }

func (p PrefixoS3) chaves(ctx context.Context, empresaID uuid.UUID) (string, []string, error) {
	prefixo := p.Prefixo(empresaID)
	if !strings.HasSuffix(prefixo, "/") {
		prefixo += "/"
	}
	chaves, err := p.S3.ListarPrefixo(ctx, prefixo)
	return prefixo, chaves, err
}

func (p PrefixoS3) Exportar(ctx context.Context, empresaID uuid.UUID) ([]Arquivo, error) {
	if p.PastaExportada == "" {
		return nil, nil
	}
	prefixo, chaves, err := p.chaves(ctx, empresaID)
	if err != nil {
		return nil, err
	}
	arquivos := make([]Arquivo, 0, len(chaves))
	for _, chave := range chaves {
		conteudo, err := p.S3.Download(ctx, chave)
		if err != nil {
			return nil, err
		}
		arquivos = append(arquivos, Arquivo{Nome: p.PastaExportada + "/" + strings.TrimPrefix(chave, prefixo), Conteudo: conteudo})
	}
	return arquivos, nil
}

func (p PrefixoS3) Contar(ctx context.Context, empresaID uuid.UUID) (Contagem, error) {
	_, chaves, err := p.chaves(ctx, empresaID)
	return Contagem{Fonte: p.NomeFonte, Quantidade: int64(len(chaves)), Retido: p.Retida, Motivo: p.MotivoRetencao}, err
}

func (p PrefixoS3) Apagar(ctx context.Context, empresaID uuid.UUID) (Contagem, error) {
	if p.Retida {
		return p.Contar(ctx, empresaID)
	}
	_, chaves, err := p.chaves(ctx, empresaID)
	if err != nil {
		return Contagem{Fonte: p.NomeFonte}, err
	}
	for i, chave := range chaves {
		if err := p.S3.Excluir(ctx, chave); err != nil {
			return Contagem{Fonte: p.NomeFonte, Quantidade: int64(i)}, err
		}
	}
	return Contagem{Fonte: p.NomeFonte, Quantidade: int64(len(chaves))}, nil
}
