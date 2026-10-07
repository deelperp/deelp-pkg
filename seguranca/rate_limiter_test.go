package seguranca

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRateLimiter_Redis_NaoProlongaJanela(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	rl := NewRateLimiter(RateLimiterConfig{Limite: 2, Janela: time.Minute, Redis: client})
	ctx := context.Background()
	verificar := func(esperado bool) {
		t.Helper()
		ok, err := rl.Allow(ctx, "ip-1")
		if err != nil || ok != esperado {
			t.Fatalf("Allow = %v, %v; esperado %v", ok, err, esperado)
		}
	}
	verificar(true)
	server.FastForward(30 * time.Second)
	verificar(true)
	verificar(false)
	server.FastForward(31 * time.Second)
	verificar(true)
}

func TestRateLimiter_Redis_CompartilhaLimiteEntreReplicasConcorrentes(t *testing.T) {
	server := miniredis.RunT(t)
	limitadores := make([]*RateLimiter, 2)
	for i := range limitadores {
		client := redis.NewClient(&redis.Options{Addr: server.Addr()})
		t.Cleanup(func() { _ = client.Close() })
		limitadores[i] = NewRateLimiter(RateLimiterConfig{Limite: 20, Janela: time.Minute, Redis: client})
	}
	var permitidas atomic.Int32
	var grupo sync.WaitGroup
	for i := range 50 {
		grupo.Add(1)
		go func() {
			defer grupo.Done()
			ok, err := limitadores[i%2].Allow(context.Background(), "ip-compartilhado")
			if err != nil {
				t.Errorf("Allow: %v", err)
			}
			if ok {
				permitidas.Add(1)
			}
		}()
	}
	grupo.Wait()
	if total := permitidas.Load(); total != 20 {
		t.Fatalf("permitidas = %d, esperado 20 entre ambas as réplicas", total)
	}
	if ok, err := limitadores[0].Allow(context.Background(), "outro-ip"); err != nil || !ok {
		t.Fatalf("outro IP deve ter limite próprio: %v, %v", ok, err)
	}
}

func TestRateLimiter_Middleware_RetryAfterEmSegundos(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{Limite: 1, Janela: 1500 * time.Millisecond})
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "1.2.3.4:1000"
	handler.ServeHTTP(httptest.NewRecorder(), req)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "2" {
		t.Fatalf("status = %d, Retry-After = %q, esperado 429 e 2 segundos", rec.Code, rec.Header().Get("Retry-After"))
	}
}

func TestRateLimiter_InMemory_RespeitaLimite(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		Limite: 3,
		Janela: time.Minute,
	})
	ctx := context.Background()

	for i := 1; i <= 3; i++ {
		ok, err := rl.Allow(ctx, "ip-1")
		if err != nil {
			t.Fatalf("req %d: erro inesperado %v", i, err)
		}
		if !ok {
			t.Fatalf("req %d: deveria ser permitida", i)
		}
	}

	ok, _ := rl.Allow(ctx, "ip-1")
	if ok {
		t.Fatal("4a req deveria ser bloqueada")
	}
}

func TestRateLimiter_InMemory_IPsIndependentes(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{Limite: 1, Janela: time.Minute})
	ctx := context.Background()

	if ok, _ := rl.Allow(ctx, "ip-A"); !ok {
		t.Fatal("ip-A primeira req deveria passar")
	}
	if ok, _ := rl.Allow(ctx, "ip-A"); ok {
		t.Fatal("ip-A segunda deveria ser bloqueada")
	}
	if ok, _ := rl.Allow(ctx, "ip-B"); !ok {
		t.Fatal("ip-B primeira deveria passar (limite por IP)")
	}
}

func TestRateLimiter_Middleware_Retorna429(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{Limite: 1, Janela: time.Minute})
	h := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	doReq := func() int {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = "1.2.3.4:1000"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := doReq(); code != http.StatusOK {
		t.Fatalf("1a req: esperado 200, obtive %d", code)
	}
	if code := doReq(); code != http.StatusTooManyRequests {
		t.Fatalf("2a req: esperado 429, obtive %d", code)
	}
}

func TestRateLimiter_ResponderCustom_EhUsado(t *testing.T) {
	chamou := 0
	custom := func(w http.ResponseWriter, status int, msg string) {
		chamou++
		w.WriteHeader(status)
	}
	rl := NewRateLimiter(RateLimiterConfig{
		Limite:    1,
		Janela:    time.Minute,
		Responder: custom,
	})
	h := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = "5.5.5.5:1234"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
	}
	if chamou != 1 {
		t.Fatalf("responder custom deveria ter sido chamado 1 vez no bloqueio, foi %d", chamou)
	}
}

func TestIPBlocker_BloqueiaAposMaxTentativas(t *testing.T) {
	b := NewIPBlocker(IPBlockerConfig{
		MaxTentativas:    3,
		DuracaoBloqueio:  time.Minute,
		JanelaTentativas: time.Minute,
	})

	if b.EstaBloqueado("x") {
		t.Fatal("nao deveria estar bloqueado antes de falhas")
	}
	b.RegistrarFalha("x")
	b.RegistrarFalha("x")
	if b.EstaBloqueado("x") {
		t.Fatal("nao deveria estar bloqueado com 2 falhas")
	}
	b.RegistrarFalha("x")
	if !b.EstaBloqueado("x") {
		t.Fatal("deveria estar bloqueado com 3 falhas")
	}
}

func TestIPBlocker_RegistrarSucessoLimpaContador(t *testing.T) {
	b := NewIPBlocker(IPBlockerConfig{
		MaxTentativas:    3,
		DuracaoBloqueio:  time.Minute,
		JanelaTentativas: time.Minute,
	})
	b.RegistrarFalha("x")
	b.RegistrarFalha("x")
	b.RegistrarSucesso("x")
	b.RegistrarFalha("x")
	if b.EstaBloqueado("x") {
		t.Fatal("contador deveria ter sido zerado pelo sucesso")
	}
}

func TestIPBlocker_Middleware_Retorna403QuandoBloqueado(t *testing.T) {
	b := NewIPBlocker(IPBlockerConfig{
		MaxTentativas:    1,
		DuracaoBloqueio:  time.Minute,
		JanelaTentativas: time.Minute,
	})
	b.RegistrarFalha("1.2.3.4")

	h := b.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("esperado 403, obtive %d", rec.Code)
	}
}
