package mensageria

import (
	"strings"
	"testing"
)

func TestValidar_ExigeCamposObrigatorios(t *testing.T) {
	casos := []struct {
		nome     string
		cfg      Config
		querErro bool
	}{
		{"sem host", Config{Port: 5672, User: "u"}, true},
		{"port invalido", Config{Host: "x", User: "u"}, true},
		{"sem user", Config{Host: "x", Port: 5672}, true},
		{"ok", Config{Host: "x", Port: 5672, User: "u", Password: "p"}, false},
	}
	for _, c := range casos {
		err := c.cfg.validar()
		if c.querErro && err == nil {
			t.Errorf("[%s] esperado erro", c.nome)
		}
		if !c.querErro && err != nil {
			t.Errorf("[%s] erro inesperado: %v", c.nome, err)
		}
	}
}

func TestURL_VHostDefault(t *testing.T) {
	cfg := Config{Host: "rabbit", Port: 5672, User: "u", Password: "p"}
	got := cfg.urlAMQP()
	want := "amqp://u:p@rabbit:5672/"
	if got != want {
		t.Errorf("esperado %q, obteve %q", want, got)
	}
}

func TestURL_VHostCustom(t *testing.T) {
	cfg := Config{Host: "r", Port: 5672, User: "u", Password: "p", VHost: "/deelp"}
	got := cfg.urlAMQP()
	if !strings.HasSuffix(got, "/deelp") {
		t.Errorf("vhost custom nao aplicado: %q", got)
	}
}

func TestURL_EscapaCredenciais(t *testing.T) {
	cfg := Config{
		Host: "r", Port: 5672,
		User: "user@deelp", Password: "se/nha?@",
	}
	got := cfg.urlAMQP()
	if strings.Contains(got, "user@deelp:") {
		t.Errorf("user nao foi escapado: %q", got)
	}
	if strings.Contains(got, "se/nha?@") {
		t.Errorf("password nao foi escapada: %q", got)
	}
}
