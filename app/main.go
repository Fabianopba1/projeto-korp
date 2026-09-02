// http-server-projeto-korp: expoe GET /projeto-korp com o nome do projeto
// e o horario atual em UTC, alem de metricas no formato Prometheus.
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

// Injetados via -ldflags no build (ver Dockerfile).
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

	// O padrao da rota vira o label "path" das metricas, nunca a URL crua.
	mux.Handle("/projeto-korp", metrics.Middleware("/projeto-korp", http.HandlerFunc(h.ProjetoKorp)))
	mux.Handle("/health", metrics.Middleware("/health", http.HandlerFunc(h.Health)))
	mux.Handle("/ready", metrics.Middleware("/ready", http.HandlerFunc(h.Ready)))

	// Fora do middleware: o scrape do Prometheus nao conta como trafego.
	mux.Handle("/metrics", metrics.Handler())

	mux.Handle("/", metrics.Middleware("other", http.HandlerFunc(h.NotFound)))

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           requestLogger(logger, mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

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

	// docker stop envia SIGTERM: drena as conexoes em voo antes de sair.
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

func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)

		logger.Info("requisicao",
			slog.String("metodo", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("client_ip", clientIP(r)),
			slog.String("duracao", time.Since(start).String()),
		)
	})
}

// clientIP prefere o X-Forwarded-For preenchido pelo nginx; sem ele o IP
// seria sempre o da rede docker.
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
