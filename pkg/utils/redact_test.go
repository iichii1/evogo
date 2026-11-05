package utils

import (
	"strings"
	"testing"
)

func TestRedactURL(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"só o host", "https://hooks.example.com", "https://hooks.example.com"},
		{"raiz", "https://hooks.example.com/", "https://hooks.example.com"},
		{"caminho", "https://hooks.example.com/services/T0/B0/XXXX", "https://hooks.example.com/…"},
		{"query", "https://hooks.example.com/in?token=abc", "https://hooks.example.com/…"},
		{"só query", "https://hooks.example.com?token=abc", "https://hooks.example.com/…"},
		{"usuário e senha", "http://user:s3cret@10.0.0.2:8080/in", "http://10.0.0.2:8080/…"},
		{"porta", "http://crm:3000/webhook", "http://crm:3000/…"},
		{"sem esquema", "hooks.example.com/in", "[url inválida]"},
		{"vazia", "", "[url inválida]"},
		{"lixo", "://%%", "[url inválida]"},
	}
	for _, c := range cases {
		if got := RedactURL(c.in); got != c.want {
			t.Errorf("%s: RedactURL(%q) = %q, esperado %q", c.name, c.in, got, c.want)
		}
	}

	for _, secret := range []string{"s3cret", "abc", "XXXX"} {
		for _, c := range cases {
			if strings.Contains(RedactURL(c.in), secret) {
				t.Errorf("RedactURL(%q) vazou %q", c.in, secret)
			}
		}
	}
}
