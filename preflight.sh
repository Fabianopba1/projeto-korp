#!/usr/bin/env bash
# Checa control node e host alvo antes do primeiro deploy. Nao altera nada.
#
#   ./preflight.sh
#   VM_IP=192.168.0.161 ./preflight.sh

VM_IP="${VM_IP:-192.168.0.161}"
VM_USER="${VM_USER:-korp}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519_korp}"
PROJ="${PROJ:-$HOME/Projetos/projeto-korp}"

SSH="ssh -i $SSH_KEY -o IdentitiesOnly=yes -o ConnectTimeout=5 -o BatchMode=yes ${VM_USER}@${VM_IP}"

ok=0; warn=0; fail=0

verde()    { printf '  \033[32m OK \033[0m %s\n' "$1"; ok=$((ok+1)); }
amarelo()  { printf '  \033[33mAVISO\033[0m %s\n' "$1"; warn=$((warn+1)); }
vermelho() { printf '  \033[31mFALHA\033[0m %s\n' "$1"; fail=$((fail+1)); }
secao()    { printf '\n\033[1m%s\033[0m\n' "$1"; }

# checa(descricao, comando, regex_esperada)
checa() {
  local desc="$1" out
  out=$(eval "$2" 2>/dev/null)
  if [[ -n "$3" ]]; then
    if [[ "$out" =~ $3 ]]; then verde "$desc: $out"; else vermelho "$desc: '$out' (esperado: $3)"; fi
  else
    if [[ -n "$out" ]]; then verde "$desc: $out"; else vermelho "$desc: sem resposta"; fi
  fi
}

echo "==============================================================="
echo "  PREFLIGHT - Projeto Korp"
echo "  Alvo: ${VM_USER}@${VM_IP}   Projeto: ${PROJ}"
echo "==============================================================="

secao "1. CONTROL NODE"

if command -v ansible >/dev/null 2>&1; then
  # Captura tudo antes de filtrar: 'ansible --version | head' fecha o pipe
  # cedo e gera BrokenPipeError.
  AVOUT=$(ansible --version 2>/dev/null)
  AV=$(printf '%s\n' "$AVOUT" | head -1 | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1)
  MAJ=${AV%%.*}; MIN=$(echo "$AV" | cut -d. -f2)
  if (( MAJ > 2 || (MAJ == 2 && MIN >= 15) )); then
    verde "ansible-core $AV (>= 2.15, deb822_repository disponivel)"
  else
    vermelho "ansible-core $AV e antigo demais. A role usa deb822_repository (precisa 2.15+)"
  fi
else
  vermelho "ansible nao instalado no control node"
fi

for col in community.docker ansible.posix; do
  if ansible-galaxy collection list 2>/dev/null | grep -q "^$col "; then
    v=$(ansible-galaxy collection list 2>/dev/null | grep "^$col " | awk '{print $2}')
    verde "colecao $col $v"
  else
    vermelho "colecao $col ausente -> rode 'make deps'"
  fi
done

command -v go >/dev/null 2>&1 \
  && verde "go $(go version | awk '{print $3}')" \
  || amarelo "go ausente (opcional: a build roda no container)"

[[ -f "$SSH_KEY" ]] && verde "chave de automacao presente: $SSH_KEY" \
                    || vermelho "chave nao encontrada em $SSH_KEY"

if [[ -f "$SSH_KEY" ]]; then
  perm=$(stat -c '%a' "$SSH_KEY")
  [[ "$perm" == "600" ]] && verde "permissao da chave privada: $perm" \
                         || amarelo "permissao da chave: $perm (o ideal e 600)"
fi

secao "2. REPOSITORIO"

if [[ -d "$PROJ" ]]; then
  verde "projeto em $PROJ"

  [[ -f "$PROJ/app/go.sum" ]] && verde "go.sum presente ($(wc -l < "$PROJ/app/go.sum") linhas)" \
                              || vermelho "go.sum ausente -> rode 'make tidy'"

  if grep -qE '^toolchain ' "$PROJ/app/go.mod" 2>/dev/null; then
    amarelo "go.mod tem diretiva 'toolchain' (o container baixaria outro toolchain)"
  else
    verde "go.mod sem diretiva toolchain"
  fi

  for f in app/Dockerfile docker/docker-compose.yml \
           docker/nginx/conf.d/http-server-projeto-korp.conf \
           docker/prometheus/prometheus.yml \
           docker/grafana/provisioning/datasources/datasources.yml \
           docker/grafana/provisioning/dashboards/http-server-projeto-korp-dashboard.json \
           ansible/site.yml; do
    [[ -f "$PROJ/$f" ]] && verde "$f" || vermelho "$f AUSENTE"
  done

  INV="$PROJ/ansible/inventory/hosts.ini"
  if grep -qE '^\s*localhost' "$INV" 2>/dev/null; then
    vermelho "inventario ainda aponta para localhost -> comente essa linha"
  elif grep -q "$VM_IP" "$INV" 2>/dev/null; then
    verde "inventario aponta para $VM_IP"
  else
    vermelho "inventario nao contem $VM_IP"
  fi

  [[ -d "$PROJ/.git" ]] && verde "repositorio git inicializado" \
                        || amarelo "git ainda nao inicializado"
else
  vermelho "projeto nao encontrado em $PROJ"
fi

secao "3. CONECTIVIDADE COM A VM"

if $SSH "echo conectado" >/dev/null 2>&1; then
  verde "SSH por chave, sem senha"
else
  vermelho "SSH falhou. Restante das checagens sera pulado."
  echo; echo "Resumo: $ok OK / $warn avisos / $fail falhas"; exit 1
fi

checa "sudo sem senha (apos sudo -k)" "$SSH 'sudo -k; sudo -n whoami'" '^root$'

nk=$($SSH 'wc -l < ~/.ssh/authorized_keys' 2>/dev/null)
[[ "$nk" == "1" ]] && verde "authorized_keys com 1 chave (so a de automacao)" \
                   || amarelo "authorized_keys com $nk chaves"

secao "4. SISTEMA OPERACIONAL DA VM"

checa "distribuicao" "$SSH 'grep ^VERSION_CODENAME= /etc/os-release | cut -d= -f2'" '^trixie$'
checa "python3"      "$SSH 'python3 --version'" 'Python 3'
checa "kernel"       "$SSH 'uname -r'" ''
checa "hostname"     "$SSH 'hostname'" 'korp-devops'

cores=$($SSH 'nproc' 2>/dev/null)
[[ "$cores" -ge 2 ]] && verde "vCPUs: $cores" || amarelo "vCPUs: $cores (esperado 2)"

ram=$($SSH "free -m | awk '/^Mem:/{print \$2}'" 2>/dev/null)
[[ "$ram" -ge 3500 ]] && verde "RAM: ${ram} MiB" || amarelo "RAM: ${ram} MiB (esperado ~4096)"

livre=$($SSH "df -BG --output=avail / | tail -1 | tr -dc '0-9'" 2>/dev/null)
[[ "$livre" -ge 15 ]] && verde "disco livre em /: ${livre} GiB" \
                      || amarelo "disco livre: ${livre} GiB (a stack precisa de ~8 GiB)"

if $SSH 'systemctl is-active --quiet qemu-guest-agent' 2>/dev/null; then
  verde "qemu-guest-agent ativo"
else
  amarelo "qemu-guest-agent inativo (IP nao aparece no Proxmox)"
fi

secao "5. SINCRONIA DE RELOGIO"

sync=$($SSH "timedatectl show -p NTPSynchronized --value" 2>/dev/null)
[[ "$sync" == "yes" ]] && verde "relogio sincronizado via NTP" \
                       || vermelho "relogio NAO sincronizado -> metricas e horario ficam errados"

drift=$($SSH "date -u +%s" 2>/dev/null)
local_s=$(date -u +%s)
delta=$(( drift > local_s ? drift - local_s : local_s - drift ))
(( delta <= 2 )) && verde "diferenca de relogio com o control node: ${delta}s" \
                 || amarelo "diferenca de relogio: ${delta}s"

secao "6. PORTAS E ESTADO LIMPO"

for p in 80 3000 8080 9090; do
  if $SSH "ss -tulpn 2>/dev/null | grep -q ':$p '" 2>/dev/null; then
    vermelho "porta $p JA OCUPADA -> vai conflitar com a stack"
  else
    verde "porta $p livre"
  fi
done

if $SSH 'command -v docker' >/dev/null 2>&1; then
  amarelo "docker JA instalado na VM (a base deveria estar limpa)"
else
  verde "docker ausente (correto: quem instala e o playbook)"
fi

if $SSH 'command -v nginx || command -v apache2' >/dev/null 2>&1; then
  vermelho "servidor web instalado no host -> vai disputar a porta 80"
else
  verde "nenhum servidor web no host"
fi

secao "7. RESOLUCAO DNS DOS REGISTRIES"

for d in deb.debian.org download.docker.com registry-1.docker.io \
         auth.docker.io gcr.io storage.googleapis.com proxy.golang.org; do
  if $SSH "getent hosts $d" >/dev/null 2>&1; then
    verde "DNS resolve $d"
  else
    vermelho "DNS NAO resolve $d"
  fi
done

secao "8. ALCANCE HTTPS AOS REGISTRIES"

if $SSH 'command -v curl' >/dev/null 2>&1; then
  for u in https://download.docker.com https://registry-1.docker.io/v2/ \
           https://gcr.io/v2/ https://proxy.golang.org; do
    code=$($SSH "curl -s -o /dev/null -w '%{http_code}' --max-time 8 $u" 2>/dev/null)
    if [[ "$code" =~ ^(200|301|302|401|403)$ ]]; then
      verde "alcanca $u (HTTP $code)"
    else
      vermelho "nao alcanca $u (HTTP $code)"
    fi
  done
else
  amarelo "curl ausente na VM, pulando teste HTTPS"
fi

echo
echo "==============================================================="
printf "  RESUMO: \033[32m%d OK\033[0m / \033[33m%d avisos\033[0m / \033[31m%d falhas\033[0m\n" "$ok" "$warn" "$fail"
echo "==============================================================="
echo
echo "  Checagens manuais no Proxmox:"
echo "    [ ] snapshot limpo existe e a VM estava desligada"
echo "    [ ] ISO removida do drive de CD/DVD"
echo "    [ ] firewall da interface desmarcado"
echo "    [ ] espaco livre no storage (lvs --units g)"
echo

(( fail > 0 )) && exit 1 || exit 0
