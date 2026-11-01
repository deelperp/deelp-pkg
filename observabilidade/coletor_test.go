package observabilidade

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

func TestHabilitada(t *testing.T) {
	casos := map[string]bool{"": true, "true": true, "auto": true, "false": false, "0": false, "OFF": false, "não": false}
	for valor, esperado := range casos {
		t.Setenv(EnvHabilitada, valor)
		if got := Habilitada(); got != esperado {
			t.Errorf("%s=%q: esperado %v, obtido %v", EnvHabilitada, valor, esperado, got)
		}
	}
}

func TestIniciar_DesabilitadaNaoValidaNemSonda(t *testing.T) {
	t.Setenv(EnvHabilitada, "false")
	desligar, err := Iniciar(context.Background(), Config{})
	if err != nil {
		t.Fatalf("desabilitada não deve validar config: %v", err)
	}
	if err := desligar(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestIniciar_ColetorForaEntraEmEsperaEDesligaSemBloquear(t *testing.T) {
	t.Setenv(EnvHabilitada, "")
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	endereco := l.Addr().String()
	_ = l.Close()

	desligar, err := Iniciar(context.Background(), Config{NomeServico: "svc", Endpoint: endereco})
	if err != nil {
		t.Fatalf("collector fora não pode impedir o serviço de subir: %v", err)
	}
	feito := make(chan error, 1)
	go func() { feito <- desligar(context.Background()) }()
	select {
	case err := <-feito:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("desligar bloqueou com a sondagem em andamento")
	}
}

func TestIniciar_AtivaQuandoColetorSobeDepois(t *testing.T) {
	t.Setenv(EnvHabilitada, "")
	original := intervaloSondagem
	intervaloSondagem = 20 * time.Millisecond
	antes := otel.GetTracerProvider()
	otel.SetTracerProvider(tracenoop.NewTracerProvider())
	t.Cleanup(func() {
		intervaloSondagem = original
		otel.SetTracerProvider(antes)
	})

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	endereco := l.Addr().String()
	_ = l.Close()

	desligar, err := Iniciar(context.Background(), Config{NomeServico: "svc", Endpoint: "http://" + endereco})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider); ok {
		t.Fatal("não deveria instalar provider com o collector fora")
	}

	l, err = net.Listen("tcp", endereco)
	if err != nil {
		t.Skipf("porta reaproveitada por outro processo: %v", err)
	}
	defer l.Close()

	limite := time.Now().Add(2 * time.Second)
	for {
		if _, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider); ok {
			break
		}
		if time.Now().After(limite) {
			t.Fatal("provider não foi instalado depois que o collector subiu")
		}
		time.Sleep(10 * time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = desligar(ctx)
}

func TestColetorAcessivel(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if !coletorAcessivel(l.Addr().String()) {
		t.Fatal("esperado acessível com listener aberto")
	}
}

func TestEnderecoSondagem(t *testing.T) {
	casos := []struct{ endpoint, esperado string }{
		{"otel-collector:4317", "otel-collector:4317"},
		{"http://otel-collector:4318", "otel-collector:4318"},
		{"http://otel-collector/v1", "otel-collector:4318"},
		{"https://otel.deelp.com.br", "otel.deelp.com.br:443"},
		{"otel-collector", "otel-collector:4317"},
	}
	for _, caso := range casos {
		c := Config{NomeServico: "svc", Endpoint: caso.endpoint}
		e, err := c.resolveEndpoint()
		if err != nil {
			t.Fatalf("endpoint=%q: %v", caso.endpoint, err)
		}
		if got := c.enderecoSondagem(e); got != caso.esperado {
			t.Errorf("endpoint=%q: esperado %q, obtido %q", caso.endpoint, caso.esperado, got)
		}
	}
}

func TestErrorHandlerLimitado_AvisaUmaVezPorJanela(t *testing.T) {
	var buf bytes.Buffer
	h := &errorHandlerLimitado{logger: slog.New(slog.NewTextHandler(&buf, nil))}
	for range 50 {
		h.Handle(errors.New("connection refused"))
	}
	if n := strings.Count(buf.String(), "falha ao exportar"); n != 1 {
		t.Fatalf("esperado 1 aviso na janela, obtido %d", n)
	}
	if h.suprimidos != 49 {
		t.Fatalf("esperado 49 suprimidos, obtido %d", h.suprimidos)
	}
}
