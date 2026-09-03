# http-server-projeto-korp

Serviço HTTP em Go atrás de um proxy reverso NGINX, com Prometheus e Grafana,
provisionado por Ansible em um único comando. Desenvolvido para o desafio
técnico de Analista DevOps.

## Arquitetura

```
                          host
                           :80
                            │
              ┌─────────────▼──────────────┐
              │  nginx                     │   única porta publicada
              │  proxy reverso → :8080     │
              └─────────────┬──────────────┘
                            │
    ════════════════════════╪════════════════════  rede bridge korp-net
                            │
              ┌─────────────▼──────────────┐
              │  http-server-projeto-korp  │   sem portas expostas ao host
              │  :8080                     │
              │  GET /projeto-korp         │
              │  GET /health  GET /ready   │
              │  GET /metrics              │
              └─────────────┬──────────────┘
                            │ scrape a cada 15s
              ┌─────────────▼──────────────┐
              │  prometheus :9090          │
              └─────────────┬──────────────┘
                            │ query
              ┌─────────────▼──────────────┐
              │  grafana :3000             │   datasource e dashboard
              └────────────────────────────┘   provisionados como código
```

A aplicação não publica portas no host. Só o NGINX e o Prometheus a
alcançam, de dentro da rede `korp-net`.

O ambiente foi validado provisionando uma VM Debian 13 via SSH a partir de
um control node separado. As mesmas roles funcionam contra `localhost`,
bastando ajustar o inventário.

## Estrutura

```
app/                    aplicação Go, testes, Dockerfile multi-stage
docker/                 docker-compose.yml, nginx, prometheus, grafana
ansible/                site.yml, inventário e roles docker / korp_stack / validate
ansible/group_vars/all/ vars.yml e vault.yml (senha do Grafana, criptografada)
preflight.sh            checa pré-condições antes do deploy
verify.sh               aceite contra os requisitos do desafio
Makefile                atalhos (make help)
```

## Pré-requisitos

No control node: Ansible Core >= 2.15 (a role do Docker usa
`deb822_repository`) e as coleções de `ansible/requirements.yml`
(`make deps`). Go só é necessário para rodar os testes fora do container.

No host alvo: Python 3, SSH por chave e sudo. O Docker é instalado pelo
playbook a partir do repositório oficial. Suporta Debian/Ubuntu e
Rocky/Alma/CentOS.

## Execução

```bash
make deps      # coleções Ansible
make deploy    # ansible-playbook site.yml
```

O playbook instala o Docker, cria a rede bridge, constrói a imagem, sobe os
quatro containers e termina com uma requisição HTTP de validação impressa
no console.

O alvo fica em `ansible/inventory/hosts.ini`. Para provisionar a própria
máquina, troque a linha do host por `localhost ansible_connection=local`.

### Executando a partir de um clone

A senha do Grafana está criptografada em `ansible/group_vars/all/vault.yml`,
e a senha que abre esse arquivo (`ansible/vault-pass.txt`) não é versionada.
Quem clonar o repositório precisa fornecer a sua própria:

```bash
cd ansible
echo "uma-senha-qualquer" > vault-pass.txt && chmod 600 vault-pass.txt
rm group_vars/all/vault.yml
printf -- '---\nvault_korp_grafana_password: "escolha-uma-senha"\n' > /tmp/v.yml
ansible-vault encrypt /tmp/v.yml --output group_vars/all/vault.yml \
  --vault-password-file vault-pass.txt
shred -u /tmp/v.yml
```

Sem Ansible, só com Docker:

```bash
make up      # cria a rede e sobe a stack
make smoke   # curl http://localhost:80/projeto-korp
```

## Verificação

`./preflight.sh` roda antes do deploy: versão do Ansible, coleções, SSH e
sudo, relógio sincronizado, portas livres e alcance DNS/HTTPS aos registries
que a build usa (Docker Hub, gcr.io, proxy.golang.org).

`./verify.sh` roda depois: percorre os itens do enunciado um a um, incluindo
os fáceis de esquecer (aplicação sem portas publicadas, imagem oficial do
NGINX, volume em `/etc/nginx/conf.d/`, rede com driver `bridge`, horário
mudando entre duas requisições). As checagens do Grafana são autenticadas de
verdade, com a credencial lida do `.env` do host alvo — foi isso que expôs a
divergência descrita em "Problemas encontrados".

| Demonstração | Como | O que prova |
|---|---|---|
| Idempotência | `make deploy` duas vezes | a segunda termina com `changed=0` |
| Reprodutibilidade | snapshot limpo da VM + `make deploy` | reconstrói tudo do zero |

## Dashboard

![Dashboard do Grafana](docs/img/grafana-dashboard.png)

Três sinais de disponibilidade: `up` diz se o alvo responde ao scrape, a
média de 6h dá o tempo no ar, e a taxa de sucesso mostra o que o cliente
recebeu. Um serviço pode estar `up` e ainda assim devolver erro para todo
mundo.

O JSON é gerado por `docker/grafana/build_dashboard.py`, não exportado da
interface.

## Métricas

| Métrica | Tipo | Uso |
|---------|------|-----|
| `up{job="http-server-projeto-korp"}` | gerada pelo Prometheus | o alvo respondeu ao scrape? |
| `korp_http_requests_total{method,path,status}` | counter | volume de requisições |
| `korp_http_request_duration_seconds` | histogram | latência por percentil |
| `korp_http_requests_in_flight` | gauge | concorrência |
| `korp_uptime_seconds` | gauge | detecta restart / crash loop |
| `korp_service_healthy` | gauge | saúde reportada pela aplicação |
| `korp_build_info` | gauge (info) | versão e commit em execução |
| `go_*`, `process_*` | vários | memória, GC, goroutines, CPU |

```promql
# disponibilidade nas últimas 6 horas
avg_over_time(up{job="http-server-projeto-korp"}[6h]) * 100

# requisições por segundo, por rota
sum by (path) (rate(korp_http_requests_total[5m]))

# latência p95
histogram_quantile(0.95, sum by (le) (rate(korp_http_request_duration_seconds_bucket[5m])))
```

## Decisões

**Aplicação.** Só `net/http`, sem framework. O horário é lido a cada
requisição com `time.Now().UTC()`; o relógio é injetado no handler para os
testes congelarem o tempo. Shutdown gracioso em SIGTERM e timeouts
explícitos no servidor.

**Cardinalidade.** Os labels `path` e `method` recebem valores de listas
fechadas (o padrão da rota registrada, e um conjunto fixo de métodos), nunca
o que veio na requisição. `/metrics` fica fora do middleware para o scrape
não contar como tráfego.

**Imagem.** Build multi-stage com `go vet` e `go test` dentro do build;
runtime `distroless/static:nonroot`, 5,9 MB, sem shell. O healthcheck é um
segundo binário Go, já que não há curl nem wget na imagem.

**Compose.** Rede `external`, criada pelo Ansible. `depends_on` com
`service_healthy` porque o NGINX resolve o upstream na inicialização.
Healthchecks em `127.0.0.1`, não `localhost` (ver abaixo). `no-new-privileges`,
`cap_drop: ALL`, `read_only` e rotação de log.

**NGINX.** `/metrics` bloqueado na borda: o Prometheus coleta direto na
8080. `X-Forwarded-For` e `X-Real-IP` para a aplicação enxergar o IP real.
Keepalive no upstream e rate limit de 10 req/s por IP.

**Ansible.** Roles separadas (`docker`, `korp_stack`, `validate`) com tags.
Docker CE do repositório oficial via `deb822_repository`. A imagem é
construída com `docker_image_build` (buildx), porque o Dockerfile usa
`RUN --mount=type=cache`. A estrutura no host alvo espelha a do repositório,
para os caminhos relativos do compose resolverem igual. A validação confere
o contrato, exige o sufixo `Z` no horário e faz duas requisições com 2 s de
intervalo para provar que o valor é dinâmico.

**Segredos.** A senha do Grafana fica em `group_vars/all/vault.yml`,
criptografada com `ansible-vault`. O arquivo vai para o repositório; a senha
que o abre não — está no `.gitignore` e é referenciada pelo
`vault_password_file` no `ansible.cfg`, o que mantém o deploy como um único
comando, sem prompt. O `vars.yml` referencia `{{ vault_korp_grafana_password }}`
em vez de conter o valor: a indireção com prefixo `vault_` permite achar
onde cada segredo é usado com um `grep`, sem descriptografar nada. Os
defaults das roles não definem a senha, então o playbook falha com
"undefined variable" se `group_vars` não fornecer — melhor que subir em
silêncio com credencial fraca. As tasks que autenticam no Grafana usam
`no_log`, e o resumo final exibe só o usuário: sem isso a senha apareceria
no console durante a demonstração.

## Problemas encontrados

**`--mount` requer BuildKit.** O módulo `docker_image` usa a API clássica do
Engine, cujo builder não entende `RUN --mount`. Na linha de comando passa
despercebido porque `docker build` já usa buildx. Trocado por
`docker_image_build`.

**Contexto de build fora do lugar.** O compose declara `build.context: ../app`.
A role copiava o compose para a raiz de `/opt/projeto-korp`, onde `../app`
não existe. Corrigido espelhando a estrutura do repositório no host.

**NGINX saudável, healthcheck falhando.** `wget http://localhost/nginx-health`
dava `Connection refused` dentro do container, enquanto `127.0.0.1`
funcionava. `localhost` também resolve para `::1`, e `listen 80` só faz
bind em IPv4. Prometheus e Grafana passavam por serem em Go, que faz bind
dual-stack. Todas as sondas passaram a usar `127.0.0.1`.

**Painéis stat em branco.** O redutor estava escrito `lastNonNull`; o
identificador correto é `lastNotNull`. O Grafana ignora a string errada em
silêncio, e o sparkline continuava desenhando porque usa a série crua.

**Callback do Ansible ausente.** `stdout_callback = yaml` depende de
`community.general`, que não estava nas coleções instaladas. Com ansible-core
puro o playbook abortava antes da primeira task. Trocado pelo callback
default com `result_format = yaml`.

**Restart do Docker depois da validação.** O handler que reinicia o daemon
após escrever o `daemon.json` só rodava no fim do play, derrubando os
containers logo depois da mensagem de sucesso. Um `flush_handlers` após a
task resolve.

**Estado em volume não se reconcilia com o código.** Depois de mover a senha
do Grafana para o cofre e reimplantar, a autenticação continuou falhando —
nem com a senha nova, nem com a antiga. `GF_SECURITY_ADMIN_PASSWORD` só é
aplicada quando o Grafana **cria** o banco; o volume `grafana-data` já
existia com o usuário admin gravado, e a senha havia sido alterada pela
interface num acesso anterior. O contraste é o argumento central a favor de
provisionamento como código: o datasource e o dashboard, declarados em
arquivo, voltaram idênticos a cada deploy; a senha, que vive só no volume,
divergiu em silêncio. Corrigido recriando o volume. Só apareceu porque o
`verify.sh` autentica de verdade — uma checagem que apenas perguntasse "o
Grafana responde?" teria dado verde o tempo todo.

## Limitações

- `sudo` sem senha para o usuário de automação, num host de laboratório
  isolado. Em produção, escopo restrito no `sudoers` ou credencial em cofre.
- A senha do Grafana não é reconciliada pelo playbook: ela vale na criação
  do banco, e alterações feitas pela interface sobrescrevem o valor
  declarado sem que uma nova execução as reverta. Reconciliar exige recriar
  o volume `grafana-data` ou usar `grafana-cli admin reset-admin-password`
  numa task condicional.
- O `.env` no host tem modo `0600` e dono root, por conter a senha. Comandos
  `docker compose` executados manualmente no host precisam de `sudo` para
  lê-lo, mesmo com o usuário no grupo `docker` — permissão do daemon e
  permissão de arquivo são coisas diferentes. O playbook não é afetado
  porque roda com `become`.
- Sem TLS, sem Alertmanager, sem alta disponibilidade. As regras de alerta
  existem e aparecem em `:9090/alerts`, mas não notificam ninguém.
- Prometheus com armazenamento local e retenção de 15 dias.

## Comandos

```
make help       lista todos os alvos
make test       testes unitários
make deps       coleções Ansible
make check      syntax-check do playbook
make deploy     provisiona o ambiente inteiro
make up         sobe a stack só com Docker
make smoke      executa o teste do desafio
make load       gera tráfego para o dashboard
make destroy    remove tudo

./preflight.sh  pré-condições antes do deploy
./verify.sh     aceite contra os requisitos
```
