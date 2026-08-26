package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEhRequisicaoDeLeitura_MetodosSeguros(t *testing.T) {
	for _, metodo := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		req := httptest.NewRequest(metodo, "/qualquer", nil)
		if !EhRequisicaoDeLeitura(req) {
			t.Errorf("%s deveria ser leitura", metodo)
		}
	}
}

func TestEhRequisicaoDeLeitura_EscritaNuncaPassa(t *testing.T) {
	for _, metodo := range []string{http.MethodPut, http.MethodPatch, http.MethodDelete} {
		req := httptest.NewRequest(metodo, "/cliente-service/v1/clientes/pesquisar", nil)
		if EhRequisicaoDeLeitura(req) {
			t.Errorf("%s nunca é leitura, mesmo com sufixo de busca", metodo)
		}
	}
}

func TestEhRequisicaoDeLeitura_PostPesquisarESalvarNaoAbre(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/cliente-service/v1/clientes/pesquisar-e-salvar", nil)
	if EhRequisicaoDeLeitura(req) {
		t.Fatal("allowlist é por sufixo exato; caminho que só contém a palavra não pode abrir")
	}
}

func TestEhRequisicaoDeLeitura_PostDeEscritaFicaBloqueado(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/cliente-service/v1/clientes", nil)
	if EhRequisicaoDeLeitura(req) {
		t.Fatal("POST de criação não é leitura")
	}
}

func TestEhRequisicaoDeLeitura_CaminhosReaisDeLeitura(t *testing.T) {
	caminhos := []string{
		"/cliente-service/v1/clientes/pesquisar",
		"/cliente-service/v1/empresas/pesquisar",
		"/cliente-service/v1/cargos/pesquisar",
		"/cliente-service/v1/fornecedores/pesquisar",
		"/cliente-service/v1/veiculos/pesquisar",
		"/cliente-service/v1/condutores/pesquisar",
		"/cliente-service/v1/departamentos/pesquisar",
		"/cliente-service/v1/transportadoras/pesquisar",
		"/cliente-service/v1/plataforma/contas/pesquisar",
		"/autenticacao-service/v1/colaboracoes/pesquisar",
		"/autenticacao-service/v1/plataforma/usuarios/pesquisar",
		"/estoque-service/v1/materiais/pesquisar",
		"/estoque-service/v1/produtos/pesquisar",
		"/estoque-service/v1/componentes/pesquisar",
		"/estoque-service/v1/categorias/pesquisar",
		"/estoque-service/v1/precos/pesquisar",
		"/financeiro-service/v1/orcamentos-compra/pesquisar",
		"/cliente-service/v1/clientes/consultar",
		"/cliente-service/v1/cargos/consultar",
		"/cliente-service/v1/cidades/consultar",
		"/cliente-service/v1/estados/consultar",
		"/cliente-service/v1/fornecedores/consultar",
		"/cliente-service/v1/veiculos/consultar",
		"/cliente-service/v1/condutores/consultar",
		"/cliente-service/v1/departamentos/consultar",
		"/cliente-service/v1/transportadoras/consultar",
		"/autenticacao-service/v1/usuarios/consultar",
		"/estoque-service/v1/materiais/consultar",
		"/estoque-service/v1/produtos/consultar",
		"/estoque-service/v1/componentes/consultar",
		"/estoque-service/v1/categorias/consultar",
		"/estoque-service/v1/cfops/consultar",
		"/estoque-service/v1/ncms/consultar",
		"/estoque-service/v1/cest/consultar",
		"/estoque-service/v1/cst/consultar",
		"/estoque-service/v1/cclasstrib/consultar",
		"/financeiro-service/v1/modelos-fiscais/consultar",
		"/nfe-service/v1/nfe/listar",
		"/mdfe-service/v1/mdfe/listar",
		"/tarefa-service/v1/tarefas/listar",
		"/financeiro-service/v1/dashboard/metricas",
		"/financeiro-service/v1/dashboard/metricas-integradas",
		"/nfe-service/v1/nfe/previa-calculo",
		"/relatorio-service/v1/relatorios/vendas",
		"/relatorio-service/v1/relatorios/nfe/danfe",
		"/relatorio-service/v1/relatorios/mdfe/damdfe",
		"/relatorio-service/v1/relatorios/orcamentos/abc/proposta",
		"/relatorio-service/v1/relatorios/orcamentos-compra/abc/comparativo",
		"/relatorio-service/v1/relatorios/ordens/abc/pdf",
		"/relatorio-service/v1/relatorios/solicitar",
		"/financeiro-service/v1/orcamentos-compra/gerar-pdf-solicitacao",
		"/notificacao-service/v1/",
	}

	for _, caminho := range caminhos {
		req := httptest.NewRequest(http.MethodPost, caminho, nil)
		if !EhRequisicaoDeLeitura(req) {
			t.Errorf("POST %s deveria ser leitura", caminho)
		}
	}
}
