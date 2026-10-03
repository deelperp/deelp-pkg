package core

import (
	"context"

	"github.com/google/uuid"
)

// FonteFunc cobre o que não é tabela nem coleção: arquivos no S3, cache,
// anonimização com regra própria. Campos nil viram no-op.
type FonteFunc struct {
	NomeFonte    string
	FaseExpurgo  int
	ExportarFunc func(ctx context.Context, empresaID uuid.UUID) ([]Arquivo, error)
	ContarFunc   func(ctx context.Context, empresaID uuid.UUID) (Contagem, error)
	ApagarFunc   func(ctx context.Context, empresaID uuid.UUID) (Contagem, error)
}

func (f FonteFunc) Nome() string { return f.NomeFonte }
func (f FonteFunc) Fase() int    { return f.FaseExpurgo }

func (f FonteFunc) Exportar(ctx context.Context, empresaID uuid.UUID) ([]Arquivo, error) {
	if f.ExportarFunc == nil {
		return nil, nil
	}
	return f.ExportarFunc(ctx, empresaID)
}

func (f FonteFunc) Contar(ctx context.Context, empresaID uuid.UUID) (Contagem, error) {
	if f.ContarFunc == nil {
		return Contagem{Fonte: f.NomeFonte}, nil
	}
	return f.ContarFunc(ctx, empresaID)
}

func (f FonteFunc) Apagar(ctx context.Context, empresaID uuid.UUID) (Contagem, error) {
	if f.ApagarFunc == nil {
		return Contagem{Fonte: f.NomeFonte}, nil
	}
	return f.ApagarFunc(ctx, empresaID)
}
