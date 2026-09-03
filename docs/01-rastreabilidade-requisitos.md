# Rastreabilidade de Requisitos

Comparação item a item entre o que o enunciado do desafio pede e o que foi
entregue. Onde houve desvio ou acréscimo, a justificativa vem junto.

O enunciado é explícito: *"os requisitos desse documento representam o mínimo
esperado"* e melhorias *"também serão consideradas na avaliação"*. Este
documento existe para tornar essas escolhas visíveis e defensáveis, em vez de
deixá-las escondidas no código.

## Legenda

| Símbolo | Significado |
|---|---|
| ✅ | Atendido exatamente como pedido |
| ➕ | Atendido, com acréscimo além do mínimo |
| 🔄 | Atendido de forma diferente da sugerida, por decisão consciente |
| 🎁 | Item marcado como bônus no enunciado |

---

## Parte 1 — Serviço e arquitetura do ambiente

| # | Requisito | Implementação | Status |
|---|---|---|---|
| 1.1 | Servidor HTTP em Golang | `app/main.go` + `app/internal/handlers` | ✅ |
| 1.2 | Serviço chamado `http-server-projeto-korp` | Nome do serviço no compose, do container e da imagem | ✅ |
| 1.3 | Recebe requisições na porta 8080 | Porta interna do container | ✅ |
| 1.4 | Endpoint `GET /projeto-korp` | Registrado no roteador | ✅ |
| 1.5 | Retorna JSON `{nome, horario}` | Struct serializada, campos exatos | ✅ |
| 1.6 | `horario` em UTC, resolvido a cada requisição | `time.Now().UTC()` dentro do handler, não em variável de pacote | ➕ Coberto por teste unitário |
| 1.7 | Dockerfile com build e execução | Multi-stage: `golang:1.22-alpine` → `distroless/static` | ➕ |
| 1.8 | Docker instalado e configurado em Linux | Role `docker`, repositório oficial, Debian 13 | 🔄 |
| 1.9 | Rede Docker em modo bridge | `korp-net`, criada pela role e referenciada no compose | ✅ |
| 1.10 | Compose com a app na rede, sem expor portas ao host | Serviço sem chave `ports:` | ✅ |
| 1.11 | Compose com NGINX oficial, mesma rede, `80:80` | Imagem oficial, `korp-net`, mapeamento 80 | ✅ |
| 1.12 | Volume montado em `/etc/nginx/conf.d/` | Bind mount, montado como **somente leitura** | ➕ |
| 1.13 | `http-server-projeto-korp.conf` com proxy reverso | No volume montado, `proxy_pass` para o serviço:8080 | ➕ |
| 1.14 | `curl http://localhost:80/projeto-korp` funciona | Alvo `make smoke`, com validação do contrato | ➕ |

---

## Parte 2 — Monitoramento e observabilidade

| # | Requisito | Implementação | Status |
|---|---|---|---|
| 2.1 | Métrica de disponibilidade do serviço | Endpoint `/health` + métrica `up` do Prometheus | 🔄 Forma livre no enunciado |
| 2.2 | Métrica de volume de requisições | Contador `http_requests_total`, com labels de rota, método e código | ➕ |
| 2.3 | Métricas no padrão Prometheus | Endpoint `/metrics` via `client_golang` | ✅ |
| 2.4 | Compose com Prometheus | Serviço na `korp-net`, config versionada | ✅ |
| 2.5 | Compose com Grafana | Serviço na `korp-net`, provisionamento por arquivo | ✅ |
| 2.6 | Prometheus coletando as métricas do serviço | Scrape direto no container `:8080`, sem passar pelo NGINX | 🔄 |
| 2.7 | Grafana configurado para visualizar | Datasource com `uid` fixo `prometheus-korp` | ➕ |
| 2.8 | Dashboard para analisar o comportamento | JSON versionado, carregado no boot | ➕ |

---

## Parte 3 — Automação com Ansible

| # | Requisito | Implementação | Status |
|---|---|---|---|
| 3.1 | Instalação do Docker em Linux | Role `docker` | ✅ |
| 3.2 | Criação da rede Docker | Módulo `community.docker.docker_network` | ➕ |
| 3.3 | Build da imagem do serviço | Módulo `community.docker.docker_image` | ➕ |
| 3.4 | Criação e execução dos containers com compose | Módulo `community.docker.docker_compose_v2` | ➕ |
| 3.5 | Configuração do NGINX com proxy reverso | Template Jinja2 materializado no host alvo | ➕ |
| 3.6 | Configuração dos componentes de monitoramento | Configs do Prometheus e provisionamento do Grafana por template | ✅ |
| 3.7 | Validação HTTP com exibição da resposta no console | Role `validate`, com retry e `debug` | ➕ |
| 3.8 | Provisionamento com um único comando | `ansible-playbook site.yml` | ✅ |
| 3.9 | Grafana automatizado (`datasources.yml`, `dashboards.yml`, `*-dashboard.json`) | Os três arquivos existem e são aplicados no boot | 🎁 |
| 3.10 | Repositório público no GitHub | — | ✅ |

---

# Mudanças em relação ao enunciado — e o porquê

## A. Ambiente

### A.1 Host alvo remoto em vez de local

**Enunciado:** *"em um ambiente Linux de sua escolha"*.

**Escolha:** uma VM Debian 13 dedicada, rodando em um Proxmox local,
provisionada por SSH a partir de um control node separado.

**Motivo:** a alternativa óbvia seria `localhost ansible_connection=local`,
provisionando a própria máquina onde o Ansible roda. Funciona, mas não
demonstra Ansible — demonstra um shell script com sintaxe YAML. O ponto do
Ansible é configurar uma máquina remota, do zero, sem tocá-la manualmente.
Um servidor limpo é também o único jeito honesto de provar que o playbook
realmente instala o Docker, em vez de encontrá-lo já instalado.

**Custo:** exige preparar SSH por chave, sudo e inventário — três pontos a
mais que podem falhar. Cada um deles está na Fase 2 do plano de execução, com
critério de aceite próprio.

### A.2 Debian 13 em vez de Ubuntu

**Motivo:** familiaridade com a distro vale mais do que qualquer ganho teórico,
principalmente numa demo ao vivo em que é preciso depurar sob pressão. O Docker
suporta oficialmente o Debian 13 (Trixie), com os mesmos passos de instalação —
muda só o codename do repositório.

**Detalhe técnico:** o erro clássico aqui é
hardcodar a URL do repositório do Ubuntu e passar `trixie` como suite, o que
gera erro de release-file no APT. A role deriva distribuição *e* codename dos
facts do Ansible e monta a URL sozinha, então trocar de distro dentro das
famílias Debian e RedHat é transparente.

### A.3 Snapshot `base-limpa` em vez de cloud-init ou Terraform

**Motivo:** com prazo de 7 dias, um snapshot entrega o mesmo benefício prático
— reset para o zero absoluto em segundos — a uma fração do esforço de montar
template cloud-init ou escrever IaC. Rollback + um comando + serviço no ar é a
demonstração que interessa.

**Roadmap:** Terraform/OpenTofu criando a VM é o passo natural, listado como
extra opcional. É melhorar depois do essencial fechado do que entregar as duas
coisas pela metade.

### A.4 `sudo` sem senha para o usuário `korp`

**Motivo:** fluidez na demo ao vivo, com `become_ask_pass = False` no
`ansible.cfg`.

**Ressalva explícita:** em produção isso não passaria. As alternativas seriam
manter a senha e rodar com `-K`, ou restringir o NOPASSWD aos comandos
específicos que o playbook precisa. A opção foi tomada com consciência do
trade-off, não por desconhecimento dele.

---

## B. Aplicação Go

### B.1 Dockerfile multi-stage com imagem distroless

**Enunciado:** pede apenas build e execução em container.

**Escolha:** `golang:1.22-alpine` para compilar,
`gcr.io/distroless/static-debian12:nonroot` para executar.

**Motivo:** a imagem final carrega o binário e mais nada — sem shell, sem
gerenciador de pacotes, sem utilitários. Isso derruba drasticamente a
superfície de ataque e o tamanho da imagem (5,9 MB). Um binário Go estático
não precisa de sistema operacional embaixo.

A tag `nonroot` não é detalhe: ela faz o processo rodar sob um UID sem
privilégios já a partir da imagem, sem depender de uma diretiva `user:` no
compose que alguém possa esquecer de replicar. Combina com o `cap_drop: ALL`,
o `read_only` e o `no-new-privileges` declarados no serviço — cada camada
cobre um vetor diferente, e nenhuma delas depende das outras para valer.

**Consequência que virou decisão:** sem shell, `curl` ou `wget`, o
`HEALTHCHECK` do Docker não tem como funcionar do jeito convencional. A
solução foi compilar um segundo binário Go minúsculo (`cmd/healthcheck`)
dentro do mesmo build e usá-lo como healthcheck. É o tipo de detalhe que só
aparece quando se leva a escolha de imagem até o fim.

### B.2 Endpoints `/health` e `/metrics` além do exigido

O enunciado pede apenas `GET /projeto-korp`. Os outros dois existem porque a
Parte 2 exige expor disponibilidade e volume de requisições no padrão
Prometheus — e porque um serviço sem endpoint de saúde não é operável.

### B.3 Testes unitários

Não pedidos. Existem porque o enunciado destaca **em negrito** dois pontos do
campo `horario`: que ele seja UTC e que seja resolvido dinamicamente a cada
requisição. Um teste que congela o relógio em um fuso `-03:00` e confere a
conversão, e outro que faz duas requisições e prova que o valor muda, provam
esses requisitos melhor do que qualquer afirmação no README.

### B.4 Shutdown gracioso

`docker stop` envia `SIGTERM`. Sem tratamento, requisições em voo morrem no
meio. O servidor captura o sinal e drena as conexões antes de encerrar.

---

## C. Docker e Compose

### C.1 Volume do NGINX montado como somente leitura

**Enunciado:** *"monte um volume no caminho `/etc/nginx/conf.d/`"*.

**Escolha:** bind mount com `:ro`.

**Motivo:** a configuração é gerada pelo Ansible no host e o container só
precisa lê-la. Montar como leitura impede que um processo comprometido dentro
do container altere a configuração do proxy. O requisito é atendido; só ficou
mais restrito do que o mínimo.

### C.2 Grafana publicado no host; Prometheus restrito ao loopback

**Enunciado:** define apenas o mapeamento de portas do NGINX (`80:80`). Não
menciona a porta 9090 nem a 3000 em nenhum ponto.

**Escolha:** o Grafana é publicado em `0.0.0.0:3000`; o Prometheus é publicado
como `127.0.0.1:9090:9090` — alcançável apenas a partir da própria VM.

**Motivo:** as duas portas foram avaliadas separadamente, e não como um par.
O Grafana é a interface que o enunciado exige demonstrar, então precisa ser
alcançável da rede. Já o Prometheus tem um único consumidor — o Grafana — que
o alcança pelo DNS interno da rede `korp-net`, em `prometheus:9090`. Publicá-lo
em todas as interfaces não atenderia nenhum consumidor legítimo; apenas
exporia na rede local um serviço sem autenticação, cuja API permite consultar
toda a volumetria e as rotas internas do ambiente.

Como o enunciado não especifica a porta 9090, restringi-la não desvia do
requisito: é acréscimo, na mesma categoria do volume do NGINX montado como
somente leitura.

**Custo:** a UI de targets e de alertas deixa de abrir pelo navegador a partir
de outra máquina. O acesso passa a ser por encaminhamento de porta:

```bash
ssh -L 9090:127.0.0.1:9090 korp@<ip-da-vm>
```

O resumo final do playbook imprime esse comando pronto, com o usuário e o host
já substituídos, para que o acesso não dependa de memória durante a demo.

**Detalhe técnico:** prefixar o bind com um
endereço IPv4 faz o Docker publicar **somente** em IPv4 — `[::]:9090`
desaparece do `docker ps`. É o mesmo mecanismo do healthcheck do NGINX
documentado no cabeçalho do compose, agora do lado do host. Nada quebrou
porque todos os consumidores restantes (`curl`, o módulo `uri` do Ansible e o
cliente Go do próprio Prometheus no self-scrape) iteram sobre os endereços
resolvidos e caem para IPv4; o `wget` do BusyBox, que não faz esse fallback,
não participa desse caminho.

**Alternativa avaliada:** colocar ambos atrás do NGINX, em `/grafana` e
`/prometheus`. Seria arquitetonicamente melhor — um único ponto de entrada,
coerente com o proxy reverso que o desafio já pede. Exigiria
`--web.external-url` no Prometheus e `GF_SERVER_ROOT_URL` mais
`GF_SERVER_SERVE_FROM_SUB_PATH` no Grafana, além de tratar caminhos de assets.
Ficou como roadmap: o ganho não justificava alterar uma stack validada às
vésperas da entrega.

### C.3 Healthchecks e `depends_on: condition: service_healthy`

Sem isso, a ordem de subida vira loteria: o Grafana tenta consultar o
Prometheus antes de ele estar pronto, o dashboard aparece vazio e parece
quebrado. Com healthcheck, a stack sobe de forma determinística — o que
importa muito quando a stack precisa subir de forma previsível numa
demonstração ao vivo.

### C.4 Rotação de logs (`json-file`, `max-size: 10m`, `max-file: 3`)

O padrão do Docker é log ilimitado. Numa VM de 32 GB rodando por dias com
tráfego gerado, isso enche o disco. É configuração de duas linhas que evita
um problema chato e nada óbvio.

---

## D. NGINX

### D.1 `/metrics` e `/health` bloqueados no proxy

O Prometheus está na mesma rede `korp-net` e faz scrape direto no container,
na porta 8080 — não passa pelo NGINX. Isso permite bloquear esses caminhos na
borda sem quebrar a coleta.

**Motivo:** `/metrics` expõe volumetria, rotas internas e comportamento do
serviço. Não há razão para deixá-lo acessível pela porta 80 pública.

### D.2 Cabeçalhos de proxy explícitos

`X-Forwarded-For`, `X-Forwarded-Proto` e `Host` são propagados. Sem eles, a
aplicação enxerga todas as requisições vindo do IP do NGINX, e qualquer log ou
métrica por origem vira ficção.

---

## E. Observabilidade

### E.1 Disponibilidade exposta de duas formas

**Enunciado:** *"a forma de expor a disponibilidade do serviço pode ser
definida pelo candidato"*.

**Escolha:** endpoint `/health` **e** a métrica sintética `up` que o próprio
Prometheus gera para cada target.

**Motivo:** são coisas diferentes e complementares. O `/health` responde
"a aplicação considera a si mesma saudável?" — é o que o Docker consulta.
O `up` responde "o Prometheus consegue falar com ela?" — que é a
disponibilidade do ponto de vista de quem observa. Se a aplicação travar sem
morrer, o `up` cai mesmo com o processo vivo. Medir disponibilidade só de
dentro do próprio processo é o erro clássico dessa etapa.

### E.2 Scrape direto no container, não via NGINX

Coletar através do proxy mediria a saúde do conjunto app+proxy, e uma falha do
NGINX apareceria como falha da aplicação. Separar as camadas mantém o
diagnóstico correto: o Prometheus também monitora a si mesmo, evitando o ponto
cego de descobrir que a coleta parou só quando alguém reclama.

### E.3 `uid` fixo no datasource

O `uid` do datasource é fixado em `prometheus-korp` porque o JSON do dashboard
o referencia. Sem fixar, o Grafana geraria um uid aleatório a cada
provisionamento e todos os painéis ficariam órfãos — um sintoma que costuma
custar uma tarde de depuração para quem nunca viu.

### E.4 Dashboard escrito como código, não exportado da interface

Dashboards exportados do Grafana vêm com centenas de linhas de ruído e são
irrevisáveis em pull request. O JSON aqui foi gerado por script, é legível e
tem diff útil.

### E.5 Regras de alerta sem Alertmanager

`alerts.yml` define regras que ficam visíveis em `/alerts` no Prometheus, mas
não há roteamento de notificação. É escopo consciente: mostrar que as regras
foram pensadas sem inflar a stack com um componente que ninguém vai testar na
demo.

---

## F. Ansible

### F.1 Um playbook, organizado em roles

**Enunciado:** *"crie um playbook Ansible"*.

**Escolha:** `site.yml` como ponto de entrada único, delegando para três roles
— `docker`, `korp_stack` e `validate`.

**Motivo:** o requisito de "um único comando" é sobre a interface de execução,
não sobre o arquivo ser monolítico. Roles separadas por responsabilidade são
reutilizáveis, testáveis isoladamente por tags e muito mais legíveis. Um YAML
de 400 linhas atenderia a letra do requisito e falharia no critério declarado
de "clareza e estruturação do raciocínio".

### F.2 Módulos `community.docker` em vez de `shell`/`command`

Chamar `docker run` via `shell` reporta `changed` em toda execução e não sabe
se o estado desejado já existe. Os módulos da coleção comparam o estado atual
com o desejado e só agem quando há diferença — que é o que torna a segunda
execução do playbook retornar `changed=0`. Idempotência não é detalhe
cosmético: é o que diferencia configuração declarativa de automação de
mutirão.

### F.3 `deb822_repository` em vez de `apt_repository`

`apt_repository` está deprecado e será removido no `ansible-core` 2.25. O
formato deb822 é o atual. Exige `ansible-core >= 2.15` e `python3-debian` no
alvo — ambos validados logo no início do playbook, com mensagem de erro que
diz o que fazer.

### F.4 Checagem de versão do SDK Python do Docker

Instalar o pacote da distro não basta: ele pode ser velho demais para a
coleção `community.docker`. A role consulta a versão que o Python realmente
enxerga e cai para o `pip` só quando necessário. O `--break-system-packages`
aparece porque Debian 12+ e Ubuntu 24.04 marcam o Python do sistema como
*externally managed* (PEP 668) — é o escape oficial para esse caso, não uma
gambiarra.

### F.5 Tags nas roles

Permitem executar partes isoladas durante o desenvolvimento
(`--tags validate`, `--skip-tags docker`). Não alteram o requisito do comando
único, que continua sendo `ansible-playbook site.yml` sem argumentos.

### F.6 Pre-tasks de validação

O playbook falha cedo, com mensagem clara, se as coleções não estiverem
instaladas ou se o código-fonte não estiver acessível no control node. Falhar
no minuto zero com "instale as coleções" é infinitamente melhor do que falhar
no minuto sete com um erro de módulo não encontrado.

---

### F.7 Senha do Grafana em `ansible-vault`

**Enunciado:** não trata de credenciais. O caminho de menor esforço seria
deixar `admin`/`admin` no compose.

**Escolha:** o valor real vive em `ansible/group_vars/all/vault.yml`,
criptografado com AES256 e versionado no repositório. O `vars.yml` guarda
apenas a indireção:

```yaml
korp_grafana_password: "{{ vault_korp_grafana_password }}"
```

**Motivo da indireção com prefixo `vault_`:** permite localizar por `grep`
onde cada segredo é usado sem precisar descriptografar o cofre. Um arquivo
criptografado inteiro é opaco para revisão; a indireção devolve visibilidade
de *onde* sem expor *o quê*.

**Como o requisito do comando único é preservado:** o `ansible.cfg` aponta
`vault_password_file = vault-pass.txt`, arquivo listado no `.gitignore`. Quem
clonar o repositório cria esse arquivo com a senha recebida por canal separado
e `make deploy` continua funcionando sem argumento extra. A alternativa
interativa (`ansible-playbook --ask-vault-pass`) está registrada em comentário
no próprio `ansible.cfg`.

**Consequência que virou decisão:** o vault protege o segredo em repouso, não
na saída. Depois de resolvida, a variável é texto comum, e
`ansible-playbook --diff` imprimiria o `.env` renderizado — com a senha — no
console. A task que gera o arquivo leva `no_log: true` por isso, e o arquivo
é materializado no destino com `mode: 0600`.



| Item | Motivo |
|---|---|
| `Makefile` | Interface uniforme de comandos. Reduz erro de digitação na demo e serve de documentação executável |
| `.gitignore` | Impede que binários, `.env`, chaves e state do Terraform vazem para o repositório público |
| `make load` | Gera tráfego, incluindo erros propositais. Sem isso o dashboard fica vazio e a demo não convence |
| `make smoke` | Valida o contrato do JSON, não só o status HTTP. Confere `nome` e o sufixo `Z` do horário |
| `HOST ?= localhost` nos alvos `smoke` e `load` | O default reproduz o comando literal do enunciado quando executado na VM; `make smoke HOST=<ip>` permite rodar do control node sem editar o Makefile |
| `README.md` com seção de decisões | O enunciado avalia "clareza e estruturação do raciocínio" explicitamente |

---

## H. Limitações conhecidas

Escolhas de escopo declaradas, não descuidos. Cada item abaixo foi avaliado
durante o projeto e adiado conscientemente, com o caminho de produção
identificado.

| Limitação | O que seria feito em produção |
|---|---|
| A senha do cofre (`vault-pass.txt`) é entregue por canal separado, fora do repositório | Cofre gerenciado (HashiCorp Vault, AWS SSM, Azure Key Vault) com autenticação por identidade da máquina, sem arquivo de senha em disco |
| `verify.sh` passa a credencial do Grafana em `curl -u`, visível em `ps` durante a execução | `--netrc-file` ou `curl -K -` alimentado por stdin; num pipeline, a credencial viria do cofre direto para a variável de ambiente do processo |
| Prometheus alcançável só da VM — inspeção externa depende de túnel SSH | Atrás do proxy reverso em `/prometheus`, com autenticação na borda |
| Sem TLS — o desafio pede HTTP na porta 80 | Terminação TLS no NGINX com certificado do Let's Encrypt |
| Sem Alertmanager | Roteamento de notificação para e-mail, Slack ou PagerDuty |
| Prometheus com armazenamento local, retenção de 15 dias | `remote_write` para Thanos ou Mimir |
| Instância única, sem alta disponibilidade | Réplicas com balanceamento e health checks externos |
| VM criada manualmente | Terraform/OpenTofu, com o playbook rodando logo após o provisionamento |
| Sem pipeline de CI | GitHub Actions com lint, testes, build e scan de vulnerabilidades (Trivy) |
