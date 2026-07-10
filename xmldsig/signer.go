// Package xmldsig assina e valida documentos fiscais eletrônicos (DFe) no
// padrão XML-DSig enveloped exigido pela SEFAZ/ITI. É a implementação única
// compartilhada por nfe-service, nfse-service e mdfe-service.
//
// Perfil de assinatura DFe (idêntico para NF-e, NFS-e nacional e MDF-e):
//   - Canonicalização: C14N inclusiva (http://www.w3.org/TR/2001/REC-xml-c14n-20010315)
//   - SignatureMethod:  RSA-SHA1
//   - DigestMethod:     SHA-1
//   - Transforms:       enveloped-signature + C14N inclusiva
//   - KeyInfo:          X509Certificate EndCertOnly (somente o certificado folha)
//   - <Signature>:      sem prefixo (namespace default), posicionada como irmã do alvo
//
// SHA-1 é exigência do leiaute DFe — não substituir por SHA-256.
package xmldsig

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/beevik/etree"
	dsig "github.com/russellhaering/goxmldsig"
	pkcs12 "software.sslmate.com/src/go-pkcs12"
)

// Signer assina e valida um elemento DFe identificado pelo atributo Id.
type Signer interface {
	// Assinar assina o elemento tagAlvo (que deve possuir atributo Id) com o
	// certificado A1 (PKCS#12) e sua senha. Retorna o XML com a <Signature>
	// enveloped posicionada como irmã do alvo. Exemplos de tagAlvo: "infNFe",
	// "infDPS", "infMDFe", "infEvento".
	Assinar(xmlBytes, pfxBytes []byte, senha, tagAlvo string) ([]byte, error)
	// Validar verifica a assinatura enveloped do elemento tagAlvo contra o
	// certificado embutido no XML, recanonicalizando com o mesmo canonicalizador
	// (biblioteca, nunca regex) e recomputando o digest.
	Validar(xmlBytes []byte, tagAlvo string) (bool, error)
}

type xmlSigner struct{}

var _ Signer = (*xmlSigner)(nil)

// NewSigner devolve o assinador DFe padrão.
func NewSigner() Signer {
	return &xmlSigner{}
}

// MemoryX509KeyStore adapta o par chave/certificado do A1 para o goxmldsig.
// GetKeyPair devolve apenas o certificado folha (EndCertOnly).
type MemoryX509KeyStore struct {
	PrivateKey *rsa.PrivateKey
	Cert       *x509.Certificate
}

func (m *MemoryX509KeyStore) GetKeyPair() (*rsa.PrivateKey, []byte, error) {
	return m.PrivateKey, m.Cert.Raw, nil
}

// canonicalizadorDFe é a C14N inclusiva (REC-xml-c14n-20010315) exigida pelo DFe.
func canonicalizadorDFe() dsig.Canonicalizer {
	return dsig.MakeC14N10RecCanonicalizer()
}

func (s *xmlSigner) Assinar(xmlBytes, pfxBytes []byte, senha, tagAlvo string) ([]byte, error) {
	privateKey, cert, _, err := pkcs12.DecodeChain(pfxBytes, senha)
	if err != nil {
		return nil, fmt.Errorf("xmldsig: erro ao decodificar PKCS#12: %w", err)
	}
	rsaKey, ok := privateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("xmldsig: chave privada do certificado não é RSA")
	}

	keyStore := &MemoryX509KeyStore{PrivateKey: rsaKey, Cert: cert}

	ctx := dsig.NewDefaultSigningContext(keyStore)
	ctx.Canonicalizer = canonicalizadorDFe()
	ctx.Prefix = ""
	if err := ctx.SetSignatureMethod(dsig.RSASHA1SignatureMethod); err != nil {
		return nil, fmt.Errorf("xmldsig: método de assinatura: %w", err)
	}

	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(xmlBytes); err != nil {
		return nil, fmt.Errorf("xmldsig: falha ao ler xml: %w", err)
	}

	alvo := doc.FindElement("//" + tagAlvo)
	if alvo == nil {
		return nil, fmt.Errorf("xmldsig: tag %s não encontrada", tagAlvo)
	}
	parent := alvo.Parent()
	if parent == nil {
		return nil, fmt.Errorf("xmldsig: elemento %s não possui pai para receber a assinatura", tagAlvo)
	}

	assinado, err := ctx.SignEnveloped(alvo)
	if err != nil {
		return nil, fmt.Errorf("xmldsig: erro ao assinar XML: %w", err)
	}

	sig := assinado.FindElement("Signature")
	if sig == nil {
		return nil, errors.New("xmldsig: assinatura não gerada")
	}
	assinado.RemoveChild(sig)

	// Posiciona <Signature> como irmã do alvo, logo após ele.
	parent.AddChild(sig)

	doc.WriteSettings = etree.WriteSettings{
		CanonicalEndTags: false,
		CanonicalText:    false,
		CanonicalAttrVal: false,
	}

	return doc.WriteToBytes()
}

func (s *xmlSigner) Validar(xmlBytes []byte, tagAlvo string) (bool, error) {
	if len(xmlBytes) == 0 {
		return false, errors.New("xmldsig: XML vazio")
	}

	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(xmlBytes); err != nil {
		return false, fmt.Errorf("xmldsig: falha ao ler xml: %w", err)
	}

	alvo := doc.FindElement("//" + tagAlvo)
	if alvo == nil {
		return false, fmt.Errorf("xmldsig: tag %s não encontrada", tagAlvo)
	}
	sig := doc.FindElement("//Signature")
	if sig == nil {
		return false, errors.New("xmldsig: elemento Signature não encontrado")
	}
	signedInfo := sig.FindElement("SignedInfo")
	if signedInfo == nil {
		return false, errors.New("xmldsig: elemento SignedInfo não encontrado")
	}

	digestValue := textoDe(sig, "SignedInfo/Reference/DigestValue")
	signatureValue := textoDe(sig, "SignatureValue")
	x509B64 := textoDe(sig, "KeyInfo/X509Data/X509Certificate")
	if digestValue == "" || signatureValue == "" || x509B64 == "" {
		return false, errors.New("xmldsig: assinatura incompleta (Digest/Signature/X509 ausente)")
	}

	cert, err := parseCertB64(x509B64)
	if err != nil {
		return false, err
	}
	pubKey, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return false, errors.New("xmldsig: chave pública do certificado não é RSA")
	}

	canonicalizer := canonicalizadorDFe()

	// 1) Recomputa o digest do alvo (a <Signature> é irmã, não descendente do
	// alvo, então a canonicalização direta equivale ao transform enveloped).
	canonAlvo, err := canonicalizer.Canonicalize(alvo)
	if err != nil {
		return false, fmt.Errorf("xmldsig: canonicalização do alvo: %w", err)
	}
	digAlvo := sha1.Sum(canonAlvo)
	if base64.StdEncoding.EncodeToString(digAlvo[:]) != digestValue {
		return false, errors.New("xmldsig: DigestValue não corresponde ao conteúdo assinado")
	}

	// 2) Verifica a assinatura RSA sobre o SignedInfo canonicalizado.
	canonSI, err := canonicalizer.Canonicalize(signedInfo)
	if err != nil {
		return false, fmt.Errorf("xmldsig: canonicalização do SignedInfo: %w", err)
	}
	siHash := sha1.Sum(canonSI)
	rawSig, err := base64.StdEncoding.DecodeString(signatureValue)
	if err != nil {
		return false, fmt.Errorf("xmldsig: SignatureValue base64 inválido: %w", err)
	}
	if err := rsa.VerifyPKCS1v15(pubKey, crypto.SHA1, siHash[:], rawSig); err != nil {
		return false, fmt.Errorf("xmldsig: assinatura digital inválida: %w", err)
	}

	return true, nil
}

func textoDe(el *etree.Element, path string) string {
	found := el.FindElement(path)
	if found == nil {
		return ""
	}
	return found.Text()
}

func parseCertB64(b64 string) (*x509.Certificate, error) {
	der, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("xmldsig: certificado base64 inválido: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("xmldsig: erro ao parsear certificado X509: %w", err)
	}
	return cert, nil
}
