package cache

import (
	"testing"
	"time"
)

func TestValidar_ExigeHost(t *testing.T) {
	cfg := Config{Port: 6379}
	if err := cfg.validar(); err == nil {
		t.Fatal("esperado erro por Host ausente")
	}
}

func TestValidar_ExigePort(t *testing.T) {
	cfg := Config{Host: "x"}
	if err := cfg.validar(); err == nil {
		t.Fatal("esperado erro por Port invalido")
	}
}

func TestValidar_OkComMinimo(t *testing.T) {
	cfg := Config{Host: "x", Port: 6379}
	if err := cfg.validar(); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
}

func TestOuPadrao_Duration(t *testing.T) {
	if got := ouPadrao(0, 500*time.Millisecond); got != 500*time.Millisecond {
		t.Errorf("esperado padrao, obteve %v", got)
	}
	if got := ouPadrao(time.Second, 500*time.Millisecond); got != time.Second {
		t.Errorf("esperado valor explicito, obteve %v", got)
	}
}

func TestOuPadraoInt(t *testing.T) {
	if got := ouPadraoInt(0, 30); got != 30 {
		t.Errorf("esperado 30, obteve %d", got)
	}
	if got := ouPadraoInt(50, 30); got != 50 {
		t.Errorf("esperado 50, obteve %d", got)
	}
	if got := ouPadraoInt(-5, 30); got != 30 {
		t.Errorf("esperado 30 quando negativo, obteve %d", got)
	}
}
