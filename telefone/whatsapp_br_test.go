package telefone_test

import (
	"testing"

	"github.com/deelperp/deelp-pkg/telefone"
)

func TestNormalizarWhatsAppBR(t *testing.T) {
	casos := []struct {
		nome     string
		bruto    string
		esperado string
	}{
		{"celular com mascara", "(11) 99999-0000", "5511999990000"},
		{"celular com ddi e mais", "+55 11 99999-0000", "5511999990000"},
		{"fixo com ddd", "41 3333-4444", "554133334444"},
		{"ja normalizado", "5511999990000", "5511999990000"},
		{"sem ddd", "99999-0000", ""},
		{"curto demais", "123", ""},
		{"longo demais", "55119999900001234", ""},
		{"sem digitos", "sem digito", ""},
		{"vazio", "", ""},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			if obtido := telefone.NormalizarWhatsAppBR(caso.bruto); obtido != caso.esperado {
				t.Fatalf("NormalizarWhatsAppBR(%q) = %q, esperado %q", caso.bruto, obtido, caso.esperado)
			}
		})
	}
}

func TestEhWhatsAppBRValido(t *testing.T) {
	if !telefone.EhWhatsAppBRValido("5511999990000") {
		t.Fatalf("esperava número com DDI válido")
	}
	if telefone.EhWhatsAppBRValido("11999990000") {
		t.Fatalf("número sem DDI não pode ser válido")
	}
}
