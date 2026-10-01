package tenantdados

import (
	"time"

	"github.com/google/uuid"
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
