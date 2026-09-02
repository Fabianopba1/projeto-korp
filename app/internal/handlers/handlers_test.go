package handlers

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestHandler() *Handler {
	return New("test", slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// Valida o contrato exato exigido pelo desafio.
func TestProjetoKorpRetornaContratoEsperado(t *testing.T) {
	h := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/projeto-korp", nil)
	rec := httptest.NewRecorder()

	h.ProjetoKorp(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusOK)
	}

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, esperado application/json", ct)
	}

	var body ProjetoKorpResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("resposta nao e JSON valido: %v", err)
	}

	if body.Nome != "Projeto Korp" {
		t.Errorf("nome = %q, esperado %q", body.Nome, "Projeto Korp")
	}
	if body.Horario == "" {
		t.Fatal("campo horario veio vazio")
	}
}

// O desafio exige explicitamente horario em UTC.
func TestHorarioEstaEmUTC(t *testing.T) {
	h := newTestHandler()

	// Congela o relogio num fuso deslocado (-03:00, horario de Brasilia).
	// Se o handler nao converter para UTC, o teste quebra.
	fuso := time.FixedZone("America/Sao_Paulo", -3*3600)
	h.Now = func() time.Time {
		return time.Date(2026, 3, 15, 9, 30, 0, 0, fuso)
	}

	rec := httptest.NewRecorder()
	h.ProjetoKorp(rec, httptest.NewRequest(http.MethodGet, "/projeto-korp", nil))

	var body ProjetoKorpResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSON invalido: %v", err)
	}

	// 09:30 em -03:00 equivale a 12:30 UTC.
	const esperado = "2026-03-15T12:30:00Z"
	if body.Horario != esperado {
		t.Errorf("horario = %q, esperado %q", body.Horario, esperado)
	}
}

// O desafio pede que o horario seja resolvido dinamicamente a cada
// requisicao, ou seja: nada de valor calculado uma vez na inicializacao.
func TestHorarioEhResolvidoACadaRequisicao(t *testing.T) {
	h := newTestHandler()

	chamadas := 0
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	h.Now = func() time.Time {
		chamadas++
		return base.Add(time.Duration(chamadas) * time.Second)
	}

	horarios := make([]string, 2)
	for i := range horarios {
		rec := httptest.NewRecorder()
		h.ProjetoKorp(rec, httptest.NewRequest(http.MethodGet, "/projeto-korp", nil))

		var body ProjetoKorpResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("JSON invalido: %v", err)
		}
		horarios[i] = body.Horario
	}

	if horarios[0] == horarios[1] {
		t.Errorf("horario nao mudou entre requisicoes: %q", horarios[0])
	}
}

func TestMetodoNaoPermitido(t *testing.T) {
	h := newTestHandler()

	rec := httptest.NewRecorder()
	h.ProjetoKorp(rec, httptest.NewRequest(http.MethodPost, "/projeto-korp", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, esperado %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if allow := rec.Header().Get("Allow"); allow != http.MethodGet {
		t.Errorf("header Allow = %q, esperado %q", allow, http.MethodGet)
	}
}

func TestHealthEReady(t *testing.T) {
	h := newTestHandler()

	casos := map[string]http.HandlerFunc{
		"/health": h.Health,
		"/ready":  h.Ready,
	}

	for rota, fn := range casos {
		rec := httptest.NewRecorder()
		fn(rec, httptest.NewRequest(http.MethodGet, rota, nil))

		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, esperado 200", rota, rec.Code)
		}

		var body HealthResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Errorf("%s: JSON invalido: %v", rota, err)
		}
		if body.Status == "" {
			t.Errorf("%s: campo status vazio", rota)
		}
	}
}
