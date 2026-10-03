package observabilidade

import (
	"context"
	"crypto/tls"
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

func TestResolveEndpoint(t *testing.T) {
	casos := []struct {
		name   string
		config Config
		want   endpointConfig
	}{
		{"grpc legado", Config{Endpoint: "collector:4317"}, endpointConfig{host: "collector:4317"}},
		{"http", Config{Endpoint: "http://collector:4318"}, endpointConfig{host: "collector:4318"}},
		{"https com prefixo", Config{Endpoint: "https://collector/otel/"}, endpointConfig{host: "collector", path: "/otel", secure: true}},
		{"grpc https explícito", Config{Endpoint: "https://collector:4317", Protocolo: ProtocoloGRPC}, endpointConfig{host: "collector:4317", secure: true}},
		{"tls sem esquema", Config{Endpoint: "collector:4317", TLSConfig: &tls.Config{}}, endpointConfig{host: "collector:4317", secure: true}},
		{"ipv6", Config{Endpoint: "[::1]:4317"}, endpointConfig{host: "[::1]:4317"}},
	}
	for _, caso := range casos {
		t.Run(caso.name, func(t *testing.T) {
			got, err := caso.config.resolveEndpoint()
			if err != nil || got != caso.want {
				t.Fatalf("resolveEndpoint() = %+v, %v; esperado %+v", got, err, caso.want)
			}
		})
	}
}

func TestIniciarRejeitaConfiguracaoInvalida(t *testing.T) {
	casos := []Config{
		{Endpoint: "collector:4317", Protocolo: "udp"},
		{Endpoint: "ftp://collector"},
		{Endpoint: "https://"},
		{Endpoint: "http://usuario:senha@collector"},
		{Endpoint: "https://collector?token=segredo"},
		{Endpoint: "https://collector?"},
		{Endpoint: "https://collector#traces"},
		{Endpoint: "https://collector:abc"},
		{Endpoint: "https://collector:65536"},
		{Endpoint: "https://collector:0"},
		{Endpoint: "https://collector:"},
		{Endpoint: "collector:4318/otel", Protocolo: ProtocoloHTTP},
		{Endpoint: "https://collector/otel", Protocolo: ProtocoloGRPC},
		{Endpoint: "http://collector", TLSConfig: &tls.Config{}},
		{Endpoint: "https://collector", TLSConfig: &tls.Config{InsecureSkipVerify: true}},
		{Endpoint: "collector:4317", TLSConfig: &tls.Config{InsecureSkipVerify: true}},
	}
	for _, config := range casos {
		t.Run(config.Endpoint+string(config.Protocolo), func(t *testing.T) {
			config.NomeServico = "test"
			shutdown, err := Iniciar(context.Background(), config)
			if err == nil {
				_ = shutdown(context.Background())
				t.Fatal("esperado erro de configuração")
			}
		})
	}
}
