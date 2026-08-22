package assinatura

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type respostaInterna struct {
	Sucesso  bool `json:"sucesso"`
	Conteudo struct {
		ContratoAtivo    bool   `json:"contratoAtivo"`
		ContratoSituacao string `json:"contratoSituacao"`
	} `json:"conteudo"`
}

func servidorAssinatura(t *testing.T, bearerEsperado string, ativo bool, situacao string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != bearerEsperado {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{"sucesso": false})
			return
		}
		w.WriteHeader(status)
		resp := respostaInterna{Sucesso: true}
		resp.Conteudo.ContratoAtivo = ativo
		resp.Conteudo.ContratoSituacao = situacao
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func TestHTTPChecker_BearerErrado_DevolveErro(t *testing.T) {
	srv := servidorAssinatura(t, "Bearer certo", true, "ativo", http.StatusOK)
	defer srv.Close()

	checker := NewHTTPChecker(srv.URL)
	_, _, err := checker.ContratoAtivo(t.Context(), "Bearer errado", "empresa-1")
	if err == nil {
		t.Fatal("esperava erro quando o bearer repassado não bate")
	}
}

func TestHTTPChecker_BearerCerto_ContratoAtivo(t *testing.T) {
	srv := servidorAssinatura(t, "Bearer certo", true, "ativo", http.StatusOK)
	defer srv.Close()

	checker := NewHTTPChecker(srv.URL)
	ativo, motivo, err := checker.ContratoAtivo(t.Context(), "Bearer certo", "empresa-1")
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if !ativo {
		t.Fatal("esperava contrato ativo")
	}
	if motivo != "ativo" {
		t.Fatalf("motivo = %q, esperava %q", motivo, "ativo")
	}
}

func TestHTTPChecker_BearerCerto_ContratoInativo(t *testing.T) {
	srv := servidorAssinatura(t, "Bearer certo", false, "expirado", http.StatusOK)
	defer srv.Close()

	checker := NewHTTPChecker(srv.URL)
	ativo, motivo, err := checker.ContratoAtivo(t.Context(), "Bearer certo", "empresa-1")
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if ativo {
		t.Fatal("esperava contrato inativo")
	}
	if motivo != "expirado" {
		t.Fatalf("motivo = %q, esperava %q", motivo, "expirado")
	}
}

func TestHTTPChecker_StatusNaoOK_DevolveErro(t *testing.T) {
	srv := servidorAssinatura(t, "Bearer certo", true, "ativo", http.StatusInternalServerError)
	defer srv.Close()

	checker := NewHTTPChecker(srv.URL)
	_, _, err := checker.ContratoAtivo(t.Context(), "Bearer certo", "empresa-1")
	if err == nil {
		t.Fatal("esperava erro em status HTTP diferente de 200")
	}
}

func TestHTTPChecker_SemBaseURL_DevolveErro(t *testing.T) {
	checker := NewHTTPChecker("")
	_, _, err := checker.ContratoAtivo(t.Context(), "Bearer certo", "empresa-1")
	if err == nil {
		t.Fatal("esperava erro quando baseURL não configurada")
	}
}

func TestHTTPChecker_SemBearer_DevolveErro(t *testing.T) {
	srv := servidorAssinatura(t, "Bearer certo", true, "ativo", http.StatusOK)
	defer srv.Close()

	checker := NewHTTPChecker(srv.URL)
	_, _, err := checker.ContratoAtivo(t.Context(), "", "empresa-1")
	if err == nil {
		t.Fatal("esperava erro quando não há bearer para repassar")
	}
}

func TestHTTPChecker_CacheEvitaSegundaChamadaDentroDoTTL(t *testing.T) {
	chamadas := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chamadas++
		w.WriteHeader(http.StatusOK)
		resp := respostaInterna{Sucesso: true}
		resp.Conteudo.ContratoAtivo = true
		resp.Conteudo.ContratoSituacao = "ativo"
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	checker := NewHTTPChecker(srv.URL)
	agora := time.Now()
	checker.agora = func() time.Time { return agora }

	if _, _, err := checker.ContratoAtivo(t.Context(), "Bearer x", "empresa-1"); err != nil {
		t.Fatalf("primeira chamada: erro inesperado: %v", err)
	}
	if _, _, err := checker.ContratoAtivo(t.Context(), "Bearer x", "empresa-1"); err != nil {
		t.Fatalf("segunda chamada (deve vir do cache): erro inesperado: %v", err)
	}
	if chamadas != 1 {
		t.Fatalf("esperava 1 chamada HTTP (cache servindo a segunda), teve %d", chamadas)
	}

	agora = agora.Add(TTLCache + time.Second)
	if _, _, err := checker.ContratoAtivo(t.Context(), "Bearer x", "empresa-1"); err != nil {
		t.Fatalf("terceira chamada (TTL expirado): erro inesperado: %v", err)
	}
	if chamadas != 2 {
		t.Fatalf("esperava 2 chamadas HTTP após expirar o TTL, teve %d", chamadas)
	}
}

func TestHTTPChecker_Invalidar_ForcaNovaConsulta(t *testing.T) {
	chamadas := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chamadas++
		w.WriteHeader(http.StatusOK)
		resp := respostaInterna{Sucesso: true}
		resp.Conteudo.ContratoAtivo = true
		resp.Conteudo.ContratoSituacao = "ativo"
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	checker := NewHTTPChecker(srv.URL)
	if _, _, err := checker.ContratoAtivo(t.Context(), "Bearer x", "empresa-1"); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	checker.Invalidar("empresa-1")
	if _, _, err := checker.ContratoAtivo(t.Context(), "Bearer x", "empresa-1"); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if chamadas != 2 {
		t.Fatalf("esperava 2 chamadas HTTP após invalidar o cache, teve %d", chamadas)
	}
}

func TestHTTPChecker_CachePorEmpresa(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		resp := respostaInterna{Sucesso: true}
		resp.Conteudo.ContratoAtivo = r.Header.Get("Authorization") == "Bearer empresa-1"
		resp.Conteudo.ContratoSituacao = "x"
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	checker := NewHTTPChecker(srv.URL)
	ativo1, _, _ := checker.ContratoAtivo(t.Context(), "Bearer empresa-1", "empresa-1")
	ativo2, _, _ := checker.ContratoAtivo(t.Context(), "Bearer empresa-2", "empresa-2")
	if !ativo1 {
		t.Fatal("empresa-1 deveria estar ativa")
	}
	if ativo2 {
		t.Fatal("empresa-2 deveria estar inativa")
	}
}

func TestChecker_BloqueioExpiraAntesDoLiberado(t *testing.T) {
	if TTLCacheBloqueado >= TTLCache {
		t.Fatal("TTL de bloqueio deve ser menor que o de liberado")
	}
}
