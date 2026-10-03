package s3

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type s3Falso struct {
	mu          sync.Mutex
	requisicoes []string
	corpoCORS   string
	status      int
}

func (f *s3Falso) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requisicoes = append(f.requisicoes, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
	if _, ok := r.URL.Query()["cors"]; ok {
		corpo, _ := io.ReadAll(r.Body)
		f.corpoCORS = string(corpo)
	}
	if f.status != 0 {
		w.WriteHeader(f.status)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (f *s3Falso) total() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requisicoes)
}

func configFalsa(endpoint string) Config {
	return Config{Region: "us-east-1", Bucket: "deelp", AccessKey: "AKIATESTE", SecretKey: "segredo", Endpoint: endpoint, CORSOrigin: "https://app.deelp.com.br"}
}

func TestOpen_NaoAlteraOBucket(t *testing.T) {
	falso := &s3Falso{}
	srv := httptest.NewServer(falso)
	defer srv.Close()

	if _, err := Open(context.Background(), configFalsa(srv.URL)); err != nil {
		t.Fatal(err)
	}

	if n := falso.total(); n != 0 {
		t.Fatalf("Open fez %d chamadas ao bucket", n)
	}
}

func TestConfigurarCORS_EnviaTodasAsOrigens(t *testing.T) {
	falso := &s3Falso{}
	srv := httptest.NewServer(falso)
	defer srv.Close()
	cli, err := Open(context.Background(), configFalsa(srv.URL))
	if err != nil {
		t.Fatal(err)
	}

	if err := cli.ConfigurarCORS(context.Background(), "https://app.deelp.com.br", " https://admin.deelp.com.br "); err != nil {
		t.Fatal(err)
	}

	for _, origem := range []string{"https://app.deelp.com.br", "https://admin.deelp.com.br"} {
		if !strings.Contains(falso.corpoCORS, "<AllowedOrigin>"+origem+"</AllowedOrigin>") {
			t.Fatalf("origem %s ausente da política: %s", origem, falso.corpoCORS)
		}
	}
}

func TestConfigurarCORS_DevolveFalhaDoBucket(t *testing.T) {
	falso := &s3Falso{status: http.StatusForbidden}
	srv := httptest.NewServer(falso)
	defer srv.Close()
	cli, err := Open(context.Background(), configFalsa(srv.URL))
	if err != nil {
		t.Fatal(err)
	}

	if err := cli.ConfigurarCORS(context.Background(), "https://app.deelp.com.br"); err == nil {
		t.Fatal("falha de provisionamento escondida")
	}
}

func TestConfigurarCORS_RecusaOrigemVaziaOuCuringa(t *testing.T) {
	falso := &s3Falso{}
	srv := httptest.NewServer(falso)
	defer srv.Close()
	cli, err := Open(context.Background(), configFalsa(srv.URL))
	if err != nil {
		t.Fatal(err)
	}

	for _, origens := range [][]string{nil, {" "}, {"https://app.deelp.com.br", "*"}} {
		if err := cli.ConfigurarCORS(context.Background(), origens...); err == nil {
			t.Fatalf("origens %q aceitas", origens)
		}
	}
	if n := falso.total(); n != 0 {
		t.Fatalf("configuração recusada chegou ao bucket (%d chamadas)", n)
	}
}

func TestNewCliente_MantemComportamentoLegado(t *testing.T) {
	falso := &s3Falso{}
	srv := httptest.NewServer(falso)
	defer srv.Close()

	if _, err := NewCliente(configFalsa(srv.URL)); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(falso.corpoCORS, "https://app.deelp.com.br") {
		t.Fatal("fachada legada deixou de aplicar CORSOrigin")
	}
}
