package config

import "testing"

func TestEnvInt(t *testing.T) {
	cases := []struct {
		name, value string
		fallback    int
		want        int
	}{
		{"vazio usa o padrão", "", 100, 100},
		{"número", "2500", 0, 2500},
		{"zero vale", "0", 100, 0},
		{"negativo usa o padrão", "-5", 100, 100},
		{"texto usa o padrão", "dois mil", 100, 100},
		{"decimal usa o padrão", "1.5", 100, 100},
	}
	for _, c := range cases {
		t.Setenv("EVOGO_TEST_ENVINT", c.value)
		if got := envInt("EVOGO_TEST_ENVINT", c.fallback); got != c.want {
			t.Errorf("%s: envInt(%q, %d) = %d, esperado %d", c.name, c.value, c.fallback, got, c.want)
		}
	}
}
