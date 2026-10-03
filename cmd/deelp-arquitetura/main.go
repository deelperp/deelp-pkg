// Command deelp-arquitetura confere as dependências entre camadas de um serviço:
// domínio não importa aplicação, adapters nem infra; aplicação não importa
// adapters nem infra. Violações já conhecidas ficam no baseline; só as novas
// reprovam.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type regra struct {
	origem    string
	proibidas []string
}

var regras = []regra{
	{origem: "internal/domain", proibidas: []string{"internal/application", "internal/adapters", "internal/infra"}},
	{origem: "internal/application", proibidas: []string{"internal/adapters", "internal/infra"}},
}

type violacao struct {
	arquivo string
	importa string
}

func (v violacao) String() string { return v.arquivo + " -> " + v.importa }

func main() {
	raiz := flag.String("raiz", ".", "diretório do serviço (com go.mod)")
	baseline := flag.String("baseline", "", "arquivo com violações aceitas")
	gerar := flag.Bool("gerar-baseline", false, "imprime as violações atuais no formato do baseline")
	estrito := flag.Bool("estrito", false, "reprova também entradas do baseline que já não existem")
	flag.Parse()
	os.Exit(executar(*raiz, *baseline, *gerar, *estrito, os.Stdout, os.Stderr))
}

func executar(raiz, caminhoBaseline string, gerar, estrito bool, saida, erros io.Writer) int {
	atuais, err := Verificar(raiz)
	if err != nil {
		fmt.Fprintln(erros, err)
		return 2
	}
	if gerar {
		for _, v := range atuais {
			fmt.Fprintln(saida, v)
		}
		return 0
	}
	aceitas := map[string]bool{}
	if caminhoBaseline != "" {
		aceitas, err = lerBaseline(caminhoBaseline)
		if err != nil {
			fmt.Fprintln(erros, err)
			return 2
		}
	}
	codigo := 0
	vistas := map[string]bool{}
	for _, v := range atuais {
		vistas[v.String()] = true
		if !aceitas[v.String()] {
			fmt.Fprintf(erros, "violação nova: %s\n", v)
			codigo = 1
		}
	}
	var obsoletas []string
	for linha := range aceitas {
		if !vistas[linha] {
			obsoletas = append(obsoletas, linha)
		}
	}
	sort.Strings(obsoletas)
	for _, linha := range obsoletas {
		fmt.Fprintf(erros, "baseline obsoleto (remova a linha): %s\n", linha)
		if estrito {
			codigo = 1
		}
	}
	if codigo == 0 {
		fmt.Fprintf(saida, "arquitetura ok: %d violação(ões) conhecida(s), %d obsoleta(s)\n", len(atuais), len(obsoletas))
	}
	return codigo
}

func Verificar(raiz string) ([]violacao, error) {
	modulo, err := moduloDe(raiz)
	if err != nil {
		return nil, err
	}
	var encontradas []violacao
	err = filepath.WalkDir(raiz, func(caminho string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			nome := d.Name()
			if caminho != raiz && (nome == "vendor" || nome == "testdata" || strings.HasPrefix(nome, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(caminho, ".go") || strings.HasSuffix(caminho, "_test.go") {
			return nil
		}
		relativo, err := filepath.Rel(raiz, caminho)
		if err != nil {
			return err
		}
		relativo = filepath.ToSlash(relativo)
		r, ok := regraDe(relativo)
		if !ok {
			return nil
		}
		arquivo, err := parser.ParseFile(token.NewFileSet(), caminho, nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("%s: %w", relativo, err)
		}
		for _, imp := range arquivo.Imports {
			caminhoImport, _ := strconv.Unquote(imp.Path.Value)
			if !strings.HasPrefix(caminhoImport, modulo+"/") {
				continue
			}
			interno := strings.TrimPrefix(caminhoImport, modulo+"/")
			for _, proibida := range r.proibidas {
				if interno == proibida || strings.HasPrefix(interno, proibida+"/") {
					encontradas = append(encontradas, violacao{arquivo: relativo, importa: interno})
				}
			}
		}
		return nil
	})
	sort.Slice(encontradas, func(i, j int) bool { return encontradas[i].String() < encontradas[j].String() })
	return encontradas, err
}

func regraDe(relativo string) (regra, bool) {
	for _, r := range regras {
		if strings.HasPrefix(relativo, r.origem+"/") {
			return r, true
		}
	}
	return regra{}, false
}

func moduloDe(raiz string) (string, error) {
	f, err := os.Open(filepath.Join(raiz, "go.mod"))
	if err != nil {
		return "", err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		linha := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(linha, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(linha, "module ")), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("go.mod sem diretiva module")
}

func lerBaseline(caminho string) (map[string]bool, error) {
	conteudo, err := os.ReadFile(caminho)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	aceitas := map[string]bool{}
	for _, linha := range strings.Split(string(conteudo), "\n") {
		linha = strings.TrimSpace(linha)
		if linha == "" || strings.HasPrefix(linha, "#") {
			continue
		}
		aceitas[linha] = true
	}
	return aceitas, nil
}
