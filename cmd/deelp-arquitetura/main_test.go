package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func servicoFalso(t *testing.T, arquivos map[string]string) string {
	t.Helper()
	raiz := t.TempDir()
	arquivos["go.mod"] = "module deelp/falso-service\n\ngo 1.27.1\n"
	for nome, conteudo := range arquivos {
		caminho := filepath.Join(raiz, nome)
		if err := os.MkdirAll(filepath.Dir(caminho), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(caminho, []byte(conteudo), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return raiz
}

func TestVerificar_ApontaDominioEAplicacaoImportandoCamadasExternas(t *testing.T) {
	raiz := servicoFalso(t, map[string]string{
		"internal/domain/models/usuario.go":         "package models\nimport _ \"deelp/falso-service/internal/infra/security\"\n",
		"internal/application/usecases/criar.go":    "package usecases\nimport (\n_ \"deelp/falso-service/internal/adapters/http_client\"\n_ \"deelp/falso-service/internal/domain/models\"\n)\n",
		"internal/adapters/repo/repo.go":            "package repo\nimport _ \"deelp/falso-service/internal/infra/database\"\n",
		"internal/domain/models/usuario_test.go":    "package models\nimport _ \"deelp/falso-service/internal/adapters/mocks\"\n",
		"internal/application/ports/out/porta.go":   "package out\nimport _ \"deelp/falso-service/internal/domain/models\"\n",
		"internal/domain/repositories/produto.go":   "package repositories\nimport _ \"deelp/falso-service/internal/application/ports/out\"\n",
		"internal/application/usecases/infra_ok.go": "package usecases\nimport _ \"github.com/deelperp/deelp-pkg/internal/infra\"\n",
	})

	violacoes, err := Verificar(raiz)
	if err != nil {
		t.Fatal(err)
	}

	var linhas []string
	for _, v := range violacoes {
		linhas = append(linhas, v.String())
	}
	esperado := []string{
		"internal/application/usecases/criar.go -> internal/adapters/http_client",
		"internal/domain/models/usuario.go -> internal/infra/security",
		"internal/domain/repositories/produto.go -> internal/application/ports/out",
	}
	if strings.Join(linhas, "\n") != strings.Join(esperado, "\n") {
		t.Fatalf("violações:\n%s\nesperado:\n%s", strings.Join(linhas, "\n"), strings.Join(esperado, "\n"))
	}
}

func TestExecutar_BaselineAceitaLegadoERecusaNovo(t *testing.T) {
	raiz := servicoFalso(t, map[string]string{
		"internal/domain/models/a.go": "package models\nimport _ \"deelp/falso-service/internal/infra/x\"\n",
		"internal/domain/models/b.go": "package models\nimport _ \"deelp/falso-service/internal/adapters/y\"\n",
	})
	baseline := filepath.Join(t.TempDir(), "baseline.txt")
	_ = os.WriteFile(baseline, []byte("# legado\ninternal/domain/models/a.go -> internal/infra/x\n"), 0o644)
	var saida, erros bytes.Buffer

	codigo := executar(raiz, baseline, false, false, &saida, &erros)

	if codigo != 1 || !strings.Contains(erros.String(), "internal/domain/models/b.go -> internal/adapters/y") || strings.Contains(erros.String(), "a.go") {
		t.Fatalf("código=%d erros=%s", codigo, erros.String())
	}
}

func TestExecutar_BaselineObsoletoSoReprovaNoModoEstrito(t *testing.T) {
	raiz := servicoFalso(t, map[string]string{"internal/domain/models/a.go": "package models\n"})
	baseline := filepath.Join(t.TempDir(), "baseline.txt")
	_ = os.WriteFile(baseline, []byte("internal/domain/models/a.go -> internal/infra/x\n"), 0o644)
	var saida, erros bytes.Buffer

	if codigo := executar(raiz, baseline, false, false, &saida, &erros); codigo != 0 || !strings.Contains(erros.String(), "obsoleto") {
		t.Fatalf("modo normal: código=%d erros=%s", codigo, erros.String())
	}
	if codigo := executar(raiz, baseline, false, true, &saida, &erros); codigo != 1 {
		t.Fatalf("modo estrito aceitou baseline obsoleto: %d", codigo)
	}
}
