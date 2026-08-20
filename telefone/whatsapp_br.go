package telefone

import (
	"regexp"
	"strings"
	"unicode"
)

var padraoWhatsAppBR = regexp.MustCompile(`^55\d{10,11}$`)

// NormalizarWhatsAppBR devolve o número em E.164 sem "+" (DDI 55) ou "" quando
// o que sobrou não é um telefone brasileiro válido. Número sem DDI é aceito
// pelo Baileys, que devolve messageId e deixa o envio registrado como "enviado"
// sem nunca ser entregue — por isso nada malformado pode ser publicado na fila.
func NormalizarWhatsAppBR(bruto string) string {
	var digitos strings.Builder
	for _, r := range strings.TrimSpace(bruto) {
		if unicode.IsDigit(r) {
			digitos.WriteRune(r)
		}
	}
	numero := digitos.String()
	if len(numero) >= 10 && len(numero) <= 11 {
		numero = "55" + numero
	}
	if !padraoWhatsAppBR.MatchString(numero) {
		return ""
	}
	return numero
}

// EhWhatsAppBRValido informa se o número já está em E.164 sem "+" (DDI 55).
func EhWhatsAppBRValido(numero string) bool {
	return padraoWhatsAppBR.MatchString(numero)
}
