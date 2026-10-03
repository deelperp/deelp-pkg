# ADR 0002 — Nomenclatura

Situação: aceita (2026-10-03). Vale para código novo e para código alterado por
motivo funcional; não há renomeação global.

| Tema | Regra |
|---|---|
| Negócio | Português (`Produto`, `OrdemServico`, `Mensagem`, `Publicar`). Vocabulário fiscal canônico do `CLAUDE.md`. |
| Mecanismo técnico | Inglês quando é termo técnico consolidado (`Open`, `Close`, `Config`, `closers`, `ttlcache`). |
| Construtor | `New`/`Novo` constrói sem efeito externo; quem abre recurso recebe `context` e devolve erro (`Open`, `AbrirRabbitMQ`). Provisionamento é método explícito (`ConfigurarCORS`). |
| Interface | Definida no consumidor, nomeada pelo papel (`ProvedorCanal`). Prefixo `I` só no legado. |
| Pacote | Curto, minúsculo, sem underscore. Subpacote não repete o pai (`tenantdados/core`, não `tenantdadoscore`). Evitar colidir com stdlib (`transporte`, não `http`). |
| Use case | Sufixo `UseCase` em tipos, campos e variáveis; nunca `UC`. |
| Path param | Variável local com o mesmo nome do parâmetro. |
| Erro | Sentinela `Err...`; erro classificado carrega código estável e causa. |
| Contrato externo | JSON/BSON/SQL e `empresaId` não mudam por causa desta ADR. |
