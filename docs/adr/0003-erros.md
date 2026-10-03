# ADR 0003 — Erros tipados e tradução HTTP

Situação: aceita para piloto (tarefa-service, 2026-10-03).

## Contexto
`resposta` inferia 404/409 por trecho de mensagem e o resto virava 400; use
cases concatenavam `err.Error()` à mensagem pública. Mudar uma frase mudava o
protocolo e falha de infraestrutura parecia erro do usuário.

## Decisão
- Pacote de classificação sem HTTP (`apperr` no piloto): `Tipo` (inválido, não
  encontrado, conflito, proibido), `Codigo` estável, `Mensagem` pública,
  `Causa` preservada para `errors.Is/As`.
- Repositório traduz o "não achou" do driver para sentinela de domínio
  (`repositories.ErrTarefaNaoEncontrada`); use case classifica.
- Handler mapeia tipo → status num ponto só. Erro não classificado vira 500 com
  mensagem genérica e log da causa; a causa nunca vai ao cliente.
- Envelope `{sucesso, mensagem, conteudo}` não muda. Expor `codigo` no envelope
  depende de acordo com web e mobile.

## Extração
Vai para `pkg/apperr` com o segundo serviço adotante. `resposta.StatusDoErro`
por texto fica para o legado até lá.
