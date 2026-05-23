package observabilidade

import (
	"testing"
)

func TestValidar_ExigeNomeServico(t *testing.T) {
	c := Config{Endpoint: "x:4317"}
	if err := c.validar(); err == nil {
		t.Fatal("esperado erro por NomeServico vazio")
	}
}

func TestValidar_ExigeEndpoint(t *testing.T) {
	c := Config{NomeServico: "svc"}
	if err := c.validar(); err == nil {
		t.Fatal("esperado erro por Endpoint vazio")
	}
}

func TestProtocoloEfetivo_AutoDetectaHttpPorScheme(t *testing.T) {
	casos := []struct {
		endpoint string
		esperado Protocolo
	}{
		{"otel-collector:4317", ProtocoloGRPC},
		{"http://otel-collector:4318", ProtocoloHTTP},
		{"https://otel.deelp.com.br", ProtocoloHTTP},
		{"otel.deelp.com.br:4317", ProtocoloGRPC}, // sem scheme = gRPC
	}
	for _, caso := range casos {
		c := Config{NomeServico: "svc", Endpoint: caso.endpoint}
		if got := c.protocoloEfetivo(); got != caso.esperado {
			t.Errorf("endpoint=%q: esperado %q, obtido %q", caso.endpoint, caso.esperado, got)
		}
	}
}

func TestProtocoloEfetivo_RespeitaValorExplicito(t *testing.T) {
	c := Config{NomeServico: "svc", Endpoint: "http://x", Protocolo: ProtocoloGRPC}
	if c.protocoloEfetivo() != ProtocoloGRPC {
		t.Fatal("esperado ProtocoloGRPC quando explicito")
	}
}

func TestHostDoEndpoint(t *testing.T) {
	casos := []struct{ in, out string }{
		{"http://x:4318", "x:4318"},
		{"https://otel.deelp.com.br/path", "otel.deelp.com.br"},
		{"x:4317", "x:4317"},
		{"https://a/b/c", "a"},
	}
	for _, caso := range casos {
		if got := hostDoEndpoint(caso.in); got != caso.out {
			t.Errorf("in=%q: esperado %q, obtido %q", caso.in, caso.out, got)
		}
	}
}
