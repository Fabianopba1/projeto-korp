// Servico http-server-projeto-korp.
//
// Servidor HTTP em Go que expoe GET /projeto-korp devolvendo o nome do
// projeto e o horario atual em UTC, alem de metricas no padrao Prometheus.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/korp/http-server-projeto-korp/internal/handlers"
	"github.com/korp/http-server-projeto-korp/internal/metrics"
)

// Injetados em build time via -ldflags. Ver Dockerfile.
var (
	version = "dev"
	commit  = "none"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(getEnv("LOG_LEVEL", "info")),
	}))
	slog.SetDefault(logger)

	port := getEnv("PORT", "8080")

	metrics.Register(version, commit, runtime.Version())
	h := handlers.New(version, logger)

	mux := http.NewServeMux()

	// Cada rota e registrada com seu proprio middleware, passando o
	// padrao da rota como label. Isso mantem a cardinalidade das
	// metricas sob controle.
	mux.Handle("/projeto-korp", metrics.Middleware("/projeto-korp", http.HandlerFunc(h.ProjetoKorp)))
	mux.Handle("/health", metrics.Middleware("/health", http.HandlerFunc(h.Health)))
	mux.Handle("/ready", metrics.Middleware("/ready", http.HandlerFunc(h.Ready)))

	// /metrics fica fora do middleware de proposito: nao faz sentido
	// contabilizar o proprio scrape do Prometheus como trafego do servico.
	mux.Handle("/metrics", metrics.Handler())

	mux.Handle("/", metrics.Middleware("other", http.HandlerFunc(h.NotFound)))

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: requestLogger(logger, mux),

		// Timeouts explicitos. Sem eles uma conexao lenta pode segurar
		// um handler indefinidamente (Slowloris).
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MiB
	}

	// Sobe o servidor em goroutine para o main poder aguardar sinais.
	errCh := make(chan error, 1)
	go func() {
		logger.Info("servidor iniciado",
			slog.String("addr", srv.Addr),
			slog.String("version", version),
			slog.String("commit", commit),
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// Graceful shutdown: o Docker envia SIGTERM ao parar o container.
	// Sem tratar o sinal, conexoes em voo sao cortadas no meio.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		logger.Error("falha no servidor", slog.Any("erro", err))
		os.Exit(1)
	case sig := <-stop:
		logger.Info("sinal recebido, encerrando", slog.String("sinal", sig.String()))
	}

	metrics.Healthy.Set(0)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("shutdown forcado", slog.Any("erro", err))
		os.Exit(1)
	}
	logger.Info("servidor encerrado com sucesso")
}

// requestLogger emite um log estruturado por requisicao.
func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)

		logger.Info("requisicao",
			slog.String("metodo", r.Method),
			slog.String("path", r.URL.Path),
			// X-Forwarded-For e preenchido pelo NGINX; sem ele o IP
			// seria sempre o da rede docker.
			slog.String("client_ip", clientIP(r)),
			slog.String("duracao", time.Since(start).String()),
		)
	})
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	return r.RemoteAddr
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func parseLogLevel(s string) slog.Level {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(s)); err != nil {
		return slog.LevelInfo
	}
	return lvl
}
