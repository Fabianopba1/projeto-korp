# syntax=docker/dockerfile:1.7

FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /src

# Manifests primeiro: o download das dependencias fica em cache enquanto
# go.mod/go.sum nao mudarem. O glob deixa o go.sum opcional.
COPY go.mod go.sum* ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download && go mod verify

COPY . .

# A imagem so e produzida se vet e testes passarem.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go vet ./... && go test ./... -count=1

ARG VERSION=dev
ARG COMMIT=none

# CGO_ENABLED=0 gera binario estatico; -trimpath e -s -w reduzem tamanho
# e removem caminhos da maquina de build.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
      -trimpath \
      -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
      -o /out/server . \
 && CGO_ENABLED=0 GOOS=linux go build \
      -trimpath -ldflags="-s -w" \
      -o /out/healthcheck ./cmd/healthcheck

# distroless/static: so certificados, tzdata e /etc/passwd. Sem shell.
FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.title="http-server-projeto-korp" \
      org.opencontainers.image.description="Servico HTTP em Go do desafio Korp" \
      org.opencontainers.image.source="https://github.com/Fabianopba1/projeto-korp" \
      org.opencontainers.image.licenses="MIT"

WORKDIR /app

COPY --from=builder /out/server      /app/server
COPY --from=builder /out/healthcheck /app/healthcheck

USER nonroot:nonroot

ENV PORT=8080 \
    LOG_LEVEL=info

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/app/healthcheck"]

# Forma exec: o processo e PID 1 e recebe o SIGTERM do docker stop.
ENTRYPOINT ["/app/server"]
