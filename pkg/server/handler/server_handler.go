package server_handler

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gomessguii/logger"

	"github.com/evolution-foundation/evolution-go/pkg/metrics"
)

// defaultCheckTimeout limita quanto tempo o health check espera cada banco. Um
// orquestrador que consulta o endpoint a cada poucos segundos não pode ficar
// preso num banco que parou de responder.
const defaultCheckTimeout = 3 * time.Second

// ServerHandler cuida das rotas de infraestrutura do servidor.
type ServerHandler interface {
	ServerOk(ctx *gin.Context)
	Health(ctx *gin.Context)
	Metrics(ctx *gin.Context)
}

// Check é uma dependência cuja disponibilidade o health check confere.
type Check struct {
	Name string
	Ping func(ctx context.Context) error
}

// Pool é um pool de conexões com o banco, lido em /metrics.
type Pool struct {
	Name  string
	Stats func() sql.DBStats
}

// ClientCounts resume os clientes do WhatsApp mantidos em memória.
type ClientCounts struct {
	Total     int
	Connected int
	LoggedIn  int
}

// Dependencies é o que o handler lê do restante do servidor. Fica aqui, e não
// importado dos serviços, para o transporte não depender das regras de negócio.
type Dependencies struct {
	// Checks são conferidos em /health; qualquer falha deixa o servidor
	// "unavailable" (503).
	Checks []Check
	// Pools aparecem em /metrics.
	Pools []Pool
	// Clients devolve a contagem atual dos clientes do WhatsApp.
	Clients func() ClientCounts
	// CheckTimeout substitui o limite padrão de cada verificação (usado nos testes).
	CheckTimeout time.Duration
}

type serverHandler struct {
	deps Dependencies
}

// ServerOkResponse é a resposta de /server/ok.
type ServerOkResponse struct {
	Status string `json:"status" example:"ok"`
}

// HealthCheckResult é o resultado da verificação de uma dependência.
type HealthCheckResult struct {
	Status    string `json:"status" example:"up" enums:"up,down"`
	LatencyMs int64  `json:"latencyMs" example:"3"`
	// Error é uma descrição genérica; o motivo detalhado fica só no log do servidor.
	Error string `json:"error,omitempty" example:"ping failed"`
}

// HealthClients é a contagem dos clientes do WhatsApp no processo.
type HealthClients struct {
	Total     int `json:"total" example:"3"`
	Connected int `json:"connected" example:"2"`
	LoggedIn  int `json:"loggedIn" example:"2"`
}

// HealthResponse é a resposta de /health.
type HealthResponse struct {
	Status  string                       `json:"status" example:"ok" enums:"ok,unavailable"`
	Checks  map[string]HealthCheckResult `json:"checks"`
	Clients HealthClients                `json:"clients"`
}

// ServerOk responde que o processo está de pé, sem consultar nada.
// @Summary Server liveness
// @Description Responde 200 se o processo está de pé. Não consulta o banco nem as instâncias; para isso use /health.
// @Tags Server
// @Produce json
// @Success 200 {object} server_handler.ServerOkResponse
// @Router /server/ok [get]
func (s *serverHandler) ServerOk(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, ServerOkResponse{Status: "ok"})
}

// Health confere os bancos e informa quantas instâncias estão conectadas.
// @Summary Server health
// @Description Confere se os bancos de dados respondem e informa quantos clientes do WhatsApp estão em memória, conectados e logados. Responde 503 se algum banco não responde. Não exige apikey; não devolve detalhes dos erros.
// @Tags Server
// @Produce json
// @Success 200 {object} server_handler.HealthResponse
// @Failure 503 {object} server_handler.HealthResponse
// @Router /health [get]
func (s *serverHandler) Health(ctx *gin.Context) {
	timeout := s.deps.CheckTimeout
	if timeout <= 0 {
		timeout = defaultCheckTimeout
	}
	checkCtx, cancel := context.WithTimeout(ctx.Request.Context(), timeout)
	defer cancel()

	response := HealthResponse{Status: "ok", Checks: map[string]HealthCheckResult{}}

	for _, check := range s.deps.Checks {
		started := time.Now()
		err := check.Ping(checkCtx)
		result := HealthCheckResult{Status: "up", LatencyMs: time.Since(started).Milliseconds()}
		if err != nil {
			// O erro do driver pode trazer endereço e usuário do banco; o endpoint
			// é público, então o detalhe vai só para o log.
			logger.LogWarn("health check %q falhou: %v", check.Name, err)
			result.Status = "down"
			result.Error = "ping failed"
			response.Status = "unavailable"
		}
		response.Checks[check.Name] = result
	}

	if s.deps.Clients != nil {
		counts := s.deps.Clients()
		response.Clients = HealthClients{Total: counts.Total, Connected: counts.Connected, LoggedIn: counts.LoggedIn}
	}

	status := http.StatusOK
	if response.Status != "ok" {
		status = http.StatusServiceUnavailable
	}
	ctx.JSON(status, response)
}

// Metrics escreve as métricas no formato de texto do Prometheus.
// @Summary Prometheus metrics
// @Description Instâncias conectadas, conexões dos pools do banco, reconexões, quedas e falhas de webhook, no formato de texto do Prometheus. Exige a chave global no header apikey.
// @Tags Server
// @Produce plain
// @Param apikey header string true "Chave global (GLOBAL_API_KEY)"
// @Success 200 {string} string "métricas em texto"
// @Failure 401 {object} gin.H
// @Router /metrics [get]
func (s *serverHandler) Metrics(ctx *gin.Context) {
	var all []metrics.Metric

	if s.deps.Clients != nil {
		counts := s.deps.Clients()
		all = append(all,
			metrics.Gauge("evogo_clients", "Clientes do WhatsApp mantidos em memória.", float64(counts.Total)),
			metrics.Gauge("evogo_clients_connected", "Clientes com o websocket conectado.", float64(counts.Connected)),
			metrics.Gauge("evogo_clients_logged_in", "Clientes com a conta do WhatsApp logada.", float64(counts.LoggedIn)),
		)
	}

	all = append(all, poolMetrics(s.deps.Pools)...)
	all = append(all, metrics.Process()...)

	ctx.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	ctx.Status(http.StatusOK)
	if err := metrics.Write(ctx.Writer, all); err != nil {
		logger.LogWarn("falha ao escrever /metrics: %v", err)
	}
}

func poolMetrics(pools []Pool) []metrics.Metric {
	type series struct {
		name, help string
		kind       metrics.Kind
		value      func(sql.DBStats) float64
	}
	definitions := []series{
		{"evogo_db_max_open_connections", "Limite de conexões abertas do pool.", metrics.KindGauge, func(s sql.DBStats) float64 { return float64(s.MaxOpenConnections) }},
		{"evogo_db_open_connections", "Conexões abertas do pool, em uso e ociosas.", metrics.KindGauge, func(s sql.DBStats) float64 { return float64(s.OpenConnections) }},
		{"evogo_db_in_use_connections", "Conexões do pool em uso.", metrics.KindGauge, func(s sql.DBStats) float64 { return float64(s.InUse) }},
		{"evogo_db_idle_connections", "Conexões do pool ociosas.", metrics.KindGauge, func(s sql.DBStats) float64 { return float64(s.Idle) }},
		{"evogo_db_wait_count_total", "Vezes que se esperou por uma conexão do pool.", metrics.KindCounter, func(s sql.DBStats) float64 { return float64(s.WaitCount) }},
		{"evogo_db_wait_seconds_total", "Tempo total esperando por uma conexão do pool.", metrics.KindCounter, func(s sql.DBStats) float64 { return s.WaitDuration.Seconds() }},
	}

	out := make([]metrics.Metric, 0, len(definitions))
	for _, def := range definitions {
		metric := metrics.Metric{Name: def.name, Help: def.help, Kind: def.kind}
		for _, pool := range pools {
			if pool.Stats == nil {
				continue
			}
			metric.Samples = append(metric.Samples, metrics.Sample{
				Labels: map[string]string{"pool": pool.Name},
				Value:  def.value(pool.Stats()),
			})
		}
		if len(metric.Samples) > 0 {
			out = append(out, metric)
		}
	}
	return out
}

// NewServerHandler cria o handler das rotas de infraestrutura.
func NewServerHandler(deps Dependencies) ServerHandler {
	return &serverHandler{deps: deps}
}
