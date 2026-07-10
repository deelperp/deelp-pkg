package xmldsig

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/beevik/etree"
)

const senhaTeste = "senha"

func carregarPfx(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "test-cert.pfx"))
	if err != nil {
		t.Fatalf("erro ao ler test-cert.pfx (gere com: cd testdata && GOWORK=off go run gen_cert.go): %v", err)
	}
	return b
}

type casoDFe struct {
	nome    string
	tagAlvo string
	raiz    string
	xml     string
	// Goldens — assinatura é determinística (PKCS1v15 + chave fixa).
	digest    string
	assinatura string
}

var casos = []casoDFe{
	{
		nome:    "NFe/infNFe",
		tagAlvo: "infNFe",
		raiz:    "NFe",
		xml: `<NFe xmlns="http://www.portalfiscal.inf.br/nfe">` +
			`<infNFe versao="4.00" Id="NFe35250300000000000191550010000000011000000010">` +
			`<ide><cUF>35</cUF><natOp>VENDA</natOp></ide></infNFe></NFe>`,
		digest:     "LvgbbsC9bdqHEFdM+X+rg4ASaGg=",
		assinatura: "ZUxLhxejsZ6cOGa8OEVMwDE3/LycZEdlQf/reYtidTJELbgWU8ZRLoyJCx6M28SUu+FFjBL9tSjbqDO6LnnBJV4vLT9xJqWURMSnSZOlr4VbBiZRuHRYIrNVsHOuswbNF6fUUi7oAMLorjfLzaEpKE1hp6KDiZFAO1MnLzX2b9RjH6Sqhxbc41kPxTF6ZhsYh0l30IvbgOC8CFXAR+oShfnQJjIZxd5QeaU74A3+amZsUcJn/TJpFpg45F9ss+aCencrlZaeINvsWo74/SWLgynoedNw7nYqItoAt07iKYiWcl8V34i8esp4QYk9RFvWavn2x71Sz/RPBivRBYyihA==",
	},
	{
		nome:    "NFSe/infDPS",
		tagAlvo: "infDPS",
		raiz:    "DPS",
		xml: `<DPS xmlns="http://www.sped.fazenda.gov.br/nfse">` +
			`<infDPS Id="DPS1"><tpAmb>2</tpAmb><serie>1</serie></infDPS></DPS>`,
		digest:     "M1J1Lzk3ipOPGy++LUBHY9Jab2g=",
		assinatura: "Ir/VuQ2gaCsZTFEIM0rXBZiw7FdEn0O0PQUTM/taY+LZz4YM0yZuzmc6e+jMAWhWzmWVLYDFs6B1irx0aHDx4RHjMtfZ5QoqvbuObErmBMUMJef3j9QTQ4JzgVdy/MZdbwpMKvBjJXgTruSAZXNkUxDbsGRz/CknExepBa2t2xys5Sl1XuSNeOmnT/aUCu91jUTLehwGymsRJfOa8ypAx10pmQsfGsHDYyQbv8CviqtXaPZJDskW1ufwkFSaIhBOjIDci56PrI7uub7gd+6khMbgnEELRtFzsY8R1o0tF3LAHS/SqoT1SrsHWaZemjnQKXWoOpbHYeceK1fg8CAmKw==",
	},
	{
		nome:    "MDFe/infMDFe",
		tagAlvo: "infMDFe",
		raiz:    "MDFe",
		xml: `<MDFe xmlns="http://www.portalfiscal.inf.br/mdfe">` +
			`<infMDFe versao="3.00" Id="MDFe35250300000000000191580010000000011000000010">` +
			`<ide><cUF>35</cUF></ide></infMDFe></MDFe>`,
		digest:     "jTPLimSHMiAOC1M3PkcbRP1TGBw=",
		assinatura: "IpZWBKZC7pxul5aZVIklfb8K5vhRJ0HIeKOxM0MvkhvdgMjmVKZTAFDu+ghuufaSPG+JysAmG+wjjr9BGUDUZ9/5ejKZTnhPMLZlXSHlp0E1ybcn6Rp+tedM6fP/MVK/QNX5JE3988fT2urPUaSURcanGJUM6167CX5eoNKVX1Vpc4C47C31+CAUy+0dJRMy3wOVGr1OrSbS8YAfopKj3Uxz1qPIUSPtNLuwTfRePVDaIlXi879E0DW6TVb8gy+pVyyoa+2+hC/ZTy+w5NguVP0CBR1n39i1/S7g/fkO2DZ06hfIdwfLbGCb/srgCi8vXU320FHQo9Qj+RFQSJgVcQ==",
	},
}

const (
	nsDSig       = "http://www.w3.org/2000/09/xmldsig#"
	algC14NIncl  = "http://www.w3.org/TR/2001/REC-xml-c14n-20010315"
	algRSASHA1   = "http://www.w3.org/2000/09/xmldsig#rsa-sha1"
	algSHA1      = "http://www.w3.org/2000/09/xmldsig#sha1"
	algEnveloped = "http://www.w3.org/2000/09/xmldsig#enveloped-signature"
)

func TestAssinarGoldenPerfilDFe(t *testing.T) {
	pfx := carregarPfx(t)
	s := NewSigner()

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			out, err := s.Assinar([]byte(c.xml), pfx, senhaTeste, c.tagAlvo)
			if err != nil {
				t.Fatalf("Assinar: %v", err)
			}

			doc := etree.NewDocument()
			if err := doc.ReadFromBytes(out); err != nil {
				t.Fatalf("parse saída: %v", err)
			}
			raiz := doc.Root()
			if raiz == nil || raiz.Tag != c.raiz {
				t.Fatalf("raiz inesperada: %v", raiz)
			}

			// 4) <Signature> é irmã do alvo, posicionada logo após ele.
			filhos := raiz.ChildElements()
			if len(filhos) != 2 || filhos[0].Tag != c.tagAlvo || filhos[1].Tag != "Signature" {
				var tags []string
				for _, f := range filhos {
					tags = append(tags, f.Tag)
				}
				t.Fatalf("posição da Signature errada, filhos da raiz: %v", tags)
			}

			sig := filhos[1]
			// 1) <Signature> sem prefixo, namespace default = xmldsig.
			if sig.Space != "" {
				t.Errorf("Signature com prefixo %q; esperado sem prefixo", sig.Space)
			}
			if ns := sig.SelectAttrValue("xmlns", ""); ns != nsDSig {
				t.Errorf("xmlns da Signature = %q; esperado %q", ns, nsDSig)
			}
			assertSemPrefixo(t, sig)

			// 2) e 3) algoritmos do perfil DFe.
			assertAttr(t, sig, "SignedInfo/CanonicalizationMethod", "Algorithm", algC14NIncl)
			assertAttr(t, sig, "SignedInfo/SignatureMethod", "Algorithm", algRSASHA1)
			assertAttr(t, sig, "SignedInfo/Reference/DigestMethod", "Algorithm", algSHA1)
			transforms := sig.FindElements("SignedInfo/Reference/Transforms/Transform")
			if len(transforms) != 2 {
				t.Fatalf("esperado 2 Transforms, obtido %d", len(transforms))
			}
			if got := transforms[0].SelectAttrValue("Algorithm", ""); got != algEnveloped {
				t.Errorf("Transform[0]=%q; esperado enveloped", got)
			}
			if got := transforms[1].SelectAttrValue("Algorithm", ""); got != algC14NIncl {
				t.Errorf("Transform[1]=%q; esperado C14N inclusiva", got)
			}

			// Reference aponta para o Id do alvo.
			if uri := textoAttr(sig, "SignedInfo/Reference", "URI"); uri != "#"+c.tagAlvoID() {
				t.Errorf("Reference URI=%q; esperado #%s", uri, c.tagAlvoID())
			}

			// EndCertOnly — exatamente um X509Certificate.
			if n := len(sig.FindElements("KeyInfo/X509Data/X509Certificate")); n != 1 {
				t.Errorf("esperado 1 X509Certificate (EndCertOnly), obtido %d", n)
			}

			digest := textoDe(sig, "SignedInfo/Reference/DigestValue")
			assinatura := textoDe(sig, "SignatureValue")
			t.Logf("GOLDEN %s digest=%q assinatura=%q", c.nome, digest, assinatura)

			// 5) e 6) goldens determinísticos.
			if c.digest != "" && digest != c.digest {
				t.Errorf("DigestValue=%q; golden=%q", digest, c.digest)
			}
			if c.assinatura != "" && assinatura != c.assinatura {
				t.Errorf("SignatureValue=%q; golden=%q", assinatura, c.assinatura)
			}

			// 7) round-trip de validação.
			ok, err := s.Validar(out, c.tagAlvo)
			if err != nil || !ok {
				t.Errorf("Validar round-trip: ok=%v err=%v", ok, err)
			}
		})
	}
}

func TestValidarDetectaAdulteracao(t *testing.T) {
	pfx := carregarPfx(t)
	s := NewSigner()
	c := casos[0]

	out, err := s.Assinar([]byte(c.xml), pfx, senhaTeste, c.tagAlvo)
	if err != nil {
		t.Fatalf("Assinar: %v", err)
	}
	adulterado := strings.Replace(string(out), "<natOp>VENDA</natOp>", "<natOp>FRAUDE</natOp>", 1)
	if adulterado == string(out) {
		t.Fatal("falha ao adulterar o XML de teste")
	}
	ok, err := s.Validar([]byte(adulterado), c.tagAlvo)
	if ok {
		t.Errorf("Validar aceitou XML adulterado (err=%v)", err)
	}
}

func TestAssinarPfxInvalido(t *testing.T) {
	s := NewSigner()
	_, err := s.Assinar([]byte(casos[0].xml), []byte("nao-e-um-pfx"), senhaTeste, "infNFe")
	if err == nil {
		t.Fatal("esperado erro com PFX inválido")
	}
}

// TestValidarContraXSDOficial valida a NF-e assinada contra o XSD oficial
// (nfe_v4.00.xsd) quando NFE_XSD_DIR está configurado. Sem o XSD, o teste é
// pulado — o Pacote de Liberação não é versionado no repositório.
func TestValidarContraXSDOficial(t *testing.T) {
	dir := os.Getenv("NFE_XSD_DIR")
	if dir == "" {
		t.Skip("NFE_XSD_DIR não configurado; XSD oficial ausente do repo")
	}
	schema := filepath.Join(dir, "nfe_v4.00.xsd")
	if _, err := os.Stat(schema); err != nil {
		t.Skipf("XSD raiz não encontrado em %s", schema)
	}
	if _, err := exec.LookPath("xmllint"); err != nil {
		t.Skip("xmllint indisponível no PATH")
	}

	pfx := carregarPfx(t)
	out, err := NewSigner().Assinar([]byte(casos[0].xml), pfx, senhaTeste, "infNFe")
	if err != nil {
		t.Fatalf("Assinar: %v", err)
	}
	tmp, err := os.CreateTemp(t.TempDir(), "nfe-*.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmp.Write(out); err != nil {
		t.Fatal(err)
	}
	tmp.Close()

	cmd := exec.Command("xmllint", "--noout", "--schema", schema, tmp.Name())
	if saida, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("XML assinado inválido contra XSD oficial: %v\n%s", err, saida)
	}
}

func (c casoDFe) tagAlvoID() string {
	doc := etree.NewDocument()
	_ = doc.ReadFromString(c.xml)
	el := doc.FindElement("//" + c.tagAlvo)
	if el == nil {
		return ""
	}
	return el.SelectAttrValue("Id", "")
}

func assertAttr(t *testing.T, el *etree.Element, path, attr, esperado string) {
	t.Helper()
	if got := textoAttr(el, path, attr); got != esperado {
		t.Errorf("%s/@%s = %q; esperado %q", path, attr, got, esperado)
	}
}

func textoAttr(el *etree.Element, path, attr string) string {
	found := el.FindElement(path)
	if found == nil {
		return ""
	}
	return found.SelectAttrValue(attr, "")
}

func assertSemPrefixo(t *testing.T, el *etree.Element) {
	t.Helper()
	if el.Space != "" {
		t.Errorf("elemento %q usa prefixo %q dentro da Signature", el.Tag, el.Space)
	}
	for _, filho := range el.ChildElements() {
		assertSemPrefixo(t, filho)
	}
}
