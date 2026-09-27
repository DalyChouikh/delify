// Package logging prevents authentication material from appearing in logs.
package logging

import (
	"log/slog"
	"regexp"
	"strings"
)

var credentialURL = regexp.MustCompile(`(/(?:interactions|webhooks)/[0-9]+/)[^/?\s"\\]+`)

// Redact is a slog ReplaceAttr hook. Errors from net/http contain the complete
// request URL, including Discord interaction and webhook credentials.
func Redact(secrets ...string) func([]string, slog.Attr) slog.Attr {
	return func(_ []string, attr slog.Attr) slog.Attr {
		switch strings.ToLower(attr.Key) {
		case "token", "password", "authorization", "secret", "refresh_token", "client_secret":
			attr.Value = slog.StringValue("[REDACTED]")
			return attr
		}
		value := attr.Value.Resolve()
		var text string
		switch value.Kind() {
		case slog.KindString:
			text = value.String()
		case slog.KindAny:
			err, ok := value.Any().(error)
			if !ok {
				return attr
			}
			text = err.Error()
		default:
			return attr
		}
		text = credentialURL.ReplaceAllString(text, "${1}[REDACTED]")
		for _, secret := range secrets {
			if secret != "" {
				text = strings.ReplaceAll(text, secret, "[REDACTED]")
			}
		}
		attr.Value = slog.StringValue(text)
		return attr
	}
}
