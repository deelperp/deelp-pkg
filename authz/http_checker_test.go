package authz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func servidorPermissoes(t *testing.T, chamadas *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*chamadas++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sucesso":true,"conteudo":{"producao":["criar","atualizar"]}}`))
	}))
}

// Sem cache, cada requisição protegida fazia uma ida e volta ao
// autenticacao-service. Uma navegação disparava dezenas, estourava o rate
// limiter (que é compartilhado com /entrar) e derrubava o próprio login.
func TestTemPermissao_ReusaCacheEmVezDeRepetirHTTP(t *testing.T) {
	chamadas := 0
	srv := servidorPermissoes(t, &chamadas)
	defer srv.Close()

	checker := NewHTTPChecker(srv.URL)
	ctx := context.Background()

	for i := 0; i < 20; i++ {
		permitido, err := checker.TemPermissao(ctx, "Bearer x", "usuario-1", "producao", "criar")
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if !permitido {
			t.Fatal("esperado permitido")
		}
	}

	if chamadas != 1 {
		t.Errorf("fez %d chamadas HTTP, esperado 1 (as demais vêm do cache)", chamadas)
	}
}

// O cache guarda o mapa inteiro de permissões, não a resposta de um par
// módulo:ação — consultar outra ação não pode gerar nova ida à rede.
func TestTemPermissao_CacheServeOutrosModulosEAcoes(t *testing.T) {
	chamadas := 0
	srv := servidorPermissoes(t, &chamadas)
	defer srv.Close()

	checker := NewHTTPChecker(srv.URL)
	ctx := context.Background()

	if permitido, _ := checker.TemPermissao(ctx, "", "usuario-1", "producao", "criar"); !permitido {
		t.Fatal("producao:criar deveria ser permitido")
	}
	if permitido, _ := checker.TemPermissao(ctx, "", "usuario-1", "producao", "excluir"); permitido {
		t.Error("producao:excluir não está na resposta, deveria ser negado")
	}
	if permitido, _ := checker.TemPermissao(ctx, "", "usuario-1", "fiscal", "criar"); permitido {
		t.Error("módulo ausente deveria ser negado")
	}

	if chamadas != 1 {
		t.Errorf("fez %d chamadas HTTP, esperado 1", chamadas)
	}
}

// Cache é por usuário: um não pode herdar as permissões do outro.
func TestTemPermissao_CachePorUsuario(t *testing.T) {
	chamadas := 0
	srv := servidorPermissoes(t, &chamadas)
	defer srv.Close()

	checker := NewHTTPChecker(srv.URL)
	ctx := context.Background()

	_, _ = checker.TemPermissao(ctx, "", "usuario-1", "producao", "criar")
	_, _ = checker.TemPermissao(ctx, "", "usuario-2", "producao", "criar")

	if chamadas != 2 {
		t.Errorf("fez %d chamadas, esperado 2 (uma por usuário)", chamadas)
	}
}

func TestTemPermissao_ConsultaDeNovoAposExpirar(t *testing.T) {
	chamadas := 0
	srv := servidorPermissoes(t, &chamadas)
	defer srv.Close()

	relogio := time.Now()
	checker := NewHTTPChecker(srv.URL)
	checker.agora = func() time.Time { return relogio }
	ctx := context.Background()

	_, _ = checker.TemPermissao(ctx, "", "usuario-1", "producao", "criar")
	relogio = relogio.Add(TTLCache + time.Second)
	_, _ = checker.TemPermissao(ctx, "", "usuario-1", "producao", "criar")

	if chamadas != 2 {
		t.Errorf("fez %d chamadas, esperado 2 (cache expirou)", chamadas)
	}
}

// Revogação de cargo precisa surtir efeito sem esperar o TTL.
func TestInvalidar_ForcaNovaConsulta(t *testing.T) {
	chamadas := 0
	srv := servidorPermissoes(t, &chamadas)
	defer srv.Close()

	checker := NewHTTPChecker(srv.URL)
	ctx := context.Background()

	_, _ = checker.TemPermissao(ctx, "", "usuario-1", "producao", "criar")
	checker.Invalidar("usuario-1")
	_, _ = checker.TemPermissao(ctx, "", "usuario-1", "producao", "criar")

	if chamadas != 2 {
		t.Errorf("fez %d chamadas, esperado 2 após invalidar", chamadas)
	}
}

// Erro não pode ser cacheado: gravar "sem permissão" por 2 minutos negaria
// acesso legítimo a partir de uma falha transitória.
func TestTemPermissao_ErroNaoEhCacheado(t *testing.T) {
	chamadas := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		chamadas++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	checker := NewHTTPChecker(srv.URL)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if _, err := checker.TemPermissao(ctx, "", "usuario-1", "producao", "criar"); err == nil {
			t.Fatal("esperado erro propagado para o middleware tratar fail-closed")
		}
	}

	if chamadas != 3 {
		t.Errorf("fez %d chamadas, esperado 3 — erro não deve virar cache", chamadas)
	}
}
