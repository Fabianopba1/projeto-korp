#!/usr/bin/env bash
# Aceite do ambiente provisionado contra os requisitos do desafio.
#
#   ./verify.sh
#   VM_IP=192.168.0.161 ./verify.sh

VM_IP="${VM_IP:-192.168.0.161}"
VM_USER="${VM_USER:-korp}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519_korp}"

S="ssh -i $SSH_KEY -o IdentitiesOnly=yes -o ConnectTimeout=5 -o BatchMode=yes ${VM_USER}@${VM_IP}"

ok=0; fail=0
verde()    { printf '  \033[32m OK \033[0m %s\n' "$1"; ok=$((ok+1)); }
vermelho() { printf '  \033[31mFALHA\033[0m %s\n' "$1"; fail=$((fail+1)); }
secao()    { printf '\n\033[1m%s\033[0m\n' "$1"; }
item()     { printf '  \033[2m%s\033[0m\n' "$1"; }

echo "==============================================================="
echo "  ACEITE - Projeto Korp   (alvo: ${VM_USER}@${VM_IP})"
echo "==============================================================="

$S "echo ok" >/dev/null 2>&1 || { echo "SSH indisponivel. Abortando."; exit 1; }

# A credencial do Grafana e lida do .env que o Ansible gera no host alvo,
# que e a fonte da verdade. Assim o script nao precisa conhecer a senha nem
# ser editado quando ela muda. O fallback cobre o caso de a stack ainda nao
# ter subido.
ENV_ALVO=/opt/projeto-korp/docker/.env
GF_USER=$($S "sudo grep '^GRAFANA_USER=' $ENV_ALVO 2>/dev/null | cut -d= -f2-" 2>/dev/null)
GF_PASS=$($S "sudo grep '^GRAFANA_PASSWORD=' $ENV_ALVO 2>/dev/null | cut -d= -f2-" 2>/dev/null)
GF_USER="${GF_USER:-admin}"
GF_PASS="${GF_PASS:-admin}"

secao "PARTE 1.1 - Servico HTTP"

item "requisito: o servico deve se chamar http-server-projeto-korp"
if $S "docker ps --format '{{.Names}}'" 2>/dev/null | grep -qx 'http-server-projeto-korp'; then
  verde "container http-server-projeto-korp em execucao"
else
  vermelho "container http-server-projeto-korp nao encontrado"
fi

item "requisito: o servico deve receber requisicoes na porta 8080"
if $S "docker exec http-server-projeto-korp /app/healthcheck" >/dev/null 2>&1; then
  verde "aplicacao respondendo na 8080 (healthcheck interno)"
else
  vermelho "aplicacao nao responde na 8080"
fi

item "requisito: GET /projeto-korp retorna JSON {nome, horario}"
RESP=$($S "curl -sS --max-time 5 http://localhost:80/projeto-korp" 2>/dev/null)
if echo "$RESP" | grep -q '"nome":"Projeto Korp"'; then
  verde "campo nome correto"
else
  vermelho "campo nome incorreto ou ausente. Resposta: ${RESP:-vazia}"
fi

item "requisito: horario atual em UTC"
HORA=$(echo "$RESP" | sed -n 's/.*"horario":"\([^"]*\)".*/\1/p')
if [[ "$HORA" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ]]; then
  verde "horario em RFC3339 com sufixo Z (UTC): $HORA"
else
  vermelho "formato de horario invalido: '${HORA:-vazio}'"
fi

item "requisito: horario resolvido dinamicamente a cada requisicao"
sleep 1.2
H2=$($S "curl -sS --max-time 5 http://localhost:80/projeto-korp" 2>/dev/null \
     | sed -n 's/.*"horario":"\([^"]*\)".*/\1/p')
if [[ -n "$HORA" && -n "$H2" && "$HORA" != "$H2" ]]; then
  verde "horario mudou entre requisicoes ($HORA -> $H2)"
else
  vermelho "horario nao mudou: pode estar sendo calculado uma unica vez"
fi

item "verificacao extra: o horario confere com o relogio real"
AGORA=$(date -u +%s)
SERV=$(date -u -d "$H2" +%s 2>/dev/null)
if [[ -n "$SERV" ]]; then
  D=$(( AGORA > SERV ? AGORA - SERV : SERV - AGORA ))
  (( D <= 5 )) && verde "diferenca com o relogio do control node: ${D}s" \
               || vermelho "horario divergente em ${D}s"
fi

secao "PARTE 1.2 - Docker instalado e configurado"

V=$($S "docker --version" 2>/dev/null)
[[ -n "$V" ]] && verde "$V" || vermelho "docker nao instalado"

C=$($S "docker compose version" 2>/dev/null)
[[ -n "$C" ]] && verde "$C" || vermelho "plugin compose ausente"

$S "systemctl is-enabled --quiet docker" 2>/dev/null \
  && verde "docker habilitado no boot" \
  || vermelho "docker nao habilitado no boot"

secao "PARTE 1.3 - Rede Docker em modo bridge"

item "requisito: criar uma rede Docker no modo bridge"
DRV=$($S "docker network inspect korp-net --format '{{.Driver}}'" 2>/dev/null)
[[ "$DRV" == "bridge" ]] && verde "rede korp-net existe, driver: bridge" \
                         || vermelho "rede korp-net ausente ou driver '$DRV'"

for c in http-server-projeto-korp nginx-projeto-korp prometheus-projeto-korp grafana-projeto-korp; do
  if $S "docker inspect $c --format '{{json .NetworkSettings.Networks}}'" 2>/dev/null | grep -q korp-net; then
    verde "$c conectado a korp-net"
  else
    vermelho "$c NAO esta em korp-net"
  fi
done

secao "PARTE 1.4 - Containers via Docker Compose"

item "requisito: o container da aplicacao NAO deve expor portas ao host"
P=$($S "docker inspect http-server-projeto-korp --format '{{json .NetworkSettings.Ports}}'" 2>/dev/null)
if [[ "$P" == "{}" || "$P" == *'"8080/tcp":null'* ]]; then
  verde "aplicacao sem portas publicadas no host"
else
  vermelho "aplicacao PUBLICA portas: $P"
fi

item "requisito: NGINX com a porta 80 do host mapeada para a 80 do container"
if $S "docker port nginx-projeto-korp 80" 2>/dev/null | grep -q ':80'; then
  verde "nginx publica 80:80"
else
  vermelho "mapeamento 80:80 ausente no nginx"
fi

item "requisito: imagem oficial do NGINX"
IMG=$($S "docker inspect nginx-projeto-korp --format '{{.Config.Image}}'" 2>/dev/null)
[[ "$IMG" == nginx:* ]] && verde "imagem oficial: $IMG" \
                        || vermelho "imagem inesperada: $IMG"

item "requisito: volume montado em /etc/nginx/conf.d/"
if $S "docker inspect nginx-projeto-korp --format '{{range .Mounts}}{{.Destination}} {{end}}'" 2>/dev/null \
   | grep -q '/etc/nginx/conf.d'; then
  verde "volume montado em /etc/nginx/conf.d"
else
  vermelho "volume em /etc/nginx/conf.d ausente"
fi

secao "PARTE 1.5 - Proxy reverso"

item "requisito: arquivo http-server-projeto-korp.conf no volume montado"
if $S "docker exec nginx-projeto-korp test -f /etc/nginx/conf.d/http-server-projeto-korp.conf" 2>/dev/null; then
  verde "http-server-projeto-korp.conf presente no container"
else
  vermelho "http-server-projeto-korp.conf ausente"
fi

$S "docker exec nginx-projeto-korp nginx -t" >/dev/null 2>&1 \
  && verde "configuracao do nginx valida (nginx -t)" \
  || vermelho "nginx -t reportou erro"

secao "PARTE 1.6 - Teste de funcionamento"

item "requisito: curl http://localhost:80/projeto-korp"
CODE=$($S "curl -s -o /dev/null -w '%{http_code}' --max-time 5 http://localhost:80/projeto-korp" 2>/dev/null)
[[ "$CODE" == "200" ]] && verde "HTTP $CODE via porta 80" \
                       || vermelho "HTTP $CODE (esperado 200)"

secao "PARTE 2.1 - Metricas no padrao Prometheus"

item "requisito: metrica de disponibilidade do servico"
# --get + --data-urlencode: as chaves e aspas do seletor precisam ir codificadas.
UP=$($S "curl -s --max-time 5 --get --data-urlencode 'query=up{job=\"http-server-projeto-korp\"}' http://localhost:9090/api/v1/query" 2>/dev/null)
if echo "$UP" | grep -q '"value".*"1"'; then
  verde "up{job=\"http-server-projeto-korp\"} = 1"
else
  vermelho "metrica up ausente ou = 0"
fi

item "requisito: metrica de volume de requisicoes"
REQ=$($S "curl -s --max-time 5 --get --data-urlencode 'query=sum(korp_http_requests_total)' http://localhost:9090/api/v1/query" 2>/dev/null)
N=$(echo "$REQ" | sed -n 's/.*"value":\[[0-9.]*,"\([0-9.]*\)".*/\1/p')
if [[ -n "$N" ]] && (( $(echo "$N > 0" | bc -l 2>/dev/null || echo 0) )); then
  verde "korp_http_requests_total = $N requisicoes contabilizadas"
else
  vermelho "korp_http_requests_total ausente ou zerado"
fi

item "verificacao: /metrics bloqueado na borda"
MC=$($S "curl -s -o /dev/null -w '%{http_code}' --max-time 5 http://localhost:80/metrics" 2>/dev/null)
[[ "$MC" == "403" || "$MC" == "404" ]] && verde "/metrics devolve $MC via nginx" \
                                       || vermelho "/metrics acessivel na borda (HTTP $MC)"

secao "PARTE 2.2 - Prometheus e Grafana"

item "requisito: prometheus coletando as metricas do servico"
H=$($S "curl -s --max-time 5 'http://localhost:9090/api/v1/targets?state=active'" 2>/dev/null)
if echo "$H" | grep -q '"health":"up"'; then
  verde "target http-server-projeto-korp com health=up"
else
  vermelho "target nao esta up no Prometheus"
fi

item "requisito: grafana configurado para visualizar as metricas"
GH=$($S "curl -s --max-time 5 http://localhost:3000/api/health" 2>/dev/null)
echo "$GH" | grep -qE '"database"[[:space:]]*:[[:space:]]*"ok"' \
  && verde "grafana saudavel" \
  || vermelho "grafana nao responde"

item "bonus: datasource provisionado por arquivo"
DS=$($S "curl -s -u \"$GF_USER:$GF_PASS\" --max-time 5 http://localhost:3000/api/datasources/uid/prometheus-korp" 2>/dev/null)
echo "$DS" | grep -q '"type":"prometheus"' && verde "datasource prometheus-korp provisionado" \
                                           || vermelho "datasource nao encontrado"

item "requisito: dashboard disponivel no Grafana"
DB=$($S "curl -s -u \"$GF_USER:$GF_PASS\" --max-time 5 http://localhost:3000/api/dashboards/uid/http-server-projeto-korp" 2>/dev/null)
if echo "$DB" | grep -q '"title"'; then
  NP=$(echo "$DB" | grep -o '"type":"timeseries"\|"type":"stat"' | wc -l)
  verde "dashboard provisionado ($NP paineis de dados)"
else
  vermelho "dashboard nao encontrado"
fi

secao "SAUDE GERAL DOS CONTAINERS"

for c in http-server-projeto-korp nginx-projeto-korp prometheus-projeto-korp grafana-projeto-korp; do
  ST=$($S "docker inspect $c --format '{{.State.Status}}|{{if .State.Health}}{{.State.Health.Status}}{{else}}sem-healthcheck{{end}}|{{.RestartCount}}'" 2>/dev/null)
  IFS='|' read -r status health restarts <<< "$ST"
  if [[ "$status" == "running" && "$restarts" == "0" ]]; then
    verde "$c: $status / $health / $restarts restarts"
  else
    vermelho "$c: $status / $health / $restarts restarts"
  fi
done

echo
echo "==============================================================="
printf "  RESULTADO: \033[32m%d OK\033[0m / \033[31m%d falhas\033[0m\n" "$ok" "$fail"
echo "==============================================================="
echo
echo "  Verificar manualmente:"
echo "    [ ] idempotencia: 'make deploy' 2x, a 2a termina com changed=0"
echo "    [ ] reprodutibilidade: snapshot limpo + 'make deploy'"
echo

(( fail > 0 )) && exit 1 || exit 0
