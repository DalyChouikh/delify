package lavalink

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/DalyChouikh/delify/internal/config"
)

func TestHealthAuthenticatesAndRejectsUnauthorized(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusUnauthorized} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			var authorized bool
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				authorized = r.Header.Get("Authorization") == "test-password"
				w.WriteHeader(status)
			}))
			defer s.Close()
			host, port, _ := net.SplitHostPort(s.Listener.Addr().String())
			portNumber, _ := strconv.Atoi(port)
			c := Client{config: config.LavalinkConfig{Host: host, Port: portNumber, Password: "test-password"}}
			err := c.checkLavalinkHealth(context.Background())
			if !authorized {
				t.Error("health check did not send authentication")
			}
			if status == http.StatusUnauthorized && err == nil {
				t.Error("invalid password accepted as healthy")
			}
			if status == http.StatusOK && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestConnectCancellationInterruptsRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancel()
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer s.Close()
	host, port, _ := net.SplitHostPort(s.Listener.Addr().String())
	portNumber, _ := strconv.Atoi(port)
	c := Client{config: config.LavalinkConfig{Host: host, Port: portNumber}, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	start := time.Now()
	if err := c.Connect(ctx); err == nil {
		t.Fatal("expected cancellation")
	}
	if time.Since(start) > time.Second {
		t.Fatal("cancellation waited for the retry delay")
	}
}
