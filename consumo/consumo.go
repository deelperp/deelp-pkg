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

// RespostaContagem é o corpo devolvido pelas rotas internas de contagem.
type RespostaContagem struct {
	Quantidade int64 `json:"quantidade"`
}
