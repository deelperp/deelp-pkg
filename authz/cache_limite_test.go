package authz

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestChaveCache_NaoRetemBearer(t *testing.T) {
	bearer := "Bearer eyJhbGciOiJIUzI1NiJ9.segredo"
	chave := chaveCache("usuario-1", bearer)
	if strings.Contains(chave, "segredo") || strings.Contains(chave, "usuario-1") {
		t.Fatalf("chave expõe token ou usuário: %s", chave)
	}
	if chave == chaveCache("usuario-1", bearer+"x") {
		t.Fatal("tokens diferentes colidiram")
	}
}

func TestTemPermissao_CacheNaoPassaDaCapacidade(t *testing.T) {
	chamadas := 0
	srv := servidorPermissoes(t, &chamadas)
	defer srv.Close()
	checker := NewHTTPChecker(srv.URL)

	for i := 0; i < CapacidadeCache+50; i++ {
		if _, err := checker.TemPermissao(context.Background(), fmt.Sprintf("Bearer token-%d", i), "usuario-1", "producao", "criar"); err != nil {
			t.Fatal(err)
		}
	}

	if n := checker.cache.Len(); n > CapacidadeCache {
		t.Fatalf("cache com %d entradas, teto %d", n, CapacidadeCache)
	}
}

func TestTemPermissao_ExpiradasSaemDaMemoria(t *testing.T) {
	chamadas := 0
	srv := servidorPermissoes(t, &chamadas)
	defer srv.Close()
	checker := NewHTTPChecker(srv.URL)
	relogio := time.Now()
	checker.agora = func() time.Time { return relogio }

	_, _ = checker.TemPermissao(context.Background(), "Bearer a", "usuario-1", "producao", "criar")
	relogio = relogio.Add(TTLCache)
	_, _ = checker.TemPermissao(context.Background(), "Bearer b", "usuario-1", "producao", "criar")

	if _, ok := checker.cache.Get(chaveCache("usuario-1", "Bearer a")); ok {
		t.Fatal("entrada expirada ainda servida")
	}
}

func TestTemPermissao_RajadaConcorrenteFazUmaConsulta(t *testing.T) {
	var chamadas atomic.Int32
	liberar := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		chamadas.Add(1)
		<-liberar
		_, _ = w.Write([]byte(`{"sucesso":true,"conteudo":{"producao":["criar"]}}`))
	}))
	defer srv.Close()
	checker := NewHTTPChecker(srv.URL)

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, err := checker.TemPermissao(context.Background(), "Bearer x", "usuario-1", "producao", "criar"); err != nil || !ok {
				t.Errorf("TemPermissao = %v, %v", ok, err)
			}
		}()
	}
	time.Sleep(30 * time.Millisecond)
	close(liberar)
	wg.Wait()

	if n := chamadas.Load(); n != 1 {
		t.Fatalf("rajada fez %d consultas ao autenticacao-service", n)
	}
}
