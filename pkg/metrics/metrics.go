// Package metrics guarda os contadores do processo e escreve as métricas no
// formato de texto do Prometheus.
//
// Os bugs mais graves do upstream (vazamento de pool de conexões, loop de
// reconexão) só apareciam tarde porque nada media isso. Aqui ficam só contadores
// atômicos, sem dependência nova: os valores que mudam pouco (instâncias
// conectadas, conexões do pool) são lidos na hora da coleta pelo handler de
// /metrics.
package metrics

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync/atomic"
)

// Counter é um contador que só cresce e pode ser usado de qualquer goroutine.
type Counter struct {
	n atomic.Uint64
}

// Inc soma 1.
func (c *Counter) Inc() { c.n.Add(1) }

// Add soma n.
func (c *Counter) Add(n uint64) { c.n.Add(n) }

// Value devolve o valor atual.
func (c *Counter) Value() uint64 { return c.n.Load() }

// Contadores do processo. Quem produz o evento chama Inc; /metrics lê.
var (
	// InstanceReconnects conta cada reconexão de cliente iniciada (automática ou
	// pedida pela API).
	InstanceReconnects Counter
	// InstanceDisconnects conta as quedas inesperadas (events.Disconnected). Uma
	// instância que cai em loop aparece aqui antes de aparecer nos logs.
	InstanceDisconnects Counter
	// WebhookDelivered conta os webhooks entregues com resposta 2xx.
	WebhookDelivered Counter
	// WebhookAttemptFailures conta cada tentativa de webhook que falhou (o
	// produtor tenta até 5 vezes).
	WebhookAttemptFailures Counter
	// WebhookGivenUp conta os webhooks descartados depois da última tentativa.
	WebhookGivenUp Counter
)

// Kind é o tipo da métrica no formato Prometheus.
type Kind string

const (
	KindCounter Kind = "counter"
	KindGauge   Kind = "gauge"
)

// Sample é um valor de uma métrica, com rótulos opcionais.
type Sample struct {
	Labels map[string]string
	Value  float64
}

// Metric é uma métrica com todas as suas amostras.
type Metric struct {
	Name    string
	Help    string
	Kind    Kind
	Samples []Sample
}

// Gauge cria uma métrica de tipo gauge com uma amostra sem rótulos.
func Gauge(name, help string, value float64) Metric {
	return Metric{Name: name, Help: help, Kind: KindGauge, Samples: []Sample{{Value: value}}}
}

// Process devolve os contadores do processo como métricas.
func Process() []Metric {
	counter := func(name, help string, c *Counter) Metric {
		return Metric{Name: name, Help: help, Kind: KindCounter, Samples: []Sample{{Value: float64(c.Value())}}}
	}
	return []Metric{
		counter("evogo_instance_reconnects_total", "Reconexões de cliente iniciadas, automáticas ou pedidas pela API.", &InstanceReconnects),
		counter("evogo_instance_disconnects_total", "Quedas inesperadas de conexão com o WhatsApp.", &InstanceDisconnects),
		counter("evogo_webhook_delivered_total", "Webhooks entregues com resposta 2xx.", &WebhookDelivered),
		counter("evogo_webhook_attempt_failures_total", "Tentativas de webhook que falharam.", &WebhookAttemptFailures),
		counter("evogo_webhook_given_up_total", "Webhooks descartados depois da última tentativa.", &WebhookGivenUp),
	}
}

// Write escreve as métricas no formato de texto do Prometheus (versão 0.0.4).
// As amostras de cada métrica saem ordenadas pelos rótulos, para a saída ser
// estável.
func Write(w io.Writer, metrics []Metric) error {
	for _, m := range metrics {
		if _, err := fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n", m.Name, escapeHelp(m.Help), m.Name, m.Kind); err != nil {
			return err
		}

		lines := make([]string, 0, len(m.Samples))
		for _, s := range m.Samples {
			lines = append(lines, fmt.Sprintf("%s%s %s", m.Name, formatLabels(s.Labels), formatValue(s.Value)))
		}
		sort.Strings(lines)
		for _, line := range lines {
			if _, err := fmt.Fprintln(w, line); err != nil {
				return err
			}
		}
	}
	return nil
}

func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	names := make([]string, 0, len(labels))
	for name := range labels {
		names = append(names, name)
	}
	sort.Strings(names)

	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, name, escapeLabelValue(labels[name])))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func formatValue(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%g", v)
}

var (
	helpEscaper  = strings.NewReplacer(`\`, `\\`, "\n", `\n`)
	labelEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
)

func escapeHelp(s string) string       { return helpEscaper.Replace(s) }
func escapeLabelValue(s string) string { return labelEscaper.Replace(s) }
