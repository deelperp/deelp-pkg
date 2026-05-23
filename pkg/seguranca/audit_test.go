package seguranca

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityAudit_DetectarSuspeito_PadroesDefault(t *testing.T) {
	a := NewSecurityAudit(AuditConfig{})
	casos := []struct {
		path     string
		suspeito bool
	}{
		{"/api/produtos", false},
		{"/api/../etc/passwd", true},
		{"/api/produtos?nome=<script>", true},
		{"/api?q=union+select+*", true},
		{"/api?q='%3B+drop+table", true},
		{"/api?fn=exec(rm+-rf)", true},
		{"/api?code=eval(123)", true},
	}
	for _, c := range casos {
		req := httptest.NewRequest(http.MethodGet, c.path, nil)
		got, _ := a.DetectarSuspeito(req)
		if got != c.suspeito {
			t.Errorf("path=%q: esperado %v, obtive %v", c.path, c.suspeito, got)
		}
	}
}

func TestSecurityAudit_Middleware_BloquearReqFalso_DeixaPassar(t *testing.T) {
	a := NewSecurityAudit(AuditConfig{BloquearReq: false})
	chamado := false
	h := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		chamado = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/../etc/passwd", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !chamado {
		t.Fatal("handler nao foi chamado mesmo com BloquearReq=false")
	}
}

func TestSecurityAudit_Middleware_BloquearReqTrue_Bloqueia(t *testing.T) {
	a := NewSecurityAudit(AuditConfig{BloquearReq: true})
	chamado := false
	h := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		chamado = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/../etc/passwd", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if chamado {
		t.Fatal("handler foi chamado mesmo com BloquearReq=true e padrao suspeito")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperado 400, obtive %d", rec.Code)
	}
}

func TestSecurityAudit_OnEvent_RecebeChamada(t *testing.T) {
	var recebido Evento
	chamadas := 0
	a := NewSecurityAudit(AuditConfig{
		OnEvent: func(e Evento) {
			recebido = e
			chamadas++
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	a.Registrar(req, "teste", "user-1", map[string]any{"info": "extra"})
	if chamadas != 1 {
		t.Fatalf("OnEvent chamadas=%d, esperado 1", chamadas)
	}
	if recebido.Tipo != "teste" || recebido.UsuarioID != "user-1" {
		t.Errorf("evento incorreto: %+v", recebido)
	}
}

func TestSecurityAudit_PadroesCustom(t *testing.T) {
	a := NewSecurityAudit(AuditConfig{Padroes: []string{"meu-padrao-secreto"}})
	req := httptest.NewRequest(http.MethodGet, "/x?q=meu-padrao-secreto", nil)
	susp, pad := a.DetectarSuspeito(req)
	if !susp {
		t.Fatal("padrao custom nao detectado")
	}
	if pad != "meu-padrao-secreto" {
		t.Errorf("padrao retornado errado: %q", pad)
	}
	// E NAO detecta os padroes default (foram substituidos)
	req2 := httptest.NewRequest(http.MethodGet, "/x?q=union+select", nil)
	if susp2, _ := a.DetectarSuspeito(req2); susp2 {
		t.Fatal("Padroes custom deve substituir os defaults")
	}
}
