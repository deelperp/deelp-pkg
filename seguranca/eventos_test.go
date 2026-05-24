package seguranca

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func capturarSlog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(original)
	fn()
	return buf.String()
}

func TestEmitirEvento_LogaCamposPadronizados(t *testing.T) {
	out := capturarSlog(t, func() {
		EmitirEvento(context.Background(), EventoSeg{
			Evento:    "login_sucesso",
			UsuarioID: "user-1",
			EmpresaID: "emp-1",
			IP:        "203.0.113.5",
			Detalhes:  map[string]any{"motivo": "ok"},
		})
	})

	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &m); err != nil {
		t.Fatalf("log nao eh JSON valido: %v\n%s", err, out)
	}
	if m["msg"] != "seguranca" {
		t.Errorf("msg=%v, esperado 'seguranca'", m["msg"])
	}
	if m["evento"] != "login_sucesso" {
		t.Errorf("evento=%v", m["evento"])
	}
	if m["usuario_id"] != "user-1" {
		t.Errorf("usuario_id=%v", m["usuario_id"])
	}
	if m["empresa_id"] != "emp-1" {
		t.Errorf("empresa_id=%v", m["empresa_id"])
	}
	if m["motivo"] != "ok" {
		t.Errorf("motivo=%v", m["motivo"])
	}
}

func TestEmitirEventoRequest_PreencheCamposFromRequest(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.RemoteAddr = "203.0.113.10:5000"
	r.Header.Set("User-Agent", "TesteAgent/1.0")

	out := capturarSlog(t, func() {
		EmitirEventoRequest(r, EventoSeg{Evento: "login_attempt"})
	})

	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &m); err != nil {
		t.Fatalf("log nao eh JSON valido: %v\n%s", err, out)
	}
	if m["evento"] != "login_attempt" {
		t.Errorf("evento=%v", m["evento"])
	}
	if m["ip"] != "203.0.113.10" {
		t.Errorf("ip=%v", m["ip"])
	}
	if m["user_agent"] != "TesteAgent/1.0" {
		t.Errorf("user_agent=%v", m["user_agent"])
	}
	if m["path"] != "/x" {
		t.Errorf("path=%v", m["path"])
	}
	if m["metodo"] != http.MethodPost {
		t.Errorf("metodo=%v", m["metodo"])
	}
}
