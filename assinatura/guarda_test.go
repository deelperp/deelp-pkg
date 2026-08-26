package assinatura

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deelperp/deelp-pkg/auth"
)

type checkerFake struct {
	ativo          bool
	motivo         string
	erro           error
	chamado        bool
	bearerRecebido string
}

func (c *checkerFake) ContratoAtivo(_ context.Context, bearer, _ string) (bool, string, error) {
	c.chamado = true
	c.bearerRecebido = bearer
	return c.ativo, c.motivo, c.erro
}

func destinoOK() (http.Handler, *bool) {
	alcancado := false
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		alcancado = true
		w.WriteHeader(http.StatusOK)
	}), &alcancado
}

func requisicaoComTenant(metodo string) *http.Request {
	req := httptest.NewRequest(metodo, "/ordem-service/v1/pedidos", nil)
	req.Header.Set("Authorization", "Bearer tok123")
	ctx := auth.ComClaims(req.Context(), auth.Claims{
		UsuarioId: "11111111-1111-1111-1111-111111111111",
		EmpresaId: "22222222-2222-2222-2222-222222222222",
	})
	return req.WithContext(ctx)
}

func TestGuarda_RepassaBearerDoRequestAoChecker(t *testing.T) {
	destino, _ := destinoOK()
	checker := &checkerFake{ativo: true}
	rec := httptest.NewRecorder()

	RequerContratoAtivo(Config{}, checker)(destino).ServeHTTP(rec, requisicaoComTenant(http.MethodPost))

	if checker.bearerRecebido != "Bearer tok123" {
		t.Fatalf("bearer repassado ao checker = %q, esperado %q", checker.bearerRecebido, "Bearer tok123")
	}
}

func TestGuarda_ContratoAtivoPassa(t *testing.T) {
	destino, alcancado := destinoOK()
	rec := httptest.NewRecorder()

	RequerContratoAtivo(Config{}, &checkerFake{ativo: true})(destino).
		ServeHTTP(rec, requisicaoComTenant(http.MethodPost))

	if !*alcancado || rec.Code != http.StatusOK {
		t.Errorf("contrato ativo deveria passar: alcancado=%v status=%d", *alcancado, rec.Code)
	}
}

func TestGuarda_ContratoInativoDevolve402(t *testing.T) {
	destino, alcancado := destinoOK()
	rec := httptest.NewRecorder()

	RequerContratoAtivo(Config{}, &checkerFake{ativo: false, motivo: "expirado"})(destino).
		ServeHTTP(rec, requisicaoComTenant(http.MethodPost))

	if *alcancado {
		t.Error("requisição não deveria alcançar o handler com contrato inativo")
	}
	// 402 e não 403: o cliente precisa distinguir bloqueio por pagamento de
	// bloqueio por permissão para abrir a tela certa.
	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d, esperado 402", rec.Code)
	}

	var corpo Erro
	if err := json.Unmarshal(rec.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("corpo ilegível: %v", err)
	}
	if corpo.Motivo != MotivoContratoInativo {
		t.Errorf("motivo = %q, esperado %q", corpo.Motivo, MotivoContratoInativo)
	}
}

// Fail-open deliberado: cliente-service fora não pode derrubar a escrita de
// todos os clientes adimplentes.
func TestGuarda_ErroDeTransporteDeixaPassar(t *testing.T) {
	destino, alcancado := destinoOK()
	rec := httptest.NewRecorder()

	RequerContratoAtivo(Config{}, &checkerFake{erro: errors.New("cliente-service fora")})(destino).
		ServeHTTP(rec, requisicaoComTenant(http.MethodPost))

	if !*alcancado {
		t.Error("erro de transporte deve deixar passar, não bloquear a plataforma inteira")
	}
}

func TestGuarda_CheckerNaoConfiguradoDeixaPassar(t *testing.T) {
	destino, alcancado := destinoOK()
	rec := httptest.NewRecorder()

	RequerContratoAtivo(Config{}, nil)(destino).
		ServeHTTP(rec, requisicaoComTenant(http.MethodPost))

	if !*alcancado {
		t.Error("checker ausente não deve bloquear")
	}
}

// Sem claims quem barra é o auth.Autenticacao. Este middleware não deve
// inventar bloqueio para requisição que nem chegou a ter tenant.
func TestGuarda_SemTenantNaoConsulta(t *testing.T) {
	destino, alcancado := destinoOK()
	checker := &checkerFake{ativo: false}
	rec := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodPost, "/qualquer", nil)
	RequerContratoAtivo(Config{}, checker)(destino).ServeHTTP(rec, req)

	if checker.chamado {
		t.Error("não deveria consultar contrato sem tenant no contexto")
	}
	if !*alcancado {
		t.Error("requisição sem tenant deve seguir para o próximo middleware")
	}
}

// Leitura fica aberta mesmo com contrato inativo: o cliente bloqueado precisa
// ver os próprios dados e chegar até a tela de pagamento.
func TestApenasEscrita_LeituraPassaComContratoInativo(t *testing.T) {
	checker := &checkerFake{ativo: false, motivo: "expirado"}
	gate := ApenasEscrita(RequerContratoAtivo(Config{}, checker))

	for _, metodo := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		t.Run(metodo, func(t *testing.T) {
			destino, alcancado := destinoOK()
			rec := httptest.NewRecorder()

			gate(destino).ServeHTTP(rec, requisicaoComTenant(metodo))

			if !*alcancado {
				t.Errorf("%s deveria passar mesmo com contrato inativo", metodo)
			}
			if rec.Code == http.StatusPaymentRequired {
				t.Errorf("%s não deveria devolver 402", metodo)
			}
		})
	}
}

func TestApenasEscrita_EscritaBloqueia(t *testing.T) {
	gate := ApenasEscrita(RequerContratoAtivo(Config{}, &checkerFake{ativo: false, motivo: "expirado"}))

	for _, metodo := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(metodo, func(t *testing.T) {
			destino, alcancado := destinoOK()
			rec := httptest.NewRecorder()

			gate(destino).ServeHTTP(rec, requisicaoComTenant(metodo))

			if *alcancado || rec.Code != http.StatusPaymentRequired {
				t.Errorf("%s deveria devolver 402: alcancado=%v status=%d", metodo, *alcancado, rec.Code)
			}
		})
	}
}

func TestApenasEscrita_PostPesquisarPassaComContratoInativo(t *testing.T) {
	gate := ApenasEscrita(RequerContratoAtivo(Config{}, &checkerFake{ativo: false, motivo: "suspenso"}))
	destino, alcancado := destinoOK()
	rec := httptest.NewRecorder()

	gate(destino).ServeHTTP(rec, requisicaoEm(http.MethodPost, "/cliente-service/v1/clientes/pesquisar"))

	if !*alcancado {
		t.Fatal("POST /pesquisar é leitura e deve passar com contrato inativo")
	}
	if rec.Code == http.StatusPaymentRequired {
		t.Fatal("POST /pesquisar não deveria devolver 402")
	}
}

func requisicaoEm(metodo, caminho string) *http.Request {
	req := httptest.NewRequest(metodo, caminho, nil)
	req.Header.Set("Authorization", "Bearer tok123")
	ctx := auth.ComClaims(req.Context(), auth.Claims{
		UsuarioId: "11111111-1111-1111-1111-111111111111",
		EmpresaId: "22222222-2222-2222-2222-222222222222",
	})
	return req.WithContext(ctx)
}

// Sem a liberação o gate barrava o próprio caminho de pagamento, e o cliente
// bloqueado não tinha como voltar a ficar adimplente.
func TestGuarda_RotaLiberadaPassaComContratoInativo(t *testing.T) {
	destino, alcancado := destinoOK()
	checker := &checkerFake{ativo: false, motivo: SituacaoExpirado}
	cfg := Config{RotasLiberadas: []string{"/cliente-service/v1/assinatura/contratar"}}
	rec := httptest.NewRecorder()

	RequerContratoAtivo(cfg, checker)(destino).
		ServeHTTP(rec, requisicaoEm(http.MethodPost, "/cliente-service/v1/assinatura/contratar"))

	if !*alcancado {
		t.Fatal("rota de contratação deveria passar com contrato expirado")
	}
	if checker.chamado {
		t.Fatal("rota liberada não deveria consultar o checker")
	}
}

func TestGuarda_RotaNaoLiberadaSegueBloqueada(t *testing.T) {
	destino, alcancado := destinoOK()
	checker := &checkerFake{ativo: false, motivo: SituacaoExpirado}
	cfg := Config{RotasLiberadas: []string{"/cliente-service/v1/assinatura/contratar"}}
	rec := httptest.NewRecorder()

	RequerContratoAtivo(cfg, checker)(destino).
		ServeHTTP(rec, requisicaoEm(http.MethodPost, "/cliente-service/v1/clientes"))

	if *alcancado {
		t.Fatal("rota comum deveria continuar bloqueada")
	}
	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d, esperado 402", rec.Code)
	}
}

func TestGuarda_MensagemDistinguePorSituacao(t *testing.T) {
	casos := map[string]string{
		SituacaoExpirado:     "período de teste",
		SituacaoInadimplente: "cobrança em aberto",
		SituacaoSuspenso:     "suspensa",
		SituacaoCancelado:    "cancelado",
	}

	for situacao, trecho := range casos {
		destino, _ := destinoOK()
		rec := httptest.NewRecorder()

		RequerContratoAtivo(Config{}, &checkerFake{ativo: false, motivo: situacao})(destino).
			ServeHTTP(rec, requisicaoComTenant(http.MethodPost))

		var corpo Erro
		if err := json.NewDecoder(rec.Body).Decode(&corpo); err != nil {
			t.Fatalf("%s: corpo ilegível: %v", situacao, err)
		}
		if corpo.Situacao != situacao {
			t.Fatalf("%s: situacao no corpo = %q", situacao, corpo.Situacao)
		}
		if !strings.Contains(strings.ToLower(corpo.Mensagem), trecho) {
			t.Fatalf("%s: mensagem %q não menciona %q", situacao, corpo.Mensagem, trecho)
		}
	}
}
