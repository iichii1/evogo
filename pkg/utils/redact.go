package utils

import "net/url"

// RedactURL devolve a URL pronta para ir a um log: só esquema e host.
//
// Webhooks costumam levar segredos na própria URL (usuário e senha, token na
// query, token no caminho, como nos webhooks do Slack e do Telegram). O log
// mostra para onde o evento foi, e o resto fica de fora.
func RedactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "[url inválida]"
	}
	redacted := u.Scheme + "://" + u.Host
	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" {
		redacted += "/…"
	}
	return redacted
}
