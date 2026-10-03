# ADR 0004 — Ciclo de vida do processo

Situação: aceita para piloto (tarefa-service, 2026-10-03).

## Decisão
- `main` chama `run() error` e só ele decide o código de saída; nada de
  `log.Fatal` depois de abrir recurso.
- Cada recurso aberto registra seu fechamento numa pilha (LIFO). Falha no meio
  da inicialização fecha o que já abriu.
- Worker em segundo plano devolve canal de término; o fechamento cancela e
  espera o término antes de fechar banco e broker.
- Orçamentos separados para drenar HTTP, fechar dependências e descarregar
  telemetria, somando menos que o `terminationGracePeriodSeconds`.
- Versão real na telemetria por `-ldflags "-X main.versao=..."`.

## Extração
`pkg/app` com o segundo adotante, mantendo DI manual no `container.go`.
