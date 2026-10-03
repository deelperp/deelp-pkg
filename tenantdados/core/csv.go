package core

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// bomUTF8 faz o Excel em português abrir o arquivo com acentuação certa.
var bomUTF8 = []byte{0xEF, 0xBB, 0xBF}

func valorCSV(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case []byte:
		return string(t)
	case string:
		return t
	case time.Time:
		return t.Format(time.RFC3339)
	case *time.Time:
		if t == nil {
			return ""
		}
		return t.Format(time.RFC3339)
	case uuid.UUID:
		return t.String()
	default:
		return fmt.Sprint(t)
	}
}

// MontarCSV usa ";" como separador: é o que o Excel configurado em pt-BR
// espera, e vírgula aparece dentro de valor decimal e de endereço.
func MontarCSV(colunas []string, linhas [][]any) ([]byte, error) {
	var buf bytes.Buffer
	buf.Write(bomUTF8)
	w := csv.NewWriter(&buf)
	w.Comma = ';'
	if err := w.Write(colunas); err != nil {
		return nil, err
	}
	registro := make([]string, len(colunas))
	for _, linha := range linhas {
		for i := range colunas {
			registro[i] = valorCSV(linha[i])
		}
		if err := w.Write(registro); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}
