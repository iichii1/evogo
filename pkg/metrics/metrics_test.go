package metrics

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

func TestCounterIsSafeForConcurrentUse(t *testing.T) {
	var c Counter
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.Inc()
			}
		}()
	}
	wg.Wait()
	c.Add(5)

	if got := c.Value(); got != 5005 {
		t.Fatalf("contador = %d, esperado 5005", got)
	}
}

func TestWriteUsesPrometheusTextFormat(t *testing.T) {
	var out bytes.Buffer
	err := Write(&out, []Metric{
		Gauge("evogo_clients", "Clientes ativos.", 3),
		{
			Name: "evogo_db_open_connections",
			Help: "Conexões abertas.",
			Kind: KindGauge,
			Samples: []Sample{
				{Labels: map[string]string{"pool": "users"}, Value: 2},
				{Labels: map[string]string{"pool": "auth"}, Value: 7},
			},
		},
		{
			Name:    "evogo_wait_seconds_total",
			Help:    "Tempo esperando.",
			Kind:    KindCounter,
			Samples: []Sample{{Value: 1.5}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	want := `# HELP evogo_clients Clientes ativos.
# TYPE evogo_clients gauge
evogo_clients 3
# HELP evogo_db_open_connections Conexões abertas.
# TYPE evogo_db_open_connections gauge
evogo_db_open_connections{pool="auth"} 7
evogo_db_open_connections{pool="users"} 2
# HELP evogo_wait_seconds_total Tempo esperando.
# TYPE evogo_wait_seconds_total counter
evogo_wait_seconds_total 1.5
`
	if out.String() != want {
		t.Fatalf("saída inesperada:\n%s\nesperado:\n%s", out.String(), want)
	}
}

func TestWriteEscapesLabelValuesAndHelp(t *testing.T) {
	var out bytes.Buffer
	err := Write(&out, []Metric{{
		Name:    "evogo_x",
		Help:    "linha1\nlinha2 \\",
		Kind:    KindGauge,
		Samples: []Sample{{Labels: map[string]string{"k": "a\"b\\c\nd"}, Value: 1}},
	}})
	if err != nil {
		t.Fatal(err)
	}

	got := out.String()
	for _, want := range []string{`# HELP evogo_x linha1\nlinha2 \\`, `evogo_x{k="a\"b\\c\nd"} 1`} {
		if !strings.Contains(got, want) {
			t.Errorf("saída não contém %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "\n") != 3 {
		t.Errorf("quebra de linha em rótulo ou ajuda vazou para a saída:\n%q", got)
	}
}

func TestProcessExposesTheProcessCounters(t *testing.T) {
	before := InstanceReconnects.Value()
	InstanceReconnects.Inc()
	t.Cleanup(func() { InstanceReconnects.n.Store(before) })

	var out bytes.Buffer
	if err := Write(&out, Process()); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{
		"evogo_instance_reconnects_total",
		"evogo_instance_disconnects_total",
		"evogo_webhook_delivered_total",
		"evogo_webhook_attempt_failures_total",
		"evogo_webhook_given_up_total",
		"evogo_send_queue_delayed_total",
		"evogo_send_queue_wait_milliseconds_total",
		"evogo_send_queue_rejected_full_total",
		"evogo_send_queue_rejected_timeout_total",
	} {
		if !strings.Contains(out.String(), "# TYPE "+name+" counter") {
			t.Errorf("métrica %s ausente:\n%s", name, out.String())
		}
	}
	if !strings.Contains(out.String(), "# TYPE evogo_send_queue_waiting gauge") {
		t.Errorf("gauge da fila ausente:\n%s", out.String())
	}
}
