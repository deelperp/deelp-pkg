package observabilidade

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRodadaTickerPropagaErro(t *testing.T) {
	esperado := errors.New("falhou")
	if err := RodadaTicker(context.Background(), "teste", time.Minute, func(context.Context) error { return esperado }); !errors.Is(err, esperado) {
		t.Fatalf("erro = %v, esperado %v", err, esperado)
	}
	if err := RodadaTicker(context.Background(), "teste", time.Minute, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
}
