package tenantdados

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deelperp/deelp-pkg/internalauth"
	"github.com/google/uuid"
)

func fonte(nome string, fase int, apagadas *[]string, falha error) FonteFunc {
	return FonteFunc{
		NomeFonte:   nome,
		FaseExpurgo: fase,
		ExportarFunc: func(context.Context, uuid.UUID) ([]Arquivo, error) {
			return []Arquivo{{Nome: nome + ".csv", Conteudo: []byte(nome)}}, nil
		},
		ContarFunc: func(context.Context, uuid.UUID) (Contagem, error) {
			return Contagem{Fonte: nome, Quantidade: 2}, nil
		},
		ApagarFunc: func(context.Context, uuid.UUID) (Contagem, error) {
			if falha != nil {
				return Contagem{}, falha
			}
			*apagadas = append(*apagadas, nome)
			return Contagem{Fonte: nome, Quantidade: 2}, nil
		},
	}
}

func TestExpurgar_RespeitaFaseEOrdem(t *testing.T) {
	var apagadas []string
	s := Servico{Nome: "x", Fontes: []Fonte{
		fonte("itens", FaseMovimento, &apagadas, nil),
		fonte("material", FaseCadastro, &apagadas, nil),
		fonte("pedido", FaseMovimento, &apagadas, nil),
	}}
	r, err := s.Expurgar(context.Background(), uuid.New(), FaseMovimento, false)
	if err != nil || len(r.Itens) != 2 || strings.Join(apagadas, ",") != "itens,pedido" {
		t.Fatalf("fase 1 deve apagar só movimento na ordem declarada: %v %v %+v", err, apagadas, r)
	}
}

func TestExpurgar_SimularNaoApaga(t *testing.T) {
	var apagadas []string
	s := Servico{Fontes: []Fonte{fonte("a", FaseMovimento, &apagadas, nil)}}
	r, err := s.Expurgar(context.Background(), uuid.New(), FaseMovimento, true)
	if err != nil || len(apagadas) != 0 || r.Itens[0].Quantidade != 2 || !r.Simulado {
		t.Fatalf("simulação só conta: %v %v %+v", err, apagadas, r)
	}
}

func TestExpurgar_ParaNoPrimeiroErro(t *testing.T) {
	var apagadas []string
	s := Servico{Fontes: []Fonte{
		fonte("a", FaseMovimento, &apagadas, nil),
		fonte("b", FaseMovimento, &apagadas, errors.New("fk")),
		fonte("c", FaseMovimento, &apagadas, nil),
	}}
	_, err := s.Expurgar(context.Background(), uuid.New(), FaseMovimento, false)
	if err == nil || strings.Join(apagadas, ",") != "a" {
		t.Fatalf("deve parar em b: %v %v", err, apagadas)
	}
	if _, err := s.Expurgar(context.Background(), uuid.New(), 3, true); !errors.Is(err, ErrFaseInvalida) {
		t.Fatalf("fase inválida: %v", err)
	}
}

func TestMontarCSV(t *testing.T) {
	data := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	csv, err := montarCSV([]string{"nome", "valor", "quando", "nulo"}, [][]any{{"Ação; teste", 10.5, data, nil}})
	if err != nil {
		t.Fatal(err)
	}
	texto := string(csv)
	if !strings.HasPrefix(texto, string(bomUTF8)) || !strings.Contains(texto, `"Ação; teste";10.5;2026-10-01T12:00:00Z;`) {
		t.Fatalf("CSV inesperado: %q", texto)
	}
}

func servidor(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var apagadas []string
	s := Servico{Nome: "estoque", Fontes: []Fonte{fonte("a", FaseMovimento, &apagadas, nil)}}
	v := internalauth.NewVerificador("chave")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /estoque-service/v1"+CaminhoExportacao, s.HandlerExportacao(v))
	mux.HandleFunc("POST /estoque-service/v1"+CaminhoExpurgo, s.HandlerExpurgo(v))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &apagadas
}

func TestHandler_SemChaveRecusa(t *testing.T) {
	srv, _ := servidor(t)
	c := NovoCliente("estoque", srv.URL, "/estoque-service/v1", "errada")
	if _, err := c.Exportar(context.Background(), uuid.New()); err == nil {
		t.Fatal("chave errada deve ser recusada")
	}
}

func TestCliente_ExportarEExpurgar(t *testing.T) {
	srv, apagadas := servidor(t)
	c := NovoCliente("estoque", srv.URL, "/estoque-service/v1", "chave")
	empresa := uuid.New()

	conteudo, err := c.Exportar(context.Background(), empresa)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(conteudo), int64(len(conteudo)))
	if err != nil || len(zr.File) != 1 || zr.File[0].Name != "a.csv" {
		t.Fatalf("zip inesperado: %v", err)
	}

	r, err := c.Expurgar(context.Background(), empresa, FaseMovimento, true)
	if err != nil || len(*apagadas) != 0 || !r.Simulado || r.Servico != "estoque" {
		t.Fatalf("simulação: %v %+v", err, r)
	}
	if _, err := c.Expurgar(context.Background(), empresa, FaseMovimento, false); err != nil || len(*apagadas) != 1 {
		t.Fatalf("expurgo real: %v %v", err, *apagadas)
	}
}

func TestHandlerExpurgo_PadraoEhSimular(t *testing.T) {
	srv, apagadas := servidor(t)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/estoque-service/v1"+CaminhoExpurgo+"?fase=1", nil)
	req.Header.Set(internalauth.Header, "chave")
	req.Header.Set(HeaderEmpresa, uuid.New().String())
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK || len(*apagadas) != 0 {
		t.Fatalf("sem simular=false nada pode ser apagado: %v %v", err, *apagadas)
	}
}

type s3Falso struct {
	objetos map[string][]byte
}

func (s *s3Falso) ListarPrefixo(_ context.Context, prefixo string) ([]string, error) {
	var chaves []string
	for k := range s.objetos {
		if strings.HasPrefix(k, prefixo) {
			chaves = append(chaves, k)
		}
	}
	return chaves, nil
}
func (s *s3Falso) Download(_ context.Context, k string) ([]byte, error) { return s.objetos[k], nil }
func (s *s3Falso) Excluir(_ context.Context, k string) error {
	delete(s.objetos, k)
	return nil
}

func TestPrefixoS3_NaoVazaParaEmpresaComPrefixoParecido(t *testing.T) {
	empresa := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	outra := "documentos/empresa/" + empresa.String() + "0/x.pdf"
	s3 := &s3Falso{objetos: map[string][]byte{
		"documentos/empresa/" + empresa.String() + "/a.pdf": []byte("a"),
		outra: []byte("b"),
	}}
	fonte := PrefixoS3{S3: s3, NomeFonte: "s3", PastaExportada: "anexos",
		Prefixo: func(id uuid.UUID) string { return "documentos/empresa/" + id.String() }}

	arquivos, err := fonte.Exportar(context.Background(), empresa)
	if err != nil || len(arquivos) != 1 || arquivos[0].Nome != "anexos/a.pdf" {
		t.Fatalf("exportação: %v %+v", err, arquivos)
	}
	if _, err := fonte.Apagar(context.Background(), empresa); err != nil {
		t.Fatal(err)
	}
	if _, ok := s3.objetos[outra]; !ok || len(s3.objetos) != 1 {
		t.Fatalf("apagou objeto de outro tenant: %v", s3.objetos)
	}
}
