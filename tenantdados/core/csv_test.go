package core

import (
	"strings"
	"testing"
	"time"
)

func TestMontarCSV(t *testing.T) {
	data := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	csv, err := MontarCSV([]string{"nome", "valor", "quando", "nulo"}, [][]any{{"Ação; teste", 10.5, data, nil}})
	if err != nil {
		t.Fatal(err)
	}
	texto := string(csv)
	if !strings.HasPrefix(texto, string(bomUTF8)) || !strings.Contains(texto, `"Ação; teste";10.5;2026-10-01T12:00:00Z;`) {
		t.Fatalf("CSV inesperado: %q", texto)
	}
}
