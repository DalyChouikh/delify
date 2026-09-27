package logging

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestRedactsCredentialsAndInteractionURLs(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{ReplaceAttr: Redact("secret-bot-token")}))
	logger.Error("request failed", "error", errors.New("Post https://discord.com/api/v10/interactions/123/temporary-token/callback: timeout"), "detail", "Bot secret-bot-token", "token", "voice-secret")
	logger.Error("webhook failed", "url", "https://discord.com/api/v10/webhooks/123/webhook-secret/messages/@original")
	for _, secret := range []string{"secret-bot-token", "temporary-token", "voice-secret", "webhook-secret"} {
		if strings.Contains(output.String(), secret) {
			t.Errorf("log contains %q", secret)
		}
	}
	if !strings.Contains(output.String(), "timeout") {
		t.Fatal("useful error context lost")
	}
}
