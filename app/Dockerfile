# syntax=docker/dockerfile:1.7

# =============================================================================
# ESTAGIO 1 - BUILD
# =============================================================================
# Pinamos a versao do Go (nao usamos :latest) para que a build de hoje
# produza o mesmo resultado da build de daqui a seis meses.
FROM golang:1.22-alpine AS builder

# git e necessario para o go mod resolver alguns modulos.
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /src

# Copiamos APENAS os manifests primeiro. Como o Docker faz cache por
# camada, enquanto go.mod/go.sum nao mudarem o download das dependencias
# e reaproveitado, mesmo que todo o codigo Go tenha mudado.
# O glob go.sum* deixa o arquivo opcional na primeira build.
COPY go.mod go.sum* ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download && go mod verify

# Agora sim o codigo-fonte.
COPY . .

# Roda os testes DENTRO da build: uma imagem so e produzida se a suite passar.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go vet ./... && go test ./... -count=1

ARG VERSION=dev
ARG COMMIT=none

# CGO_ENABLED=0     -> binario estatico, roda em imagem sem libc
# -trimpath         -> remove caminhos da maquina de build (reproducibilidade)
# -ldflags="-s -w"  -> descarta tabela de simbolos e debug info (~30% menor)
# -X                -> injeta versao/commit nas variaveis do main
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
      -trimpath \
      -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
      -o /out/server . \
 && CGO_ENABLED=0 GOOS=linux go build \
      -trimpath -ldflags="-s -w" \
      -o /out/healthcheck ./cmd/healthcheck

# =============================================================================
# ESTAGIO 2 - RUNTIME
# =============================================================================
# distroless/static contem apenas certificados CA, timezone e /etc/passwd.
# Sem shell, sem gerenciador de pacotes, sem coreutils: a superficie de
# ataque cai drasticamente e nao ha binarios para um invasor reutilizar.
# A tag :nonroot ja define um usuario sem privilegios (uid 65532).
FROM gcr.io/distroless/static-debian12:nonroot

# Metadados no padrao OCI. Ficam visiveis em `docker inspect`.
LABEL org.opencontainers.image.title="http-server-projeto-korp" \
      org.opencontainers.image.description="Servico HTTP em Go do desafio Korp" \
      org.opencontainers.image.source="https://github.com/SEU_USUARIO/projeto-korp" \
      org.opencontainers.image.licenses="MIT"

WORKDIR /app

COPY --from=builder /out/server      /app/server
COPY --from=builder /out/healthcheck /app/healthcheck

# Redundante com a tag :nonroot, mas explicito: quem le o Dockerfile
# ve de imediato que o processo nao roda como root.
USER nonroot:nonroot

ENV PORT=8080 \
    LOG_LEVEL=info

EXPOSE 8080

# Usa o binario Go que compilamos, ja que nao ha curl/wget na imagem.
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/app/healthcheck"]

# Forma exec (array), nao shell. Assim o processo e PID 1 de verdade e
# recebe o SIGTERM do `docker stop` diretamente, permitindo o shutdown
# gracioso implementado no main.go.
ENTRYPOINT ["/app/server"]
