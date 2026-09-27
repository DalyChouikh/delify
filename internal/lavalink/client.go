// Package lavalink handles the connection to the Lavalink audio server.
package lavalink

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/DalyChouikh/delify/internal/config"
	"github.com/bwmarrin/discordgo"
	"github.com/disgoorg/disgolink/v3/disgolink"
	"github.com/disgoorg/disgolink/v3/lavalink"
	"github.com/disgoorg/snowflake/v2"
)

// Client wraps the disgolink client with additional functionality.
type Client struct {
	Link   disgolink.Client
	config config.LavalinkConfig
	logger *slog.Logger
}

// NewClient creates a new Lavalink client wrapper.
func NewClient(session *discordgo.Session, cfg config.LavalinkConfig, logger *slog.Logger) (*Client, error) {
	// Create the disgolink client
	link := disgolink.New(
		snowflake.ID(mustParseSnowflake(session.State.User.ID)),
		disgolink.WithLogger(logger),
		disgolink.WithListenerFunc(onPlayerPause),
		disgolink.WithListenerFunc(onPlayerResume),
		disgolink.WithListenerFunc(onTrackEnd),
		disgolink.WithListenerFunc(onTrackStuck),
		disgolink.WithListenerFunc(onWebSocketClosed),
	)

	return &Client{
		Link:   link,
		config: cfg,
		logger: logger,
	}, nil
}

// Connect establishes connection to the Lavalink server with retry logic.
func (c *Client) Connect(ctx context.Context) error {
	nodeConfig := disgolink.NodeConfig{
		Name:     "main",
		Address:  fmt.Sprintf("%s:%d", c.config.Host, c.config.Port),
		Password: c.config.Password,
		Secure:   c.config.Secure,
	}

	maxRetries := 30
	retryDelay := 2 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		c.logger.Info("attempting to connect to Lavalink",
			"attempt", attempt,
			"max_retries", maxRetries,
			"address", nodeConfig.Address,
		)

		// Check if Lavalink is ready
		if err := c.checkLavalinkHealth(ctx); err != nil {
			c.logger.Warn("Lavalink not ready, retrying...",
				"error", err,
				"retry_in", retryDelay,
			)
			if err := waitForRetry(ctx, retryDelay); err != nil {
				return err
			}
			continue
		}

		// Try to add the node
		_, err := c.Link.AddNode(ctx, nodeConfig)
		if err != nil {
			c.logger.Warn("failed to add Lavalink node, retrying...",
				"error", err,
				"retry_in", retryDelay,
			)
			if err := waitForRetry(ctx, retryDelay); err != nil {
				return err
			}
			continue
		}

		c.logger.Info("successfully connected to Lavalink",
			"address", nodeConfig.Address,
		)
		return nil
	}

	return fmt.Errorf("failed to connect to Lavalink after %d attempts", maxRetries)
}

// checkLavalinkHealth verifies the Lavalink server is responding.
func (c *Client) checkLavalinkHealth(ctx context.Context) error {
	protocol := "http"
	if c.config.Secure {
		protocol = "https"
	}

	url := fmt.Sprintf("%s://%s:%d/version", protocol, c.config.Host, c.config.Port)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.config.Password)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Close gracefully shuts down the Lavalink client.
func (c *Client) Close() {
	c.Link.Close()
}

// mustParseSnowflake parses a string to snowflake ID, panics on error.
func mustParseSnowflake(s string) uint64 {
	id, err := snowflake.Parse(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse snowflake: %v", err))
	}
	return uint64(id)
}

// Event handlers for Lavalink events
func onPlayerPause(player disgolink.Player, event lavalink.PlayerPauseEvent) {
	slog.Debug("player paused", "guild", player.GuildID())
}

func onPlayerResume(player disgolink.Player, event lavalink.PlayerResumeEvent) {
	slog.Debug("player resumed", "guild", player.GuildID())
}

func onTrackStart(player disgolink.Player, event lavalink.TrackStartEvent) {
	slog.Info("track started",
		"guild", player.GuildID(),
		"track", event.Track.Info.Title,
	)
}

func onTrackEnd(player disgolink.Player, event lavalink.TrackEndEvent) {
	slog.Info("track ended",
		"guild", player.GuildID(),
		"track", event.Track.Info.Title,
		"reason", event.Reason,
	)
}

func onTrackException(player disgolink.Player, event lavalink.TrackExceptionEvent) {
	slog.Error("track exception",
		"guild", player.GuildID(),
		"track", event.Track.Info.Title,
		"error", event.Exception.Message,
	)
}

func onTrackStuck(player disgolink.Player, event lavalink.TrackStuckEvent) {
	slog.Warn("track stuck",
		"guild", player.GuildID(),
		"track", event.Track.Info.Title,
		"threshold", event.Threshold,
	)
}

func onWebSocketClosed(player disgolink.Player, event lavalink.WebSocketClosedEvent) {
	slog.Warn("websocket closed",
		"guild", player.GuildID(),
		"code", event.Code,
		"reason", event.Reason,
	)
}
