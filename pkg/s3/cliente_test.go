package s3

import (
	"strings"
	"testing"
)

func TestValidar_ExigeRegion(t *testing.T) {
	cfg := Config{Bucket: "b"}
	if err := cfg.validar(); err == nil {
		t.Fatal("esperado erro por Region ausente")
	}
}

func TestValidar_ExigeBucket(t *testing.T) {
	cfg := Config{Region: "us-east-1"}
	if err := cfg.validar(); err == nil {
		t.Fatal("esperado erro por Bucket ausente")
	}
}

func TestValidar_OkComMinimo(t *testing.T) {
	cfg := Config{Region: "us-east-1", Bucket: "b"}
	if err := cfg.validar(); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
}

func TestNewCliente_ErroQuandoSemRegion(t *testing.T) {
	_, err := NewCliente(Config{Bucket: "b"})
	if err == nil {
		t.Fatal("esperado erro")
	}
	if !strings.Contains(err.Error(), "Region") {
		t.Errorf("erro deve mencionar 'Region', obteve %v", err)
	}
}

// NewCliente real cria sessao AWS — não precisa de credenciais ativas para
// instanciar (a session é lazy), mas o cliente pode ser usado offline para
// testar getters.
func TestCliente_Getters(t *testing.T) {
	c, err := NewCliente(Config{
		Region:    "us-east-1",
		Bucket:    "meu-bucket",
		AccessKey: "AKIA-fake",
		SecretKey: "secret-fake",
	})
	if err != nil {
		t.Fatalf("erro instanciando cliente: %v", err)
	}
	if c.Bucket() != "meu-bucket" {
		t.Errorf("Bucket(): esperado meu-bucket, obteve %q", c.Bucket())
	}
	if c.Region() != "us-east-1" {
		t.Errorf("Region(): esperado us-east-1, obteve %q", c.Region())
	}
}
