package ttlcache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type relogio struct {
	mu    sync.Mutex
	atual time.Time
}

func (r *relogio) agora() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.atual
}

func (r *relogio) avancar(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.atual = r.atual.Add(d)
}

func novoRelogio() *relogio { return &relogio{atual: time.Unix(1_700_000_000, 0)} }

func TestCache_ExpiraERemoveEntrada(t *testing.T) {
	r := novoRelogio()
	c := New[string, int](10, r.agora)
	c.Set("a", 1, time.Minute)

	r.avancar(time.Minute)

	if _, ok := c.Get("a"); ok {
		t.Fatal("entrada expirada devolvida")
	}
	if c.Len() != 0 {
		t.Fatalf("entrada expirada retida: %d", c.Len())
	}
}

func TestCache_CapacidadeNuncaExcedida(t *testing.T) {
	r := novoRelogio()
	c := New[int, int](3, r.agora)
	for i := 0; i < 100; i++ {
		c.Set(i, i, time.Duration(i+1)*time.Second)
		if c.Len() > 3 {
			t.Fatalf("capacidade excedida: %d", c.Len())
		}
	}
	if _, ok := c.Get(99); !ok {
		t.Fatal("entrada mais recente descartada")
	}
}

func TestCache_CheioPurgaExpiradosAntesDeDescartarVigentes(t *testing.T) {
	r := novoRelogio()
	c := New[string, int](2, r.agora)
	c.Set("curta", 1, time.Second)
	c.Set("longa", 2, time.Hour)
	r.avancar(2 * time.Second)

	c.Set("nova", 3, time.Hour)

	if _, ok := c.Get("longa"); !ok {
		t.Fatal("entrada vigente descartada enquanto havia expirada")
	}
}

func TestCache_LoadColapsaChamadasConcorrentes(t *testing.T) {
	c := New[string, int](10, time.Now)
	var cargas atomic.Int32
	liberar := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := c.Load(context.Background(), "k", func(context.Context) (int, time.Duration, error) {
				cargas.Add(1)
				<-liberar
				return 7, time.Minute, nil
			})
			if err != nil || v != 7 {
				t.Errorf("Load = %d, %v", v, err)
			}
		}()
	}
	time.Sleep(20 * time.Millisecond)
	close(liberar)
	wg.Wait()
	if n := cargas.Load(); n != 1 {
		t.Fatalf("carga executada %d vezes", n)
	}
}

func TestCache_LoadNaoGuardaErroNemTTLZero(t *testing.T) {
	c := New[string, int](10, time.Now)
	falha := errors.New("remoto fora")
	if _, err := c.Load(context.Background(), "k", func(context.Context) (int, time.Duration, error) { return 0, time.Minute, falha }); !errors.Is(err, falha) {
		t.Fatalf("erro perdido: %v", err)
	}
	_, _ = c.Load(context.Background(), "k", func(context.Context) (int, time.Duration, error) { return 1, 0, nil })
	if c.Len() != 0 {
		t.Fatal("erro ou TTL zero foi guardado")
	}
}

func TestCache_LoadComPanicNaoTravaEsperantes(t *testing.T) {
	c := New[string, int](10, time.Now)
	func() {
		defer func() { _ = recover() }()
		_, _ = c.Load(context.Background(), "k", func(context.Context) (int, time.Duration, error) { panic("boom") })
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	v, err := c.Load(ctx, "k", func(context.Context) (int, time.Duration, error) { return 3, time.Minute, nil })
	if err != nil || v != 3 {
		t.Fatalf("chave travada após panic: %d, %v", v, err)
	}
}

func TestCache_EsperanteRespeitaCancelamento(t *testing.T) {
	c := New[string, int](10, time.Now)
	liberar := make(chan struct{})
	defer close(liberar)
	go func() {
		_, _ = c.Load(context.Background(), "k", func(context.Context) (int, time.Duration, error) { <-liberar; return 1, time.Minute, nil })
	}()
	time.Sleep(10 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Load(ctx, "k", func(context.Context) (int, time.Duration, error) { return 2, time.Minute, nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("esperante ignorou cancelamento: %v", err)
	}
}
