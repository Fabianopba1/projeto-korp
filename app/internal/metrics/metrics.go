// Package metrics concentra a instrumentacao Prometheus do servico.
// Namespace "korp", contadores com sufixo _total, unidades base no nome e
// apenas labels de baixa cardinalidade.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const namespace = "korp"

var (
	// Volume de requisicoes (requisito do desafio).
	RequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "http_requests_total",
			Help:      "Total de requisicoes HTTP recebidas, por metodo, rota e status.",
		},
		[]string{"method", "path", "status"},
	)

	// Latencia por quantil via histogram_quantile().
	RequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_request_duration_seconds",
			Help:      "Duracao das requisicoes HTTP em segundos.",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		},
		[]string{"method", "path"},
	)

	InFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "http_requests_in_flight",
			Help:      "Numero de requisicoes HTTP sendo processadas neste instante.",
		},
	)

	// Volta a zero num restart: detecta crash loop rapido demais para o
	// `up` do Prometheus registrar.
	UpTimeSeconds = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "uptime_seconds",
			Help:      "Segundos desde a inicializacao do processo.",
		},
		func() float64 { return time.Since(startedAt).Seconds() },
	)

	// Info metric: valor sempre 1, a informacao esta nos labels.
	BuildInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "build_info",
			Help:      "Metadados da build. O valor e sempre 1.",
		},
		[]string{"version", "commit", "go_version"},
	)

	Healthy = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "service_healthy",
			Help:      "1 quando o servico esta saudavel, 0 quando degradado.",
		},
	)

	startedAt = time.Now()

	// Registry proprio: tudo que e exposto esta listado em Register.
	Registry = prometheus.NewRegistry()
)

func Register(version, commit, goVersion string) {
	Registry.MustRegister(
		RequestsTotal,
		RequestDuration,
		InFlight,
		UpTimeSeconds,
		BuildInfo,
		Healthy,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	BuildInfo.WithLabelValues(version, commit, goVersion).Set(1)
	Healthy.Set(1)
}

func Handler() http.Handler {
	return promhttp.HandlerFor(Registry, promhttp.HandlerOpts{
		Registry:          Registry,
		EnableOpenMetrics: true,
	})
}

// statusRecorder captura o status code, que o ResponseWriter nao expoe.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

var knownMethods = map[string]struct{}{
	http.MethodGet:     {},
	http.MethodHead:    {},
	http.MethodPost:    {},
	http.MethodPut:     {},
	http.MethodPatch:   {},
	http.MethodDelete:  {},
	http.MethodOptions: {},
}

// normalizeMethod limita o label "method" a uma lista fechada. O servidor
// HTTP do Go aceita qualquer token como metodo, e cada valor novo criaria
// uma serie no counter e uma por bucket no histograma.
func normalizeMethod(m string) string {
	if _, ok := knownMethods[m]; ok {
		return m
	}
	return "other"
}

// Middleware instrumenta um handler. O label "path" recebe a rota
// registrada, nao r.URL.Path, pelo mesmo motivo de normalizeMethod.
func Middleware(routePattern string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		InFlight.Inc()
		defer InFlight.Dec()

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		method := normalizeMethod(r.Method)
		elapsed := time.Since(start).Seconds()
		RequestsTotal.WithLabelValues(method, routePattern, strconv.Itoa(rec.status)).Inc()
		RequestDuration.WithLabelValues(method, routePattern).Observe(elapsed)
	})
}
