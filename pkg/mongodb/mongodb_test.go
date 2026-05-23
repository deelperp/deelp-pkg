package mongodb

import (
	"strings"
	"testing"
)

func TestValidar_ExigeName(t *testing.T) {
	cfg := Config{Host: "x", Port: 27017}
	if err := cfg.validar(); err == nil {
		t.Fatal("esperado erro por Name ausente")
	}
}

func TestValidar_AceitaURICompleta(t *testing.T) {
	cfg := Config{URI: "mongodb://x:27017/?replicaSet=rs0", Name: "db"}
	if err := cfg.validar(); err != nil {
		t.Fatalf("erro inesperado com URI: %v", err)
	}
}

func TestValidar_ExigeHostQuandoSemURI(t *testing.T) {
	cfg := Config{Name: "db"}
	if err := cfg.validar(); err == nil {
		t.Fatal("esperado erro por Host ausente sem URI")
	}
}

func TestValidar_ExigePortQuandoSemURI(t *testing.T) {
	cfg := Config{Host: "x", Name: "db"}
	if err := cfg.validar(); err == nil {
		t.Fatal("esperado erro por Port invalido sem URI")
	}
}

func TestURI_RetornaURIDireta(t *testing.T) {
	want := "mongodb://x:27017/?replicaSet=rs0"
	cfg := Config{URI: want, Name: "db"}
	if got := cfg.uri(); got != want {
		t.Errorf("esperado %q, obteve %q", want, got)
	}
}

func TestURI_BuildaSemAuth(t *testing.T) {
	cfg := Config{Host: "host", Port: 27017, Name: "db"}
	got := cfg.uri()
	want := "mongodb://host:27017"
	if got != want {
		t.Errorf("esperado %q, obteve %q", want, got)
	}
}

func TestURI_BuildaComAuthDefault(t *testing.T) {
	cfg := Config{Host: "h", Port: 27017, User: "u", Password: "p", Name: "db"}
	got := cfg.uri()
	if !strings.Contains(got, "authSource=admin") {
		t.Errorf("authSource default deve ser admin, obteve %q", got)
	}
	if !strings.Contains(got, "authMechanism=SCRAM-SHA-256") {
		t.Errorf("authMechanism default deve ser SCRAM-SHA-256, obteve %q", got)
	}
	if !strings.Contains(got, "u:p@h:27017") {
		t.Errorf("credenciais ausentes na URI, obteve %q", got)
	}
}

func TestURI_RespeitaAuthSourceECustom(t *testing.T) {
	cfg := Config{
		Host: "h", Port: 27017, User: "u", Password: "p", Name: "db",
		AuthSource: "outroDB", AuthMechanism: "PLAIN",
	}
	got := cfg.uri()
	if !strings.Contains(got, "authSource=outroDB") {
		t.Errorf("authSource custom nao aplicado: %q", got)
	}
	if !strings.Contains(got, "authMechanism=PLAIN") {
		t.Errorf("authMechanism custom nao aplicado: %q", got)
	}
}

func TestURI_AuthMechanismAutoNaoEnviaParam(t *testing.T) {
	cfg := Config{
		Host: "h", Port: 27017, User: "u", Password: "p", Name: "db",
		AuthMechanism: "auto",
	}
	got := cfg.uri()
	if strings.Contains(got, "authMechanism") {
		t.Errorf("authMechanism=auto deve deixar o driver negociar (sem param), obteve %q", got)
	}
}

func TestURI_EscapaCredenciaisComCaracteresEspeciais(t *testing.T) {
	cfg := Config{
		Host: "h", Port: 27017,
		User: "user@deelp", Password: "se/nha?@!",
		Name: "db",
	}
	got := cfg.uri()
	if strings.Contains(got, "user@deelp:") {
		t.Errorf("user nao foi escapado: %q", got)
	}
	if strings.Contains(got, "se/nha?@!") {
		t.Errorf("password nao foi escapada: %q", got)
	}
}
