// Package consumo define o contrato das rotas internas de contagem que
// alimentam a apuração de consumo do plano.
//
// A contagem é feita por PULL: cliente-service pergunta a cada serviço dono do
// dado quantas unidades o tenant produziu na competência. A alternativa —
// contador incrementado por evento — erra em silêncio quando um evento se
// perde ou é reentregue, e o erro só aparece na fatura. Recontar sempre dá o
// mesmo número.
package consumo

import (
	"fmt"
	"time"
)

// Competencia é o mês de apuração no formato "2006-01", o mesmo usado por
// fatura.Competencia.
type Competencia string

const formatoCompetencia = "2006-01"

func CompetenciaDe(t time.Time) Competencia {
	return Competencia(t.Format(formatoCompetencia))
}

func (c Competencia) Valida() error {
	if _, err := time.Parse(formatoCompetencia, string(c)); err != nil {
		return fmt.Errorf("competência %q inválida: use o formato AAAA-MM", string(c))
	}
	return nil
}

// Intervalo devolve [inicio, fim) da competência no fuso local do servidor.
//
// O fim é o primeiro instante do mês seguinte, e a comparação a jusante é
// `< fim`, nunca `<= ultimoDia`: usar o último dia como limite superior perde
// tudo que acontece depois de 00:00:00 do dia 31.
func (c Competencia) Intervalo() (inicio, fim time.Time, err error) {
	t, parseErr := time.ParseInLocation(formatoCompetencia, string(c), time.Local)
	if parseErr != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("competência %q inválida: use o formato AAAA-MM", string(c))
	}
	inicio = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.Local)
	fim = inicio.AddDate(0, 1, 0)
	return inicio, fim, nil
}

// FaixasRFC3339 devolve o intervalo em string nos dois offsets que o Mongo
// encontra quando autorizadoEm foi gravado com Format(RFC3339).
//
// Range lexicográfico só casa quando o bound usa o MESMO offset do valor
// persistido: "2026-09-13T11:56:00Z" não compara com
// "2026-10-01T00:00:00-03:00" da forma que um instante compara. O filtro a
// jusante aplica as duas faixas (local e UTC) e, quando o campo é Date, a
// comparação nativa de time.Time.
func FaixasRFC3339(inicio, fim time.Time) (localIni, localFim, utcIni, utcFim string) {
	return inicio.Format(time.RFC3339), fim.Format(time.RFC3339),
		inicio.UTC().Format(time.RFC3339), fim.UTC().Format(time.RFC3339)
}

// RespostaContagem é o corpo devolvido pelas rotas internas de contagem.
type RespostaContagem struct {
	Quantidade int64 `json:"quantidade"`
}
