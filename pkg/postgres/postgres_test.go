package postgres

import (
	"strings"
	"testing"
	"time"
)

func TestValidar_ExigeCamposObrigatorios(t *testing.T) {
	casos := []struct {
		nome     string
		cfg      Config
		querErro bool
	}{
		{"sem host", Config{Port: 5432, User: "u", Name: "n"}, true},
		{"port invalido", Config{Host: "x", Port: 0, User: "u", Name: "n"}, true},
		{"sem user", Config{Host: "x", Port: 5432, Name: "n"}, true},
		{"sem name", Config{Host: "x", Port: 5432, User: "u"}, true},
		{"tudo ok", Config{Host: "x", Port: 5432, User: "u", Name: "n"}, false},
	}
	for _, c := range casos {
		err := c.cfg.validar()
		if c.querErro && err == nil {
			t.Errorf("[%s] esperado erro, recebeu nil", c.nome)
		}
		if !c.querErro && err != nil {
			t.Errorf("[%s] erro inesperado: %v", c.nome, err)
		}
	}
}

func TestOuPadrao_RetornaPadraoQuandoZero(t *testing.T) {
	if got := ouPadrao(0, 50); got != 50 {
		t.Errorf("esperado 50, obteve %d", got)
	}
	if got := ouPadrao(10, 50); got != 10 {
		t.Errorf("esperado 10 (valor explicito), obteve %d", got)
	}
	if got := ouPadrao(-1, 50); got != 50 {
		t.Errorf("esperado 50 quando negativo, obteve %d", got)
	}
}

func TestOuPadraoDuration_RetornaPadraoQuandoZero(t *testing.T) {
	if got := ouPadraoDuration(0, time.Minute); got != time.Minute {
		t.Errorf("esperado 1m, obteve %v", got)
	}
	if got := ouPadraoDuration(30*time.Second, time.Minute); got != 30*time.Second {
		t.Errorf("esperado 30s, obteve %v", got)
	}
}

// Conectar real precisa de Postgres rodando; aqui só validamos o caminho de erro
// quando a Config é inválida (cobertura sem rede).
func TestConectar_ErroSeConfigInvalida(t *testing.T) {
	_, err := Conectar(nil, Config{})
	if err == nil {
		t.Fatal("esperado erro de validacao")
	}
	if !strings.Contains(err.Error(), "postgres") {
		t.Errorf("erro deve mencionar 'postgres', obteve: %v", err)
	}
}
