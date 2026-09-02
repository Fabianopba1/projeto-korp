package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// valor le o valor atual de um counter sem depender do pacote testutil,
// que traria uma dependencia nova ao go.mod.
func valor(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatalf("falha ao ler o counter: %v", err)
	}
	return m.GetCounter().GetValue()
}

func TestNormalizeMethod(t *testing.T) {
	casos := map[string]string{
		"GET":    "GET",
		"POST":   "POST",
		"HEAD":   "HEAD",
		"FOOBAR": "other",
		"get":    "other", // metodos sao case-sensitive no HTTP
		"":       "other",
	}
	for entrada, esperado := range casos {
		if got := normalizeMethod(entrada); got != esperado {
			t.Errorf("normalizeMethod(%q) = %q, esperado %q", entrada, got, esperado)
		}
	}
}

// Um cliente nao pode criar series novas inventando metodos HTTP.
func TestMiddlewareAgrupaMetodosDesconhecidos(t *testing.T) {
	h := Middleware("/teste", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, m := range []string{"FOOBAR", "BAZ", "QUX"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(m, "/teste", nil))
	}

	got := valor(t, RequestsTotal.WithLabelValues("other", "/teste", "204"))
	if got != 3 {
		t.Errorf("serie other/teste/204 = %v, esperado 3", got)
	}
	for _, m := range []string{"FOOBAR", "BAZ", "QUX"} {
		if n := valor(t, RequestsTotal.WithLabelValues(m, "/teste", "204")); n != 0 {
			t.Errorf("serie com metodo %q nao deveria existir, tem %v", m, n)
		}
	}
}
