// Package dfe define a chave S3 canônica dos documentos fiscais eletrônicos
// (NF-e, MDF-e, NFS-e e futuros).
//
// Formato:
//
//	{inscricaoFederal}/{ambiente}/{tipo}/{competencia}/{chaveAcesso}/envio.xml
//	{inscricaoFederal}/{ambiente}/{tipo}/{competencia}/{chaveAcesso}/proc.xml
//	{inscricaoFederal}/{ambiente}/{tipo}/{competencia}/{chaveAcesso}/eventos/{evento}[-seq{n}].xml
package dfe

import (
	"fmt"
	"strings"
	"unicode"
)

const (
	TipoNFe  = "nfe"
	TipoMDFe = "mdfe"
	TipoNFSe = "nfse"

	ArtefatoEnvio = "envio"
	ArtefatoProc  = "proc"

	AmbienteHomologacao = "homologacao"
	AmbienteProducao    = "producao"
)

// ChaveDocumento monta o path do artefato principal (envio ou proc).
// competencia deve ser AAAA-MM; artefato tipicamente ArtefatoEnvio ou ArtefatoProc.
func ChaveDocumento(inscricao, ambiente, tipo, competencia, chaveAcesso, artefato string) string {
	base := baseDocumento(inscricao, ambiente, tipo, competencia, chaveAcesso)
	art := strings.Trim(strings.ToLower(strings.TrimSpace(artefato)), "/")
	if art == "" {
		art = ArtefatoEnvio
	}
	if !strings.HasSuffix(art, ".xml") {
		art += ".xml"
	}
	return base + "/" + art
}

// ChaveEvento monta o path de um procEvento sob .../eventos/.
// seq <= 0 omite o sufixo -seq{n} (ex.: cancelamento sem sequência).
func ChaveEvento(inscricao, ambiente, tipo, competencia, chaveAcesso, evento string, seq int) string {
	base := baseDocumento(inscricao, ambiente, tipo, competencia, chaveAcesso)
	nome := strings.Trim(strings.ToLower(strings.TrimSpace(evento)), "/")
	nome = strings.TrimSuffix(nome, ".xml")
	if nome == "" {
		nome = "evento"
	}
	if seq > 0 {
		nome = fmt.Sprintf("%s-seq%d", nome, seq)
	}
	return base + "/eventos/" + nome + ".xml"
}

func baseDocumento(inscricao, ambiente, tipo, competencia, chaveAcesso string) string {
	return fmt.Sprintf("%s/%s/%s/%s/%s",
		SomenteDigitos(inscricao),
		NormalizarAmbiente(ambiente),
		NormalizarTipo(tipo),
		normalizarCompetencia(competencia),
		strings.TrimSpace(chaveAcesso),
	)
}

// SomenteDigitos remove máscara de CPF/CNPJ.
func SomenteDigitos(s string) string {
	b := make([]rune, 0, len(s))
	for _, r := range s {
		if unicode.IsDigit(r) {
			b = append(b, r)
		}
	}
	return string(b)
}

// NormalizarAmbiente aceita português, inglês e código SEFAZ (1/2).
func NormalizarAmbiente(valor string) string {
	v := strings.ToLower(strings.TrimSpace(valor))
	switch v {
	case "1", "producao", "production", "prod":
		return AmbienteProducao
	default:
		return AmbienteHomologacao
	}
}

// NormalizarTipo devolve nfe|mdfe|nfse; desconhecido vira o valor limpo em minúsculas.
func NormalizarTipo(tipo string) string {
	t := strings.ToLower(strings.TrimSpace(tipo))
	switch t {
	case TipoNFe, TipoMDFe, TipoNFSe:
		return t
	default:
		return t
	}
}

func normalizarCompetencia(c string) string {
	c = strings.TrimSpace(c)
	if len(c) >= 7 {
		return c[:7]
	}
	return c
}
