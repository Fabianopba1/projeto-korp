# http-server-projeto-korp

Serviço HTTP em Go, containerizado, com proxy reverso NGINX, observabilidade
via Prometheus e Grafana, e provisionamento completo por Ansible em um único
comando.

Projeto desenvolvido para o desafio técnico de Analista DevOps.

---

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

O único caminho de entrada do tráfego é o NGINX. A aplicação não publica
portas no host — fica acessível apenas de dentro da rede `korp-net`, para o
proxy e para o Prometheus.

---

## Estrutura do repositório

```
.
├── app/                              aplicação Go
│   ├── main.go                       servidor, rotas, shutdown gracioso
│   ├── internal/handlers/            endpoints + testes unitários
│   ├── internal/metrics/             instrumentação Prometheus
│   ├── cmd/healthcheck/              binário auxiliar do HEALTHCHECK
│   └── Dockerfile                    build multi-stage → distroless
│
├── docker/
│   ├── docker-compose.yml            os 4 containers da stack
│   ├── nginx/conf.d/
│   │   └── http-server-projeto-korp.conf    proxy reverso
│   ├── prometheus/
│   │   ├── prometheus.yml            configuração de scrape
│   │   └── alerts.yml                regras de alerta
│   └── grafana/provisioning/
│       ├── datasources/datasources.yml
│       └── dashboards/
│           ├── dashboards.yml
│           └── http-server-projeto-korp-dashboard.json
│
├── ansible/
│   ├── site.yml                      playbook principal
│   ├── inventory/hosts.ini
│   ├── group_vars/all.yml
│   └── roles/
│       ├── docker/                   instala e configura o Docker Engine
│       ├── korp_stack/               rede, build, configs, containers
│       └── validate/                 requisição HTTP + saída no console
│
└── Makefile                          atalhos (`make help`)
```

---

## Pré-requisitos

**No control node** (a máquina de onde você roda o Ansible):

- Ansible Core >= 2.15
- Coleções: `make deps`

**No host alvo:** apenas Python 3 e acesso SSH com sudo. O Docker é
instalado pelo próprio playbook, a partir do repositório oficial.

Distribuições suportadas: família Debian (Debian 11/12/13, Ubuntu 22.04/24.04)
e família RedHat (Rocky, Alma, CentOS Stream). O playbook lê o codename da
distribuição dos facts do Ansible e monta a URL do repositório do Docker de
acordo, então trocar de distro dentro dessas famílias não exige alteração.

---

## Execução

### Provisionamento completo (o comando único do desafio)

```bash
# 1. gere o go.sum (só na primeira vez)
make tidy

# 2. instale as coleções Ansible
make deps

# 3. provisione tudo
make deploy
```

O alvo `deploy` executa `ansible-playbook site.yml`, que instala o Docker,
cria a rede bridge, constrói a imagem, sobe os quatro containers, configura
o proxy reverso e o monitoramento, e termina fazendo uma requisição HTTP de
validação com a resposta impressa no console.

### Escolhendo o alvo

Edite `ansible/inventory/hosts.ini`:

- **Local** — provisiona a própria máquina (padrão, útil para testar rápido)
- **Remoto** — descomente o bloco `korp-vm` e ajuste IP, usuário e chave SSH

O playbook é idêntico nos dois casos: as roles não assumem que o alvo é local.

### Sem Ansible (só Docker)

```bash
make up      # cria a rede e sobe a stack
make smoke   # roda o teste do desafio
```

---

## Verificação

```bash
curl http://localhost:80/projeto-korp
```

```json
{"nome":"Projeto Korp","horario":"2026-09-01T19:53:10Z"}
```

| Serviço    | URL                                    | Credenciais     |
|------------|----------------------------------------|-----------------|
| Aplicação  | http://localhost/projeto-korp          | —               |
| Prometheus | http://localhost:9090                  | —               |
| Grafana    | http://localhost:3000                  | `admin`/`admin` |

Para popular os gráficos antes de olhar o dashboard: `make load`.

---

## Métricas expostas

Ambas as métricas obrigatórias do desafio estão cobertas.

### Disponibilidade

Abordada por três ângulos complementares:

| Sinal | Origem | O que responde |
|-------|--------|----------------|
| `up{job="http-server-projeto-korp"}` | gerada pelo Prometheus a cada scrape | O alvo respondeu? |
| taxa de respostas 2xx/3xx | derivada de `korp_http_requests_total` | O serviço está funcionando *para o cliente*? |
| `korp_uptime_seconds` | aplicação | Houve restart recente / crash loop? |

Os três juntos existem porque cada um tem um ponto cego. `up` não enxerga um
serviço que responde 500 para todo mundo. A taxa de sucesso não distingue
"sem tráfego" de "tudo ok". E o uptime pega crash loops rápidos demais para o
`up` registrar entre dois scrapes.

### Volume de requisições

`korp_http_requests_total{method, path, status}` — counter, o tipo correto
para contagem monotônica. `rate()` e `increase()` tratam corretamente o reset
que ocorre quando o processo reinicia.

### Demais métricas

| Métrica | Tipo | Uso |
|---------|------|-----|
| `korp_http_request_duration_seconds` | histogram | latência por percentil |
| `korp_http_requests_in_flight` | gauge | concorrência instantânea |
| `korp_service_healthy` | gauge | saúde reportada pela aplicação |
| `korp_build_info` | gauge (info) | versão/commit em execução |
| `go_*`, `process_*` | vários | memória, GC, goroutines, CPU |

### Consultas PromQL úteis

```promql
# disponibilidade percentual nas últimas 6 horas
avg_over_time(up{job="http-server-projeto-korp"}[6h]) * 100

# requisições por segundo, por rota
sum by (path) (rate(korp_http_requests_total[5m]))

# taxa de erro 5xx
sum(rate(korp_http_requests_total{status=~"5.."}[5m]))
  / sum(rate(korp_http_requests_total[5m]))

# latência p95
histogram_quantile(0.95,
  sum by (le) (rate(korp_http_request_duration_seconds_bucket[5m])))
```

---

## Decisões técnicas

### Aplicação

**Biblioteca padrão, sem framework web.** O serviço tem quatro rotas. Gin ou
Echo trariam dependências, superfície de ataque e uma camada de abstração sem
ganho real nessa escala. `net/http` cobre tudo.

**Horário resolvido a cada requisição.** `time.Now().UTC().Format(time.RFC3339)`
é chamado dentro do handler, nunca em inicialização. O `.UTC()` garante o
mesmo resultado independente do timezone do container ou do host. Há um teste
que congela o relógio em `-03:00` e confirma a conversão, e outro que compara
duas requisições consecutivas para provar que o valor não está sendo cacheado.

**Injeção do relógio.** O handler recebe `Now func() time.Time` em vez de
chamar `time.Now()` direto. Isso torna o comportamento temporal testável sem
`sleep`.

**Shutdown gracioso.** O processo trata SIGTERM (o sinal que `docker stop`
envia) e chama `srv.Shutdown()`, drenando as conexões em voo antes de sair.
Sem isso, toda atualização derruba requisições no meio.

**Timeouts explícitos no servidor.** `ReadHeaderTimeout`, `ReadTimeout`,
`WriteTimeout` e `IdleTimeout` definidos. O padrão do Go é "sem limite", o
que permite que uma conexão lenta segure um handler indefinidamente
(Slowloris).

### Cardinalidade das métricas

O label `path` recebe **o padrão da rota registrada**, não `r.URL.Path`.
Rotas desconhecidas são agrupadas em `other`.

Isso não é detalhe estético. Usando a URL crua, qualquer cliente poderia
gerar séries temporais infinitas batendo em `/a`, `/b`, `/c`... Cada
combinação de labels vira uma série no Prometheus, e explosão de
cardinalidade é uma das formas mais comuns de derrubar um Prometheus em
produção.

Pelo mesmo motivo o endpoint `/metrics` fica fora do middleware de
instrumentação: contar o scrape do Prometheus como tráfego do serviço
poluiria a métrica de volume.

### Imagem Docker

**Multi-stage com runtime distroless.** O estágio de build usa
`golang:1.22-alpine`; o final é `gcr.io/distroless/static-debian12:nonroot`,
que contém apenas certificados CA, timezone e `/etc/passwd`. Sem shell, sem
gerenciador de pacotes, sem coreutils — não há binários para um invasor
reutilizar após um comprometimento. A imagem final fica em poucos MB.

**Ordem das camadas.** `go.mod`/`go.sum` são copiados antes do código-fonte,
então o download das dependências fica em cache enquanto os manifests não
mudarem, mesmo que todo o código tenha mudado.

**Testes dentro do build.** `go vet` e `go test` rodam no estágio de build.
Uma imagem só é produzida se a suíte passar.

**Binário de healthcheck próprio.** Como não há curl nem wget no distroless,
compilamos um segundo binário Go minúsculo usado pela instrução
`HEALTHCHECK`. A alternativa seria carregar uma imagem Alpine inteira em
produção só para ter o wget.

**Build reproduzível.** `-trimpath` remove caminhos da máquina de build;
`-ldflags="-s -w"` descarta símbolos e debug info; `CGO_ENABLED=0` produz um
binário estático. Versão e commit são injetados via `-X`.

**Forma exec no ENTRYPOINT.** Array, não string. Assim o processo é PID 1 de
verdade e recebe o SIGTERM diretamente, o que faz o shutdown gracioso
funcionar.

### Rede e Compose

**Rede declarada como `external: true`.** Ela é criada pelo Ansible, não pelo
Compose — exatamente como o item 3 do desafio pede. Isso também desacopla o
ciclo de vida da rede do ciclo de vida da stack.

**A aplicação não publica portas.** Não existe seção `ports` no serviço
`http-server-projeto-korp`, por exigência do desafio. A porta 8080 é
alcançável apenas de dentro de `korp-net`.

**`depends_on` com `condition: service_healthy`.** O NGINX resolve os
hostnames do upstream na inicialização. Se subir antes da aplicação existir,
falha com `host not found in upstream` e não se recupera sozinho. A condição
de saúde garante a ordem correta.

**Endurecimento dos containers.** `no-new-privileges`, `cap_drop: ALL`,
`read_only` no filesystem da aplicação, limites de CPU e memória, e rotação
de log configurada (`max-size` + `max-file`) — disco cheio por log de
container é um incidente clássico.

### NGINX

**`/metrics` bloqueado na borda.** O Prometheus está na mesma rede e fala
direto com o container na porta 8080, sem passar pelo proxy. Então não há
razão para expor as métricas publicamente: elas revelam volume de tráfego,
rotas internas e versão do binário.

**Headers de encaminhamento.** Sem `X-Forwarded-For` e `X-Real-IP`, a
aplicação enxergaria o IP interno do container do NGINX como origem de todo
o tráfego.

**`keepalive` no upstream.** Mantém conexões TCP abertas com o backend em vez
de reabrir a cada requisição. Exige HTTP/1.1 e `Connection ""` no location.

**Rate limiting.** 10 req/s por IP com burst de 20.

### Ansible

**Roles separadas por responsabilidade.** `docker` (instalação),
`korp_stack` (deploy) e `validate` (verificação) são independentes e
reutilizáveis. Tags permitem rodar etapas isoladas durante o
desenvolvimento: `--tags validate`.

**Repositório oficial do Docker,** não o pacote `docker.io` das distros —
traz o plugin `docker-compose-v2` e versões atualizadas. A chave GPG é
adicionada com `signed-by`, restrita a esse repositório, em vez de ir para o
chaveiro global.

**Idempotência.** Handlers só disparam quando algo realmente muda; o
`nginx -s reload` recarrega a configuração sem derrubar conexões, e o
Prometheus recarrega via `POST /-/reload`. Uma segunda execução do playbook
não deve reportar nenhum `changed`.

**Validação que realmente valida.** A role `validate` não se contenta com
HTTP 200: confere o campo `nome`, valida o formato do horário com regex
exigindo o sufixo `Z` (UTC), faz duas requisições para provar que o valor é
dinâmico, verifica que `/metrics` está bloqueado na borda, e confirma que o
datasource e o dashboard foram provisionados no Grafana.

### Grafana

Datasource e dashboard nascem prontos, por arquivos de provisionamento —
o item bônus do desafio. O `uid` do datasource é fixo (`prometheus-korp`)
porque o JSON do dashboard o referencia; sem fixar, o Grafana geraria um uid
aleatório e os painéis ficariam órfãos.

O dashboard foi escrito como código e gerado por script, não exportado da
interface. Fica legível, versionável e revisável em pull request.

---

## Limitações conhecidas

Escolhas conscientes de escopo, não descuidos:

- **Credenciais do Grafana em texto plano.** `admin`/`admin` via variável.
  Em produção isso viria de `ansible-vault`, Vault ou SSM.
- **Sem TLS.** O desafio pede HTTP na porta 80. Em produção o NGINX
  terminaria TLS com certificado do Let's Encrypt.
- **Sem Alertmanager.** As regras de alerta existem e ficam visíveis em
  `/alerts`, mas não há roteamento de notificação.
- **Prometheus com armazenamento local,** retenção de 15 dias. Para
  retenção longa, `remote_write` para Thanos ou Mimir.
- **Instância única.** Sem alta disponibilidade nem balanceamento.

---

## Roadmap

O que eu faria com mais tempo, em ordem de prioridade:

1. **Pipeline de CI** — lint, testes, build da imagem e scan de
   vulnerabilidades (Trivy) a cada push
2. **Provisionamento da VM com Terraform/OpenTofu** — infraestrutura e
   configuração ambas como código
3. **Alertmanager** com notificação real
4. **TLS** com certificado automatizado
5. **Testes de integração** subindo a stack completa via Testcontainers

---

## Comandos

```
make help       lista todos os alvos
make tidy       gera o go.sum
make test       roda os testes unitários
make deps       instala as coleções Ansible
make deploy     provisiona o ambiente inteiro
make up         sobe a stack só com Docker
make smoke      executa o teste do desafio
make load       gera tráfego para o dashboard
make destroy    remove tudo
```
