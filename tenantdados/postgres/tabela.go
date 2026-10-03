package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"

	"github.com/deelperp/deelp-pkg/tenantdados/core"
	"github.com/google/uuid"
)

// TabelaPG declara uma tabela do tenant. Filtro é a condição do WHERE com $1
// = empresaId; tabela filha sem empresa_id filtra pelo pai via subconsulta.
// Retida exporta e não apaga (motivo vai no relatório). Anonimizar, quando
// preenchido, troca o DELETE por um UPDATE ... SET <Anonimizar>: a linha fica
// para não quebrar referência, sem dado pessoal.
type TabelaPG struct {
	DB             *sql.DB
	Tabela         string
	Filtro         string
	FaseExpurgo    int
	Retida         bool
	MotivoRetencao string
	Anonimizar     string
	// Colunas restringe a exportação (padrão "*"): tabela com segredo, como
	// hash de senha, não pode sair inteira.
	Colunas     string
	NaoExportar bool
	// Opcional tolera tabela ausente no banco (criada fora dos scripts de
	// estrutura): conta zero em vez de derrubar o expurgo inteiro.
	Opcional bool
}

func (t TabelaPG) ausente(err error) bool {
	var pqErr *pq.Error
	return t.Opcional && errors.As(err, &pqErr) && pqErr.Code == "42P01"
}

func (t TabelaPG) Nome() string { return t.Tabela }
func (t TabelaPG) Fase() int    { return t.FaseExpurgo }

func (t TabelaPG) Exportar(ctx context.Context, empresaID uuid.UUID) ([]core.Arquivo, error) {
	if t.NaoExportar {
		return nil, nil
	}
	colunas := t.Colunas
	if colunas == "" {
		colunas = "*"
	}
	rows, err := t.DB.QueryContext(ctx, fmt.Sprintf("SELECT %s FROM %s WHERE %s", colunas, t.Tabela, t.Filtro), empresaID)
	if t.ausente(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	nomes, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	linhas := make([][]any, 0)
	for rows.Next() {
		valores := make([]any, len(nomes))
		ponteiros := make([]any, len(nomes))
		for i := range valores {
			ponteiros[i] = &valores[i]
		}
		if err := rows.Scan(ponteiros...); err != nil {
			return nil, err
		}
		linhas = append(linhas, valores)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	conteudo, err := core.MontarCSV(nomes, linhas)
	if err != nil {
		return nil, err
	}
	return []core.Arquivo{{Nome: core.NomeArquivo(t.Tabela), Conteudo: conteudo}}, nil
}

func (t TabelaPG) Contar(ctx context.Context, empresaID uuid.UUID) (core.Contagem, error) {
	var n int64
	err := t.DB.QueryRowContext(ctx, fmt.Sprintf("SELECT count(*) FROM %s WHERE %s", t.Tabela, t.Filtro), empresaID).Scan(&n)
	if t.ausente(err) {
		err = nil
	}
	return core.Contagem{Fonte: t.Tabela, Quantidade: n, Retido: t.Retida, Motivo: t.MotivoRetencao}, err
}

func (t TabelaPG) Apagar(ctx context.Context, empresaID uuid.UUID) (core.Contagem, error) {
	if t.Retida {
		return t.Contar(ctx, empresaID)
	}
	comando := fmt.Sprintf("DELETE FROM %s WHERE %s", t.Tabela, t.Filtro)
	if t.Anonimizar != "" {
		comando = fmt.Sprintf("UPDATE %s SET %s WHERE %s", t.Tabela, t.Anonimizar, t.Filtro)
	}
	res, err := t.DB.ExecContext(ctx, comando, empresaID)
	if t.ausente(err) {
		return core.Contagem{Fonte: t.Tabela}, nil
	}
	if err != nil {
		return core.Contagem{Fonte: t.Tabela}, err
	}
	n, _ := res.RowsAffected()
	return core.Contagem{Fonte: t.Tabela, Quantidade: n}, nil
}
