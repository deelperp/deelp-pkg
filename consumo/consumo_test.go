package consumo

import (
	"strings"
	"testing"
	"time"
)

func TestCompetenciaDe(t *testing.T) {
	c := CompetenciaDe(time.Date(2026, 8, 31, 23, 59, 59, 0, time.Local))
	if c != "2026-08" {
		t.Errorf("competência = %q, esperado \"2026-08\"", c)
	}
}

func TestCompetenciaValida(t *testing.T) {
	casos := []struct {
		entrada Competencia
		valida  bool
	}{
		{"2026-08", true},
		{"2026-1", false},
		{"08-2026", false},
		{"", false},
		{"2026-13", false},
	}

	for _, caso := range casos {
		err := caso.entrada.Valida()
		if (err == nil) != caso.valida {
			t.Errorf("Valida(%q): err = %v, esperado valida = %v", caso.entrada, err, caso.valida)
		}
	}
}

func TestIntervalo_FimEhInicioDoMesSeguinte(t *testing.T) {
	inicio, fim, err := Competencia("2026-08").Intervalo()
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	esperadoInicio := time.Date(2026, 8, 1, 0, 0, 0, 0, time.Local)
	esperadoFim := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)

	if !inicio.Equal(esperadoInicio) {
		t.Errorf("inicio = %v, esperado %v", inicio, esperadoInicio)
	}
	if !fim.Equal(esperadoFim) {
		t.Errorf("fim = %v, esperado %v", fim, esperadoFim)
	}
}

// O intervalo é semiaberto. Um documento criado às 23:59 do último dia precisa
// cair dentro da competência — usar o último dia como teto o perderia.
func TestIntervalo_UltimoInstanteDoMesEntra(t *testing.T) {
	inicio, fim, _ := Competencia("2026-08").Intervalo()
	ultimoInstante := time.Date(2026, 8, 31, 23, 59, 59, 999_000_000, time.Local)

	if ultimoInstante.Before(inicio) || !ultimoInstante.Before(fim) {
		t.Errorf("%v deveria estar em [%v, %v)", ultimoInstante, inicio, fim)
	}
}

func TestIntervalo_ViradaDeAno(t *testing.T) {
	_, fim, err := Competencia("2026-12").Intervalo()
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	esperado := time.Date(2027, 1, 1, 0, 0, 0, 0, time.Local)
	if !fim.Equal(esperado) {
		t.Errorf("fim = %v, esperado %v", fim, esperado)
	}
}

func TestIntervalo_CompetenciaInvalida(t *testing.T) {
	if _, _, err := Competencia("lixo").Intervalo(); err == nil {
		t.Error("esperado erro para competência inválida")
	}
}

func TestFaixasRFC3339_IncluiLocalEUTC(t *testing.T) {
	inicio, fim, err := Competencia("2026-09").Intervalo()
	if err != nil {
		t.Fatalf("intervalo: %v", err)
	}

	localIni, localFim, utcIni, utcFim := FaixasRFC3339(inicio, fim)

	if localIni != inicio.Format(time.RFC3339) || localFim != fim.Format(time.RFC3339) {
		t.Errorf("faixa local = [%s, %s)", localIni, localFim)
	}
	if utcIni != inicio.UTC().Format(time.RFC3339) || utcFim != fim.UTC().Format(time.RFC3339) {
		t.Errorf("faixa utc = [%s, %s)", utcIni, utcFim)
	}
	if !strings.HasPrefix(localIni, "2026-09-01T") || !strings.HasPrefix(utcIni, "2026-09-01T") {
		t.Errorf("inícios deveriam cair em 1º de setembro: local=%s utc=%s", localIni, utcIni)
	}
}
