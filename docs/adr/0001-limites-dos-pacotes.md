# ADR 0001 — Limites do deelp-pkg

Situação: aceita (2026-10-03).

## Contexto
O `pkg` cresceu para 17 pacotes; parte é infraestrutura técnica, parte é
capacidade de negócio compartilhada (`assinatura`, `consumo`, `dfe`). O
`tenantdados` reunia contrato, HTTP e três drivers no mesmo pacote.

## Decisão
- Módulo e versionamento únicos por enquanto.
- Pacote novo só entra com consumidor real em dois serviços. O piloto acontece
  no serviço (ex.: `apperr` e encerramento em pilha no tarefa-service) e é
  extraído quando o segundo consumidor aparecer.
- Núcleo sem driver: quando um pacote tem contrato e adapters, o contrato fica
  num subpacote que não importa driver nem `net/http`, travado por teste
  (`tenantdados/core`).
- API antiga continua por fachada (aliases e delegação) até a adoção acabar;
  não manter duas implementações da mesma regra.
- `pkg` nunca importa serviço.

## Consequências
Serviço novo importa núcleo + adapter que usa, sem arrastar drivers. A fachada
tem custo de manutenção e sai na versão major seguinte à adoção completa.
