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
)

func main() {
	// Initialize structured logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	logger.Info("starting Delify Music Bot...")

	// Load configuration
	cfg := config.Load()
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

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the bot
	if err := musicBot.Start(ctx); err != nil {
		logger.Error("failed to start bot", "error", err)
		os.Exit(1)
	}

	// Wait for shutdown signal
	logger.Info("bot is running. Press Ctrl+C to stop.")
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	// Graceful shutdown
	logger.Info("received shutdown signal")
	if err := musicBot.Stop(); err != nil {
		logger.Error("error during shutdown", "error", err)
		os.Exit(1)
	}

	logger.Info("bot stopped successfully")
}
