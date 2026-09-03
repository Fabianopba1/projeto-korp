.DEFAULT_GOAL := help
SHELL := /bin/bash -o pipefail

APP_DIR     := app
DOCKER_DIR  := docker
ANSIBLE_DIR := ansible
NETWORK     := korp-net
VERSION     ?= 1.0.0

# Host alvo dos testes HTTP.
# Default 'localhost' atende o requisito literal do desafio quando rodado na VM.
# Do notebook: make smoke HOST=192.168.0.161
HOST        ?= localhost

.PHONY: help
help: ## Lista os alvos disponiveis
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
	 | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

# --- Go ----------------------------------------------------------------------
.PHONY: tidy
tidy: ## Atualiza go.mod e go.sum
	cd $(APP_DIR) && go mod tidy

.PHONY: test
test: ## Roda os testes unitarios
	cd $(APP_DIR) && go test ./... -v -count=1

.PHONY: lint
lint: ## Roda go vet
	cd $(APP_DIR) && go vet ./...

.PHONY: run
run: ## Sobe a aplicacao localmente, sem Docker
	cd $(APP_DIR) && go run .

# --- Docker ------------------------------------------------------------------
.PHONY: network
network: ## Cria a rede bridge korp-net (idempotente)
	@docker network inspect $(NETWORK) >/dev/null 2>&1 \
	  || docker network create --driver bridge $(NETWORK)

.PHONY: build
build: ## Constroi a imagem da aplicacao
	docker build -t http-server-projeto-korp:$(VERSION) \
	  --build-arg VERSION=$(VERSION) \
	  --build-arg COMMIT=$$(git rev-parse --short HEAD 2>/dev/null || echo local) \
	  $(APP_DIR)

.PHONY: up
up: network ## Sobe a stack completa
	cd $(DOCKER_DIR) && docker compose up -d --build

.PHONY: down
down: ## Derruba a stack (mantem os volumes)
	cd $(DOCKER_DIR) && docker compose down

.PHONY: destroy
destroy: ## Derruba a stack e apaga volumes e rede
	cd $(DOCKER_DIR) && docker compose down -v
	-docker network rm $(NETWORK)

.PHONY: ps
ps: ## Mostra o estado dos containers
	cd $(DOCKER_DIR) && docker compose ps

.PHONY: logs
logs: ## Acompanha os logs da stack
	cd $(DOCKER_DIR) && docker compose logs -f

# --- Validacao ---------------------------------------------------------------
.PHONY: smoke
smoke: ## Teste exigido pelo desafio
	@echo "--> curl http://$(HOST):80/projeto-korp"
	@resp=$$(curl -fsS http://$(HOST):80/projeto-korp) \
	  || { echo "FALHA: nenhum servico respondeu em http://$(HOST):80"; exit 1; }; \
	echo "$$resp"; \
	echo "$$resp" | python3 -c "import json,sys; d=json.load(sys.stdin); \
	  assert d['nome']=='Projeto Korp'; assert d['horario'].endswith('Z'); \
	  print('OK: contrato valido, horario em UTC')"

.PHONY: load
load: ## Gera trafego para popular os graficos do Grafana
	@echo "Gerando 300 requisicoes contra $(HOST)..."
	@fail=0; for i in $$(seq 1 300); do \
	  curl -fsS -o /dev/null http://$(HOST)/projeto-korp || fail=$$((fail+1)); \
	  if [ $$((i % 10)) -eq 0 ]; then \
	    curl -sS -o /dev/null http://$(HOST)/rota-inexistente; \
	  fi; \
	  sleep 0.1; \
	done; \
	if [ $$fail -gt 0 ]; then echo "FALHA: $$fail de 300 requisicoes nao completaram"; exit 1; fi; \
	echo "pronto"

# --- Ansible -----------------------------------------------------------------
.PHONY: deps
deps: ## Instala as colecoes Ansible necessarias
	cd $(ANSIBLE_DIR) && ansible-galaxy collection install -r requirements.yml

.PHONY: check
check: ## Valida a sintaxe do playbook
	cd $(ANSIBLE_DIR) && ansible-playbook site.yml --syntax-check

.PHONY: deploy
deploy: ## Provisiona o ambiente inteiro
	cd $(ANSIBLE_DIR) && ansible-playbook site.yml

.PHONY: validate
validate: ## Roda apenas a etapa de validacao
	cd $(ANSIBLE_DIR) && ansible-playbook site.yml --tags validate
