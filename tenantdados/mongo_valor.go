package tenantdados

import (
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func valorMongoSimples(v any) any {
	switch t := v.(type) {
	case primitive.DateTime:
		return t.Time().UTC().Format(time.RFC3339)
	case primitive.ObjectID:
		return t.Hex()
	case primitive.Binary:
		if t.Subtype == 0x04 || t.Subtype == 0x03 || len(t.Data) == 16 {
			if id, err := uuid.FromBytes(t.Data); err == nil {
				return id.String()
			}
		}
		return t.Data
	case primitive.Decimal128:
		return t.String()
	}
	return v
}

// IdentidadesUUID cobre as quatro formas em que o UUID já foi gravado nos
// serviços Mongo (UUID nativo, texto e binário subtipos 4 e 0). Filtrar por uma
// só deixaria documentos antigos fora da exportação e do expurgo.
func IdentidadesUUID(id uuid.UUID) bson.A {
	return bson.A{
		id,
		id.String(),
		primitive.Binary{Subtype: 0x04, Data: append([]byte(nil), id[:]...)},
		primitive.Binary{Subtype: 0x00, Data: append([]byte(nil), id[:]...)},
	}
}

// FiltroPorCampo é o filtro mais comum: campo de tenant em qualquer das formas.
func FiltroPorCampo(campo string) func(uuid.UUID) bson.M {
	return func(id uuid.UUID) bson.M {
		return bson.M{campo: bson.M{"$in": IdentidadesUUID(id)}}
	}
}
