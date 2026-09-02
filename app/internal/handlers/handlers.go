// Package handlers reune os endpoints HTTP do servico.
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// ProjetoKorpResponse e o contrato exigido pelo desafio.
// As tags json mantem os nomes exatos em portugues.
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

// Handler agrupa as dependencias injetadas nos endpoints.
// Manter "now" como campo permite congelar o tempo nos testes.
type Handler struct {
	Version   string
	StartedAt time.Time
	Now       func() time.Time
	Logger    *slog.Logger
}

// New cria um Handler com as dependencias padrao de producao.
func New(version string, logger *slog.Logger) *Handler {
	return &Handler{
		Version:   version,
		StartedAt: time.Now(),
		Now:       time.Now,
		Logger:    logger,
	}
}

// ProjetoKorp responde GET /projeto-korp.
//
// O horario e resolvido a cada requisicao (nada de cache) e convertido
// para UTC com .UTC(), garantindo o mesmo resultado independente do
// timezone do container ou do host.
func (h *Handler) ProjetoKorp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		h.writeError(w, http.StatusMethodNotAllowed, "metodo nao permitido")
		return
	}

	resp := ProjetoKorpResponse{
		Nome: "Projeto Korp",
		// RFC3339 e o formato interoperavel padrao. O sufixo "Z"
		// deixa explicito que o horario esta em UTC.
		Horario: h.Now().UTC().Format(time.RFC3339),
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// Health responde a liveness probe: o processo esta vivo?
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, HealthResponse{
		Status:  "ok",
		Uptime:  time.Since(h.StartedAt).Round(time.Second).String(),
		Version: h.Version,
	})
}

// Ready responde a readiness probe: o servico pode receber trafego?
//
// Neste projeto nao ha dependencias externas (banco, cache, fila), entao
// liveness e readiness coincidem. Elas ficam separadas porque no momento
// em que uma dependencia existir, so este handler precisa mudar.
func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, HealthResponse{
		Status:  "ready",
		Uptime:  time.Since(h.StartedAt).Round(time.Second).String(),
		Version: h.Version,
	})
}

// NotFound devolve 404 em JSON, mantendo o content-type consistente
// em toda a API.
func (h *Handler) NotFound(w http.ResponseWriter, r *http.Request) {
	h.writeError(w, http.StatusNotFound, "rota nao encontrada")
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		// O header ja foi enviado, entao nao da para trocar o status.
		// Resta registrar para o operador.
		h.Logger.Error("falha ao serializar resposta", slog.Any("erro", err))
	}
}

func (h *Handler) writeError(w http.ResponseWriter, status int, msg string) {
	h.writeJSON(w, status, map[string]string{"erro": msg})
}
