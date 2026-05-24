package seguranca

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIPDoRequest_PreferenciaXForwardedFor_PegaApenasPrimeiroIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.10, 10.0.0.1, 10.0.0.2")
	got := IPDoRequest(req)
	if got != "203.0.113.10" {
		t.Errorf("esperado primeiro IP do XFF (203.0.113.10), obteve %q", got)
	}
}

func TestIPDoRequest_XForwardedForSemEspacoExtra(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.10")
	if got := IPDoRequest(req); got != "203.0.113.10" {
		t.Errorf("esperado 203.0.113.10, obteve %q", got)
	}
}

func TestIPDoRequest_FallbackParaXRealIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Real-IP", "203.0.113.42")
	if got := IPDoRequest(req); got != "203.0.113.42" {
		t.Errorf("esperado X-Real-IP fallback, obteve %q", got)
	}
}

func TestIPDoRequest_FallbackFinalRemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "203.0.113.99:54321"
	if got := IPDoRequest(req); got != "203.0.113.99" {
		t.Errorf("esperado 203.0.113.99 (sem porta), obteve %q", got)
	}
}

func TestIPDoRequest_RemoteAddrSemPorta(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "127.0.0.1"
	if got := IPDoRequest(req); got != "127.0.0.1" {
		t.Errorf("esperado 127.0.0.1, obteve %q", got)
	}
}
