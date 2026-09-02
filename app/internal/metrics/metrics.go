// Package metrics concentra a instrumentacao Prometheus do servico.
//
// Convencoes seguidas (https://prometheus.io/docs/practices/naming/):
//   - namespace unico "korp" para evitar colisao com metricas de runtime;
//   - contadores terminam em _total;
//   - unidades base no nome (seconds, bytes);
//   - labels de baixa cardinalidade apenas.
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
	// RequestsTotal atende ao requisito "volume de requisicoes".
	// Counter e o tipo correto: so cresce e e resiliente a restart
	// (o Prometheus detecta o reset via rate()/increase()).
	RequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "http_requests_total",
			Help:      "Total de requisicoes HTTP recebidas, por metodo, rota e status.",
		},
		[]string{"method", "path", "status"},
	)

	// RequestDuration permite calcular latencia por quantil no Grafana
	// via histogram_quantile(). Nao e obrigatorio no desafio, mas e o
	// terceiro sinal classico (RED: Rate, Errors, Duration).
	RequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_request_duration_seconds",
			Help:      "Duracao das requisicoes HTTP em segundos.",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		},
		[]string{"method", "path"},
	)

	// InFlight mostra concorrencia instantanea. Gauge sobe e desce.
	InFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "http_requests_in_flight",
			Help:      "Numero de requisicoes HTTP sendo processadas neste instante.",
		},
	)

	// UpTimeSeconds complementa a disponibilidade: permite detectar
	// restarts (o valor volta a zero) mesmo que o alvo nunca fique down
	// entre dois scrapes.
	UpTimeSeconds = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "uptime_seconds",
			Help:      "Segundos desde a inicializacao do processo.",
		},
		func() float64 { return time.Since(startedAt).Seconds() },
	)

	// BuildInfo e o padrao "info metric": valor sempre 1, a informacao
	// util fica nos labels. Serve para correlacionar versao x incidente.
	BuildInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "build_info",
			Help:      "Metadados da build. O valor e sempre 1.",
		},
		[]string{"version", "commit", "go_version"},
	)

	// Healthy expoe a disponibilidade percebida pela propria aplicacao.
	// 1 = pronto para receber trafego, 0 = degradado.
	Healthy = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "service_healthy",
			Help:      "1 quando o servico esta saudavel, 0 quando degradado.",
		},
	)

	startedAt = time.Now()

	// Registry proprio em vez do DefaultRegisterer: evita metricas
	// registradas por dependencias sem que a gente perceba e deixa
	// explicito tudo que e exposto.
	Registry = prometheus.NewRegistry()
)

// Register inscreve todos os coletores no registry da aplicacao.
func Register(version, commit, goVersion string) {
	Registry.MustRegister(
		RequestsTotal,
		RequestDuration,
		InFlight,
		UpTimeSeconds,
		BuildInfo,
		Healthy,
		// Coletores padrao: CPU, memoria, GC, goroutines, file descriptors.
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	BuildInfo.WithLabelValues(version, commit, goVersion).Set(1)
	Healthy.Set(1)
}

// Handler devolve o handler HTTP do endpoint /metrics.
func Handler() http.Handler {
	return promhttp.HandlerFor(Registry, promhttp.HandlerOpts{
		Registry:          Registry,
		EnableOpenMetrics: true,
	})
}

// statusRecorder captura o status code, que o http.ResponseWriter
// padrao nao expoe depois de escrito.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// knownMethods e a lista fechada de valores aceitos no label "method".
var knownMethods = map[string]struct{}{
	http.MethodGet:     {},
	http.MethodHead:    {},
	http.MethodPost:    {},
	http.MethodPut:     {},
	http.MethodPatch:   {},
	http.MethodDelete:  {},
	http.MethodOptions: {},
}

// normalizeMethod devolve o metodo se ele for conhecido, ou "other".
//
// O servidor HTTP do Go aceita qualquer token como metodo (FOOBAR, BAZ...),
// e cada valor novo criaria uma serie no counter e uma por bucket no
// histograma. Sem esta normalizacao um cliente conseguiria o mesmo efeito
// de cardinalidade infinita que o label "path" evita.
func normalizeMethod(m string) string {
	if _, ok := knownMethods[m]; ok {
		return m
	}
	return "other"
}

// Middleware instrumenta qualquer handler HTTP.
//
// Importante: o label "path" recebe a rota registrada (ex: /projeto-korp)
// e nao r.URL.Path. Usar a URL crua permitiria a um cliente gerar
// cardinalidade infinita (/a, /b, /c...) e derrubar o Prometheus. O label
// "method" passa pelo mesmo tratamento em normalizeMethod.
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
