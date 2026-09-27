// Delify - A Discord Music Bot
//
// This is the main entry point for the Delify music bot.
// It handles initialization, signal handling, and graceful shutdown.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/DalyChouikh/delify/internal/bot"
	"github.com/DalyChouikh/delify/internal/config"
	"github.com/DalyChouikh/delify/internal/logging"
)

func main() {
	cfg := config.Load()
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		slog.Error("invalid LOG_LEVEL", "value", cfg.LogLevel)
		os.Exit(1)
	}
	// Initialize structured logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: logging.Redact(cfg.Discord.Token, cfg.Lavalink.Password, cfg.Lyrics.RapidAPIKey),
	}))
	slog.SetDefault(logger)

	logger.Info("starting Delify Music Bot...")

	// Load configuration
	if err := cfg.Validate(); err != nil {
		logger.Error("configuration error", "error", err)
		os.Exit(1)
	}

	// Create bot instance
	musicBot, err := bot.New(cfg, logger)
	if err != nil {
		logger.Error("failed to create bot", "error", err)
		os.Exit(1)
	}

	// Install signal handling before startup so Docker can stop a bot that is
	// still connecting or registering commands.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Start the bot
	if err := musicBot.Start(ctx); err != nil {
		_ = musicBot.Stop()
		if ctx.Err() != nil {
			return
		}
		logger.Error("failed to start bot", "error", err)
		os.Exit(1)
	}

	// Wait for shutdown signal
	logger.Info("bot is running. Press Ctrl+C to stop.")
	<-ctx.Done()

	// Graceful shutdown
	logger.Info("received shutdown signal")
	if err := musicBot.Stop(); err != nil {
		logger.Error("error during shutdown", "error", err)
		os.Exit(1)
	}

	logger.Info("bot stopped successfully")
}
