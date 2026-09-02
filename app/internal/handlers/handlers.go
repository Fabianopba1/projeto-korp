// Package handlers reune os endpoints HTTP do servico.
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// ProjetoKorpResponse e o contrato exigido pelo desafio.
type ProjetoKorpResponse struct {
	Nome    string `json:"nome"`
	Horario string `json:"horario"`
}

// HealthResponse e usada por /health e /ready.
type HealthResponse struct {
	Status  string `json:"status"`
	Uptime  string `json:"uptime"`
	Version string `json:"version"`
}

// Handler agrupa as dependencias dos endpoints. Now e um campo para que
// os testes possam congelar o relogio.
type Handler struct {
	Version   string
	StartedAt time.Time
	Now       func() time.Time
	Logger    *slog.Logger
}

func New(version string, logger *slog.Logger) *Handler {
	return &Handler{
		Version:   version,
		StartedAt: time.Now(),
		Now:       time.Now,
		Logger:    logger,
	}
}

// ProjetoKorp responde GET /projeto-korp. O horario e lido a cada
// requisicao e convertido para UTC, independente do fuso do host.
func (h *Handler) ProjetoKorp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		h.writeError(w, http.StatusMethodNotAllowed, "metodo nao permitido")
		return
	}

	resp := ProjetoKorpResponse{
		Nome:    "Projeto Korp",
		Horario: h.Now().UTC().Format(time.RFC3339),
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// Health e a liveness probe.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, HealthResponse{
		Status:  "ok",
		Uptime:  time.Since(h.StartedAt).Round(time.Second).String(),
		Version: h.Version,
	})
}

// Ready e a readiness probe. Hoje coincide com Health porque nao ha
// dependencias externas; fica separada para quando houver.
func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, HealthResponse{
		Status:  "ready",
		Uptime:  time.Since(h.StartedAt).Round(time.Second).String(),
		Version: h.Version,
	})
}

func (h *Handler) NotFound(w http.ResponseWriter, r *http.Request) {
	h.writeError(w, http.StatusNotFound, "rota nao encontrada")
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		h.Logger.Error("falha ao serializar resposta", slog.Any("erro", err))
	}
}

func (h *Handler) writeError(w http.ResponseWriter, status int, msg string) {
	h.writeJSON(w, status, map[string]string{"erro": msg})
}
