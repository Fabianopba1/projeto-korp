# Registro de Execução

Registro das etapas executadas durante o projeto, com a saída real dos
comandos. Complementa o `README.md` (o que foi construído) e o
`01-rastreabilidade-requisitos.md` (por que foi construído assim) com a
evidência de que cada etapa foi de fato executada e validada.

**Ambiente**

| Item | Valor |
|---|---|
| Control node | Debian 13 — notebook |
| Hipervisor | Proxmox VE — mini PC Intel i3-10100T |
| VM alvo | `korp-devops`, VMID `610`, Debian 13.6 (trixie) |
| Usuário no alvo | `korp` |
| Rede | reserva de DHCP por MAC, IP fixo em VLAN de laboratório |
| Repositório | `https://github.com/Fabianopba1/projeto-korp` |

---

## Fase 0 — Control node

```
# ansible --version
ansible [core 2.19.4]

# go version
go version go1.24.4 linux/amd64
```

O `ansible-core` precisa ser >= 2.15: a role do Docker usa
`deb822_repository`, que não existe em versões anteriores. O Go no control
node serve apenas para rodar os testes fora do container — a build da imagem
não depende dele.

```
# make check
cd ansible && ansible-playbook site.yml --syntax-check
playbook: site.yml
```

```
# make deps
cd ansible && ansible-galaxy collection install -r requirements.yml
Starting galaxy collection install process
Nothing to do. All requested collections are already installed.
```

```
# make test
cd app && go test ./... -v -count=1
?   github.com/korp/http-server-projeto-korp             [no test files]
?   github.com/korp/http-server-projeto-korp/cmd/healthcheck  [no test files]
--- PASS: TestProjetoKorpRetornaContratoEsperado (0.00s)
--- PASS: TestHorarioEstaEmUTC (0.00s)
--- PASS: TestHorarioEhResolvidoACadaRequisicao (0.00s)
--- PASS: TestMetodoNaoPermitido (0.00s)
--- PASS: TestHealthEReady (0.00s)
ok  github.com/korp/http-server-projeto-korp/internal/handlers  0.003s
--- PASS: TestNormalizeMethod (0.00s)
--- PASS: TestMiddlewareAgrupaMetodosDesconhecidos (0.00s)
ok  github.com/korp/http-server-projeto-korp/internal/metrics   0.003s
```

Sete testes, dois pacotes. O `-count=1` desativa o cache do Go: sem ele, uma
segunda execução imprimiria `(cached)` sem rodar nada.

---

## Fase 1 — Hipervisor

Liberação de espaço no Proxmox antes de criar a VM alvo. A VM anterior foi
salva com `vzdump` (arquivo independente, restaurável em um VMID novo) antes
de ser destruída — snapshot não serviria, porque no `local-lvm` os snapshots
são volumes filhos do disco da VM e `qm destroy --purge` os remove junto.

<!-- PREENCHER: VMID removido, nome e tamanho do arquivo de backup, saída de `lvs` -->

---

## Fase 2 — VM alvo

VM criada com 2 vCPU (type `host`), 4 GB de RAM sem ballooning, disco de
32 GB em `local-lvm` e QEMU Guest Agent habilitado. Instalação mínima do
Debian: apenas SSH server e utilitários padrão.

Três ajustes necessários antes do primeiro playbook, todos causas de falha
pouco óbvia: instalar `sudo` (não vem quando se define senha de root no
instalador), instalar `python3` (sem ele o Ansible não executa módulo algum)
e instalar o `qemu-guest-agent` dentro da VM (habilitar no Proxmox não basta).

Snapshot `base-limpa` tirado com a VM desligada, antes de qualquer execução
do projeto e antes da primeira conexão do VS Code Remote SSH — que instala o
`vscode-server` na VM e alteraria o estado capturado.

```
# ssh korp@<ip> "sudo -n true && echo 'sudo ok'"
sudo ok

# ansible korp -m ping
korp-devops | SUCCESS => { "ping": "pong" }
```

<!-- PREENCHER: confirmar as duas saídas acima e a data do snapshot -->

### Preflight

`./preflight.sh` — 48 verificações, 0 falhas. Cobre versão do `ansible-core`,
coleções instaladas, permissão da chave SSH, sudo sem senha, distribuição e
codename, sincronia de relógio entre control node e alvo, portas livres
(80, 3000, 8080, 9090), ausência de Docker e de servidor web no host, e
alcance DNS/HTTPS aos registries usados na build.

---

## Fase 3 — Parte 1 do desafio

```
# docker network inspect korp-net -f '{{.Driver}}'
bridge
```

```
# docker compose ps
NAME                       IMAGE                            STATUS                    PORTS
grafana-projeto-korp       grafana/grafana:11.1.0           Up (healthy)              0.0.0.0:3000->3000/tcp
http-server-projeto-korp   http-server-projeto-korp:1.0.0   Up (healthy)              8080/tcp
nginx-projeto-korp         nginx:1.27-alpine                Up (healthy)              0.0.0.0:80->80/tcp
prometheus-projeto-korp    prom/prometheus:v2.53.0          Up (healthy)              127.0.0.1:9090->9090/tcp
```

A coluna PORTS da aplicação traz `8080/tcp` sem prefixo de endereço: a porta
é exposta na rede Docker, mas não publicada no host. É a evidência direta do
requisito "não deve expor portas diretamente ao host".

```
# curl -i http://<ip-da-vm>/projeto-korp
HTTP/1.1 200 OK
Server: nginx
Content-Type: application/json; charset=utf-8
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
Referrer-Policy: no-referrer

{"nome":"Projeto Korp","horario":"2026-09-03T03:28:23Z"}
```

`Server: nginx` sem número de versão confirma `server_tokens off`.

```
# make smoke
--> curl http://localhost:80/projeto-korp
{"nome":"Projeto Korp","horario":"2026-09-03T04:06:02Z"}
OK: contrato valido, horario em UTC
```

**Ordem de subida.** Os quatro containers foram criados no mesmo instante,
mas subiram escalonados: aplicação e Prometheus imediatamente, NGINX ~11 s
depois e Grafana ~17 s depois. O atraso é o `depends_on` com
`condition: service_healthy` esperando o healthcheck da dependência passar —
startup determinístico, não `depends_on` simples.

**Checklist**

- [x] Rede em modo `bridge`
- [x] Aplicação sem portas publicadas no host
- [x] JSON com os campos `nome` e `horario`
- [x] `horario` em UTC, com sufixo `Z`
- [x] `horario` muda entre requisições
- [x] Porta 8080 inacessível a partir do host

---

## Fase 4 — Parte 2 do desafio

### Targets do Prometheus

```
http-server-projeto-korp (1/1 up)
  http://http-server-projeto-korp:8080/metrics   UP
  labels: camada="aplicacao" instance="http-server-projeto-korp:8080"
          job="http-server-projeto-korp" servico="http-server-projeto-korp"

prometheus (1/1 up)
  http://localhost:9090/metrics                  UP
  labels: camada="monitoramento" instance="localhost:9090" job="prometheus"
```

O scrape da aplicação usa o nome do serviço, resolvido pelo DNS interno da
rede `korp-net` — não passa pelo NGINX. O Prometheus também monitora a si
mesmo, evitando o ponto cego de descobrir que a coleta parou só quando
alguém reclama.

| Consulta | Resultado |
|---|---|
| `up{job="http-server-projeto-korp"}` | `1` |
| `sum(korp_http_requests_total)` | > 0 após `make load` |

**Evidências**

- [x] `docs/img/targets.png` — página de targets do Prometheus
- [x] `docs/img/grafana-dashboard.png` — dashboard populado após `make load`
- [x] `docs/img/grafana-datasource.png` — datasource provisionado por arquivo

**Checklist**

- [x] Todos os targets `up`
- [x] Datasource e dashboard aparecem sem intervenção manual
- [x] Painéis com dados reais

---

## Fase 5 — Parte 3 do desafio

### Execução do playbook

```
PLAY RECAP *********************************************************************
korp-devops : ok=49  changed=0  unreachable=0  failed=0  skipped=4  rescued=0  ignored=0

PLAYBOOK RECAP *****************************************************************
Playbook run took 0 days, 0 hours, 0 minutes, 33 seconds
```

Log completo em `docs/logs/deploy-idempotencia.log`.

| Métrica | Valor |
|---|---|
| Tempo total | 33 s (execução idempotente, sem rebuild) |
| `ok` | 49 |
| `changed` | **0** |
| `failed` | 0 |

`changed=0` na segunda execução prova que nenhuma task é imperativa
disfarçada de declarativa. Foi confirmado também **depois** das alterações
de segurança (cofre e bind em loopback), o que mostra que elas não
introduziram nenhuma task não idempotente.

### Versões instaladas pelo playbook

```
Docker version 29.7.2, build a7dcaa6
Docker Compose version v5.5.0
SDK Docker (Python): 7.1.0  (mínimo exigido: 7.0.0)
Rede 'korp-net' pronta (driver: bridge, subnet: 172.28.0.0/16)
```

### Resposta HTTP exibida pelo próprio playbook

```
==================================================================
  GET http://localhost:80/projeto-korp   (executado em korp-devops)
==================================================================
  HTTP 200
  Content-Type: application/json; charset=utf-8

  {"nome":"Projeto Korp","horario":"2026-09-03T05:27:30Z"}

  nome    : Projeto Korp
  horario : 2026-09-03T05:27:30Z  (UTC)
==================================================================
```

```
  AMBIENTE PROJETO KORP
==================================================================
  Aplicacao via NGINX  : OK
  Contrato JSON        : OK (nome + horario em UTC)
  Horario dinamico     : OK (2026-09-03T05:27:30Z -> 2026-09-03T05:27:33Z)
  /metrics protegido   : OK (bloqueado na borda)
  Prometheus coletando : OK
  Grafana              : ok
  Datasource           : provisionado
  Dashboard            : provisionado
==================================================================
```

O horário mudou entre as duas requisições feitas com 2 s de intervalo,
provando que o valor é resolvido a cada chamada e não em uma variável de
pacote.

### Estado final dos containers

```
/http-server-projeto-korp | running | healthy
/nginx-projeto-korp       | running | healthy
/prometheus-projeto-korp  | running | healthy
/grafana-projeto-korp     | running | healthy
```

### Aceite

```
===============================================================
  RESULTADO: 32 OK / 0 falhas
===============================================================
```

`./verify.sh` percorre os requisitos do enunciado um a um, autenticando de
verdade no Grafana com a credencial lida do `.env` do host alvo.

**Checklist**

- [x] Um único comando provisiona o ambiente
- [x] Segunda execução com `changed=0`
- [x] Playbook exibiu a resposta HTTP no console
- [x] Grafana de pé sem toque manual
- [x] Logs salvos em `docs/logs/`
- [ ] Rollback do snapshot + deploy do zero <!-- PREENCHER após executar -->

---

## Problemas encontrados e soluções

Nove defeitos reais encontrados durante o projeto. Os sete primeiros durante
o desenvolvimento e estão descritos em detalhe no `README.md`; os dois últimos
foram revelados pelo teste destrutivo documentado adiante. A tabela resume
causa raiz e correção.

| # | Problema | Causa raiz | Solução |
|---|---|---|---|
| 1 | `RUN --mount=type=cache` falhava na build via Ansible, mas funcionava na linha de comando | `docker_image` usa a API clássica do Engine, cujo builder legado não entende `--mount`. O `docker build` do CLI já usa buildx por padrão, o que mascarava o problema | Trocado para `docker_image_build`, que invoca `docker buildx` |
| 2 | `build.context: ../app` não resolvia no host alvo | A role copiava o compose para a raiz de `/opt/projeto-korp`, onde `../app` aponta para fora do projeto | A árvore no host alvo passou a espelhar a do repositório (`/opt/projeto-korp/docker/`) |
| 3 | Healthcheck do NGINX com `Connection refused`, apesar de o serviço responder | `localhost` resolve também para `::1` dentro do container, e `listen 80` faz bind apenas em IPv4. Prometheus e Grafana não sofriam o mesmo por serem em Go, que faz bind dual-stack | Todas as sondas passaram a usar `127.0.0.1` explicitamente |
| 4 | Painéis `stat` do Grafana em branco, sem erro | O redutor foi escrito como `lastNonNull`; o identificador válido é `lastNotNull`. O Grafana ignora a string inválida em silêncio, e o sparkline continuava desenhando por usar a série crua | Corrigido o identificador; consultas trocadas de `instant` para intervalo e `textMode: "value"` |
| 5 | Playbook abortava antes da primeira task | `stdout_callback = yaml` depende da coleção `community.general`, ausente no ambiente | Callback default com `result_format = yaml` no `ansible.cfg` |
| 6 | Containers caíam logo após a mensagem de sucesso | O handler que reinicia o daemon Docker após escrever o `daemon.json` só executa no fim do play, derrubando a stack já validada | `flush_handlers` logo após a task de configuração do daemon |
| 7 | Autenticação no Grafana falhava com a senha nova **e** com a antiga | `GF_SECURITY_ADMIN_PASSWORD` só é aplicada quando o Grafana cria o banco. O volume `grafana-data` já existia com o usuário admin gravado, e a senha havia sido alterada pela interface num acesso anterior | Volume recriado. O contraste é o argumento central a favor de provisionamento como código: datasource e dashboard, declarados em arquivo, voltaram idênticos a cada deploy; a senha, que vive só no volume, divergiu em silêncio |

| 8 | O check "prometheus coletando as métricas" ficava verde com a aplicação parada | `grep '"health":"up"'` varria a resposta inteira da API de targets; o self-scrape do próprio Prometheus satisfazia o padrão | Passou a filtrar pelo job `http-server-projeto-korp` e a reportar o estado real (`down`, `target-ausente`, `resposta-invalida`) |
| 9 | Com a aplicação parada, o script acusava `horario divergente em 57693s` | Sem resposta, `$H2` fica vazio e `date -u -d ""` devolve a meia-noite de hoje; a diferença medida eram só as horas do dia | Guarda explícita para `$H2` vazio, com mensagem "sem horário para comparar" |

Os problemas 8 e 9 têm a mesma raiz metodológica: as duas checagens nunca
haviam sido executadas contra um ambiente quebrado. O 7 só apareceu porque o
`verify.sh` autentica de verdade — uma checagem que apenas perguntasse "o
Grafana responde?" teria dado verde o tempo todo.

---

## Validação das regras de alerta

Validação estática, com o `promtool` que já vem na imagem do Prometheus:

```
# docker exec prometheus-projeto-korp promtool check config /etc/prometheus/prometheus.yml
Checking /etc/prometheus/prometheus.yml
  SUCCESS: 1 rule files found
 SUCCESS: /etc/prometheus/prometheus.yml is valid prometheus config file syntax

Checking /etc/prometheus/alerts.yml
  SUCCESS: 6 rules found
```

Estado das seis regras com a stack saudável — todas `inactive`, nos dois
grupos (`disponibilidade` e `trafego_e_erros`):

```
ServicoIndisponivel           inactive
DisponibilidadeAbaixoDoSLO    inactive
ServicoReiniciouRecentemente  inactive
TaxaDeErro5xxAlta             inactive
LatenciaP95Alta               inactive
SemTrafego                    inactive
```

### Um alerta que não consegue disparar

`promtool` confirma sintaxe, não semântica. Percorrendo cada regra com a
pergunta "sob quais condições isto ficaria verdadeiro?", a `SemTrafego`
falhou no teste. Medição no ambiente em repouso, sem tráfego de usuário há
horas:

```
# sum by (path) (rate(korp_http_requests_total[10m]))
/health          0.1009 req/s
/projeto-korp    0.0000 req/s
other            0.0000 req/s

# sum by (path) (korp_http_requests_total)
/health          22587
/projeto-korp    644
other            83
```

A expressão `sum(rate(korp_http_requests_total[10m])) == 0` agrega todas as
rotas, e o healthcheck do Docker bate em `/health` a cada 10 segundos —
0,1 req/s constantes. A soma nunca chega a zero e o alerta nunca dispara.

Registrado como limitação em `01-rastreabilidade-requisitos.md`, seção E.6,
com a correção identificada (`path!="/health"` na expressão) e o motivo de
não tê-la aplicado às vésperas da entrega.

O rótulo `other` na terceira linha é a proteção de cardinalidade em ação: as
requisições do `make load` para rotas inexistentes caem num balde fixo em vez
de criar uma série nova por URL.

---

## Teste destrutivo — provando que as verificações falham

Uma verificação que passa sempre não é verificação. O `verify.sh` acumulava
32 checagens verdes, mas nenhuma delas havia sido vista em vermelho. O ciclo
abaixo derruba a aplicação de propósito para responder a uma pergunta
objetiva: **as checagens conseguem acusar a falha?**

### Estado inicial

Stack saudável, `verify.sh` em 32 OK / 0 falhas, as seis regras de alerta em
`inactive`.

### A quebra

```
# docker stop http-server-projeto-korp
http-server-projeto-korp
```

Dois sintomas diferentes para a mesma falha, dependendo de onde a requisição
nasce:

```
# do control node
# curl -i --max-time 5 http://<ip-da-vm>/projeto-korp
curl: (28) Operation timed out after 5002 milliseconds

# de dentro da VM, no mesmo instante
HTTP/1.1 502 Bad Gateway
```

A explicação está no keepalive do upstream: o NGINX mantém conexões abertas
com o backend num pool. A primeira requisição após a queda pega uma conexão
morta e espera o `proxy_read_timeout` (60 s por padrão), acima dos 5 s do
`--max-time`. As seguintes já não encontram conexão no pool, tentam abrir uma
nova, levam recusa e devolvem 502 de imediato.

### Primeira rodada de verificação

```
RESULTADO: 21 OK / 11 falhas
```

As checagens acusaram — mas uma delas **passou quando não devia**:

```
requisito: prometheus coletando as metricas do servico
 OK  target http-server-projeto-korp com health=up
```

### Defeito encontrado no próprio script

A implementação usava `grep '"health":"up"'` sobre a resposta inteira da API
de targets. O Prometheus faz self-scrape, então o target dele próprio
satisfazia o padrão: o check nunca olhava o job da aplicação. Duas seções
acima, o check equivalente consultava `up{job="http-server-projeto-korp"}` —
seletor específico — e falhou corretamente. Mesma pergunta, dois níveis de
rigor.

Corrigido para filtrar pelo job e reportar o estado real:

```
FALHA target http-server-projeto-korp com health=down
```

Um segundo defeito apareceu na mesma rodada. A comparação de relógio exibia
`horario divergente em 57693s` — mas o relógio não havia divergido. Com a
resposta vazia, `date -u -d ""` devolve a meia-noite de hoje, e os 57.693
segundos eram apenas as horas decorridas desde então. A checagem falhava pelo
motivo certo com a mensagem errada, o que num incidente real levaria alguém a
investigar NTP em vez do container. Passou a distinguir "sem horário para
comparar" de "horário divergente".

```
RESULTADO: 20 OK / 12 falhas
```

### Detecção pelos alertas

`scrape_interval` de 15 s e `for: 1m` na regra. A transição observada foi
`inactive` → `pending` → `firing`, no tempo esperado:

```
ServicoIndisponivel           firing     up == 0 por mais de 1 minuto
DisponibilidadeAbaixoDoSLO    firing     media de 1h abaixo de 99%, apos for: 5m
ServicoReiniciouRecentemente  inactive   korp_uptime_seconds sumiu com a aplicacao
TaxaDeErro5xxAlta             inactive   ver observacao abaixo
LatenciaP95Alta               inactive   sem requisicoes novas, sem histograma
SemTrafego                    inactive   alerta inerte, documentado acima
```

Evidências: `docs/img/alerta-firing.png` (dois alertas em `firing` na UI do
Prometheus) e `docs/img/dashboard-degradado.png`.

### Duas observações que o teste revelou

**A `TaxaDeErro5xxAlta` não dispara numa queda total, e isso está correto.**
A métrica `korp_http_requests_total` é instrumentada dentro da aplicação Go.
Aplicação parada não conta nada — nem sucesso nem erro. Os 502 nascem no
NGINX, que não é instrumentado. O `ServicoIndisponivel` cobre esse caso pelo
lado do `up`, mas um cenário de 502 com a aplicação viva não teria alerta. O
caminho de produção seria expor métricas do próprio NGINX.

**O painel "Taxa de Sucesso (5m)" exibiu 100% em verde com o serviço fora do
ar.** Mesma causa: sem amostras novas, o `rate()` não atualiza e o redutor
`lastNotNull` mostra o último valor conhecido, de antes da queda. O painel
"Uptime do Processo" congelou pelo mesmo motivo. É o efeito colateral da
correção do defeito 4 (`lastNonNull` → `lastNotNull`): o redutor resolve o
painel em branco, mas nunca exibe "sem dados". O contraste dentro do mesmo
dashboard é instrutivo — "Status Atual" mostrou **FORA DO AR** corretamente,
porque lê `up`, que é gerada pelo Prometheus e não pela aplicação.

### Restauração

```
TASK [korp_stack : Subir a stack com docker compose]
changed: [korp-devops]

PLAY RECAP *********************************************************************
korp-devops : ok=49  changed=1  unreachable=0  failed=0  skipped=4
```

Uma única task alterada, exatamente a que precisava. As outras 48 reportaram
`ok`: Docker já instalado, arquivos no lugar, rede existente, imagem já
atualizada. Isso é **convergência** — o playbook corrige o desvio sem tocar
no que está correto —, afirmação distinta da idempotência.

```
RESULTADO: 32 OK / 0 falhas
```

### O que o ciclo estabeleceu

| Momento | Resultado |
|---|---|
| Stack saudável | 32 OK / 0 falhas |
| Aplicação parada, script original | 21 OK / 11 falhas (um check mentiu) |
| Aplicação parada, script corrigido | 20 OK / 12 falhas |
| Após restauração | 32 OK / 0 falhas |

O primeiro e o último placar são numericamente iguais e epistemicamente
diferentes: o segundo veio depois de se saber que ele consegue virar 20/12
quando deve.

---

## Endurecimento aplicado após a primeira versão funcional

Duas mudanças feitas depois de a stack já estar validada, ambas
reverificadas com `verify.sh` (32 OK / 0 falhas) e com `PLAY RECAP`
`changed=0`.

**Senha do Grafana movida para `ansible-vault`.** O valor real passou para
`group_vars/all/vault.yml`, criptografado; o `vars.yml` guarda apenas a
indireção `{{ vault_korp_grafana_password }}`. A task que renderiza o `.env`
recebeu `no_log: true` — o cofre protege o segredo em repouso, mas depois de
resolvido a variável é texto comum, e `ansible-playbook --diff` imprimiria o
arquivo renderizado no console.

**Prometheus restrito ao loopback.** O bind passou de `9090:9090` para
`127.0.0.1:9090:9090`. O único consumidor é o Grafana, que alcança o
Prometheus pelo DNS interno da rede; publicar em todas as interfaces expunha
na rede local um serviço sem autenticação. Confirmado no host alvo:

```
# ss -lntp | grep 9090
LISTEN 0  4096  127.0.0.1:9090  0.0.0.0:*
```

Uma linha, em IPv4 apenas — `0.0.0.0:9090` e `[::]:9090` desapareceram.
A inspeção externa da UI de targets passou a usar encaminhamento de porta,
comando que o resumo final do playbook imprime pronto.

---

## Entrega

- [x] `README.md` revisado
- [x] `docs/01-rastreabilidade-requisitos.md` conferido contra o código real
- [x] Este registro preenchido
- [x] Screenshots em `docs/img/`
- [x] Logs em `docs/logs/`
- [x] Nenhum segredo no histórico do Git
- [ ] Repositório público <!-- PREENCHER -->
- [ ] Tag `v1.0.0` publicada <!-- PREENCHER -->
- [ ] E-mail enviado para `rh@korp.com.br` <!-- PREENCHER -->
