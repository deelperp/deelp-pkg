package mongo

import (
	"context"
	"sort"

	"github.com/deelperp/deelp-pkg/tenantdados/core"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	mongodriver "go.mongodb.org/mongo-driver/mongo"
)

// ColecaoMongo declara uma coleção do tenant. Filtro recebe o empresaId e
// devolve o filtro (o tipo do campo varia por serviço: string ou UUID binário).
// Campos aninhados viram JSON na célula do CSV — nada se perde na exportação.
type ColecaoMongo struct {
	DB             *mongodriver.Database
	Colecao        string
	Filtro         func(empresaID uuid.UUID) bson.M
	FaseExpurgo    int
	Retida         bool
	MotivoRetencao string
	// Rotulo distingue recortes da mesma coleção (ex.: rascunho e oficial)
	// no relatório e no nome do arquivo exportado.
	Rotulo string
	// NaoExportar evita repetir no ZIP um recorte que outro já exporta.
	NaoExportar bool
	// SomenteExportar entra no ZIP e fica fora do expurgo (outros recortes da
	// mesma coleção decidem o que sai).
	SomenteExportar bool
}

func (c ColecaoMongo) Nome() string {
	if c.Rotulo != "" {
		return "mongo." + c.Colecao + "." + c.Rotulo
	}
	return "mongo." + c.Colecao
}
func (c ColecaoMongo) Fase() int {
	if c.SomenteExportar {
		return 0
	}
	return c.FaseExpurgo
}

func (c ColecaoMongo) Exportar(ctx context.Context, empresaID uuid.UUID) ([]core.Arquivo, error) {
	if c.NaoExportar {
		return nil, nil
	}
	cursor, err := c.DB.Collection(c.Colecao).Find(ctx, c.Filtro(empresaID))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	documentos := make([]bson.M, 0)
	chaves := map[string]bool{}
	for cursor.Next(ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			return nil, err
		}
		for k := range doc {
			chaves[k] = true
		}
		documentos = append(documentos, doc)
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}

	colunas := make([]string, 0, len(chaves))
	for k := range chaves {
		colunas = append(colunas, k)
	}
	sort.Strings(colunas)
	linhas := make([][]any, 0, len(documentos))
	for _, doc := range documentos {
		linha := make([]any, len(colunas))
		for i, k := range colunas {
			linha[i] = celulaMongo(doc[k])
		}
		linhas = append(linhas, linha)
	}
	conteudo, err := core.MontarCSV(colunas, linhas)
	if err != nil {
		return nil, err
	}
	return []core.Arquivo{{Nome: core.NomeArquivo(c.Nome()), Conteudo: conteudo}}, nil
}

func celulaMongo(v any) any {
	switch v.(type) {
	case bson.M, bson.A, bson.D, map[string]any, []any:
		texto, err := bson.MarshalExtJSON(bson.M{"v": v}, false, false)
		if err != nil {
			return ""
		}
		return string(texto[len(`{"v":`) : len(texto)-1])
	}
	return valorMongoSimples(v)
}

func (c ColecaoMongo) Contar(ctx context.Context, empresaID uuid.UUID) (core.Contagem, error) {
	n, err := c.DB.Collection(c.Colecao).CountDocuments(ctx, c.Filtro(empresaID))
	return core.Contagem{Fonte: c.Nome(), Quantidade: n, Retido: c.Retida, Motivo: c.MotivoRetencao}, err
}

func (c ColecaoMongo) Apagar(ctx context.Context, empresaID uuid.UUID) (core.Contagem, error) {
	if c.Retida {
		return c.Contar(ctx, empresaID)
	}
	res, err := c.DB.Collection(c.Colecao).DeleteMany(ctx, c.Filtro(empresaID))
	if err != nil {
		return core.Contagem{Fonte: c.Nome()}, err
	}
	return core.Contagem{Fonte: c.Nome(), Quantidade: res.DeletedCount}, nil
}
