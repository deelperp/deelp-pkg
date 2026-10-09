package observabilidade

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

var (
	segmentoUUID        = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	segmentoNumerico    = regexp.MustCompile(`^[0-9]{10,}$`)
	segmentoCPFCNPJ     = regexp.MustCompile(`^[0-9]{2,3}\.[0-9]{3}\.[0-9]{3}[/-][0-9/-]{2,7}$`)
	textoEmail          = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	textoJWT            = regexp.MustCompile(`eyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]*`)
	textoBearer         = regexp.MustCompile(`(?i)bearer\s+\S+`)
	textoCredencial     = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.\-]*://)[^@/\s:]+:[^@/\s]+@`)
	textoNumero         = regexp.MustCompile(`\d[\d.\-/ ]{8,}\d`)
	textoAutorizacao    = regexp.MustCompile(`(?i)\bauthorization\b\s*[:=]\s*(?:(?:bearer|basic)\s+)?\S+`)
	textoSegredo        = regexp.MustCompile(`(?i)\b(password|passwd|senha|secret|api[_-]?key|access[_-]?token|token|cookie|x-internal-key)\b\s*[:=]\s*("[^"]*"|'[^']*'|\S+)`)
	textoAWS            = regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`)
	segmentoTelefone    = regexp.MustCompile(`^\+\d{8,}$`)
	limiteTextoRedigido = 2000
	numeroIPv4          = regexp.MustCompile(`^\d{1,3}(\.\d{1,3}){3}$`)
	numeroData          = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}( \d{1,2})?$`)
	atributosDeURL      = map[attribute.Key]bool{"url.path": true, "http.target": true, "url.full": true, "http.url": true, "url.query": true}
)

func palavraDeIdentificador(parte string) bool {
	if len(parte) < 3 {
		return false
	}
	for _, r := range parte {
		if !unicode.IsLower(r) {
			return false
		}
	}
	return true
}

func pareceIdentificador(seg string) bool {
	partes := strings.FieldsFunc(seg, func(r rune) bool { return r == '-' || r == '_' })
	if len(partes) < 3 {
		return false
	}
	palavras := 0
	for _, p := range partes {
		if palavraDeIdentificador(p) {
			palavras++
		}
	}
	return palavras*10 >= len(partes)*6
}

func segmentoSensivel(seg string) bool {
	if strings.Contains(seg, "@") || segmentoTelefone.MatchString(seg) || segmentoUUID.MatchString(seg) || segmentoNumerico.MatchString(seg) || segmentoCPFCNPJ.MatchString(seg) || textoJWT.MatchString(seg) {
		return true
	}
	if len(seg) < 24 {
		return false
	}
	var digito, maiuscula, minuscula bool
	for _, r := range seg {
		switch {
		case unicode.IsDigit(r):
			digito = true
		case unicode.IsUpper(r):
			maiuscula = true
		case unicode.IsLetter(r):
			minuscula = true
		case r == '-' || r == '_' || r == '=':
		default:
			return false
		}
	}
	if !digito || pareceIdentificador(seg) {
		return false
	}
	return len(seg) >= 32 || (maiuscula && minuscula)
}

func sanitizarPath(path string) string {
	segs := strings.Split(path, "/")
	for i, s := range segs {
		if segmentoSensivel(s) {
			segs[i] = ":id"
		}
	}
	return strings.Join(segs, "/")
}

// sanitizarURL tira credenciais e query, e troca segmentos que parecem id, token ou
// chave de acesso. Token de link público e chave de NF-e vão no path e não podem
// parar no backend de traces.
func sanitizarURL(bruta string) string {
	if !strings.Contains(bruta, "://") {
		caminho, _, _ := strings.Cut(bruta, "?")
		return sanitizarPath(caminho)
	}
	u, err := url.Parse(bruta)
	if err != nil {
		return "[url inválida]"
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	u.Path = sanitizarPath(u.Path)
	u.RawPath = ""
	return u.String()
}

func redigirAtributosDeURL(s sdktrace.ReadWriteSpan) {
	if k := s.SpanKind(); k != trace.SpanKindServer && k != trace.SpanKindClient {
		return
	}
	for _, kv := range s.Attributes() {
		if !atributosDeURL[kv.Key] || kv.Value.Type() != attribute.STRING {
			continue
		}
		if limpo := sanitizarURL(kv.Value.AsString()); limpo != kv.Value.AsString() {
			s.SetAttributes(attribute.String(string(kv.Key), limpo))
		}
	}
}

func mascararNumero(m string) string {
	if numeroIPv4.MatchString(m) || numeroData.MatchString(m) {
		return m
	}
	return "[numero]"
}

// redigirTexto remove de mensagens de erro o que identifica pessoa ou dá acesso:
// e-mail, JWT, bearer, senha em URL, CPF/CNPJ/telefone/chave (sequências longas de dígitos)
// e tokens. Erro de banco costuma carregar o valor da linha (violação de unicidade).
func redigirTexto(msg string) string {
	if len(msg) > limiteTextoRedigido {
		msg = string([]rune(msg)[:min(len([]rune(msg)), limiteTextoRedigido)])
	}
	msg = textoJWT.ReplaceAllString(msg, "[jwt]")
	msg = textoAutorizacao.ReplaceAllString(msg, "Authorization: [redigido]")
	msg = textoBearer.ReplaceAllString(msg, "bearer [token]")
	msg = textoSegredo.ReplaceAllString(msg, "${1}=[redigido]")
	msg = textoCredencial.ReplaceAllString(msg, "${1}[credenciais]@")
	msg = textoEmail.ReplaceAllString(msg, "[email]")
	msg = textoAWS.ReplaceAllString(msg, "[chave-aws]")
	msg = textoNumero.ReplaceAllStringFunc(msg, mascararNumero)
	palavras := strings.FieldsFunc(msg, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '=')
	})
	var pares []string
	vistos := map[string]bool{}
	for _, p := range palavras {
		if len(p) >= 24 && !vistos[p] && segmentoSensivel(p) {
			vistos[p] = true
			pares = append(pares, p, "[token]")
		}
	}
	if len(pares) > 0 {
		msg = strings.NewReplacer(pares...).Replace(msg)
	}
	return msg
}

// RedigirTexto expõe a redação para quem grava texto livre em atributo de span.
func RedigirTexto(msg string) string { return redigirTexto(msg) }
