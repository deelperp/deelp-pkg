# Compatibilidade dos consumidores

O inventário em [consumers.json](../.github/consumers.json) contém os 16 serviços Go
que dependem de `deelp-pkg`. Inclui marketing e NFS-e; WhatsApp é Node e não
consome este módulo. Adicione um novo consumidor ao inventário junto de sua
primeira adoção do pacote.

O workflow `quality.yml` executa duas verificações distintas:

- `quality`: testa o pacote isoladamente com `GOWORK=off`, `go vet` e race detector.
- `Compatibilidade dos consumidores`: exige testes de todos os consumidores
  contra o candidato, em até dois serviços simultâneos. Cada job registra os
  commits do candidato e da branch `main` do consumidor.

O candidato substitui a versão publicada somente em um `go.work` temporário com
dois módulos: pacote e serviço. O script confirma que Go resolveu o pacote local,
usa `-mod=readonly` e remove o workspace ao terminar. Os `go.mod`/`go.sum` dos
repositórios permanecem intactos. A seleção de dependências é feita por par:
outros serviços do workspace de desenvolvimento não podem mascarar conflitos.

## Execução local

Requisitos: Python 3, Git, toolchain declarada nos módulos e acesso às dependências.
Os testes fiscais usam `xmllint` (no Ubuntu, pacote `libxml2-utils`). Testes com
servidores locais precisam de permissão para abrir sockets.

Na raiz de `deelp-pkg`, com os consumidores em diretórios irmãos:

```sh
python3 scripts/check_consumer.py ../tarefa-service
python3 scripts/check_consumer.py ../tarefa-service --build-only
python3 - <<'PY'
import json
import subprocess
from pathlib import Path

for service in json.loads(Path('.github/consumers.json').read_text()):
    subprocess.run(['python3', 'scripts/check_consumer.py', '../' + service], check=True)
PY
```

Sem `--build-only`, cada pacote tem timeout de 90 segundos e testes sem cache de
resultado (`-count=1`). O script propaga o código de falha; não ignora pacotes nem
transforma falhas conhecidas em sucesso. `--build-only` não substitui o gate de CI.

## Configuração no GitHub

Crie o secret `GH_PAT_DEELP_CONSUMERS` no repositório `deelp-pkg`: token de leitura
com `Contents: read` apenas nos 16 repositórios do inventário. O token padrão de
Actions não dá acesso aos outros repositórios privados. O checkout usa
`persist-credentials: false`, e o token não é passado aos comandos Go.

Sem o secret, o gate falha com diagnóstico explícito. PRs de forks não recebem
o secret; para validar esses candidatos, revise o conteúdo e leve-o a uma branch
interna. O workflow não usa `pull_request_target` nem executa código de fork com
credenciais privilegiadas. As permissões gerais ficam em `contents: read`.

Configure os checks `quality` e `Compatibilidade dos consumidores` como
obrigatórios na proteção de `main`. A criação dos arquivos não configura secrets
nem proteção de branches no GitHub.

A matriz avalia o código atual dos serviços, por isso mudanças independentes em
`main` podem falhar. Use os SHAs registrados para reproduzir e distinguir regressão
do candidato de falha preexistente. Após publicar uma tag, os serviços ainda devem
atualizar a dependência e testar a versão publicada com `GOWORK=off`; a matriz não
substitui essa validação nem testes de integração com infraestrutura real.

## Arquitetura dos consumidores

```sh
go run ./cmd/deelp-arquitetura -raiz ../tarefa-service -baseline .github/arquitetura/tarefa-service.txt
go run ./cmd/deelp-arquitetura -raiz ../tarefa-service -gerar-baseline
```

Ao corrigir uma violação, remova a linha do baseline no mesmo PR do pacote; o
modo `-estrito` reprova linhas que já não correspondem a nada.

## Integração com RabbitMQ

```sh
DEELP_RABBITMQ_URL=amqp://guest:guest@127.0.0.1:5672/ go test -race -run Integracao ./mensageria/...
```
