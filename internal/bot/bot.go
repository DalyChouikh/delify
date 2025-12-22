// Package bot provides Discord bot initialization and lifecycle management.
package bot

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/bwmarrin/discordgo"
	"github.com/DalyChouikh/delify/internal/commands"
	"github.com/DalyChouikh/delify/internal/config"
	"github.com/DalyChouikh/delify/internal/lavalink"
	"github.com/DalyChouikh/delify/internal/player"
	"github.com/disgoorg/disgolink/v3/disgolink"
	"github.com/disgoorg/snowflake/v2"
)

// Bot represents the Discord music bot.
type Bot struct {
	session        *discordgo.Session
	lavalinkClient *lavalink.Client
	playerManager  *player.Manager
	commandHandler *commands.Handler
	config         *config.Config
	logger         *slog.Logger
}

// New creates a new Discord bot instance.
func New(cfg *config.Config, logger *slog.Logger) (*Bot, error) {
	// Create Discord session
	session, err := discordgo.New("Bot " + cfg.Discord.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to create Discord session: %w", err)
	}

	// Set intents
	session.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildVoiceStates |
		discordgo.IntentsGuildMessages

	bot := &Bot{
		session: session,
		config:  cfg,
		logger:  logger,
	}

	return bot, nil
}

// Start initializes and starts the bot.
func (b *Bot) Start(ctx context.Context) error {
	// Add voice state update handler for disgolink
	b.session.AddHandler(b.handleVoiceStateUpdate)
	b.session.AddHandler(b.handleVoiceServerUpdate)

	// Open Discord connection
	if err := b.session.Open(); err != nil {
		return fmt.Errorf("failed to open Discord session: %w", err)
	}
	b.logger.Info("connected to Discord", "user", b.session.State.User.Username)

	// Create Lavalink client
	lavalinkClient, err := lavalink.NewClient(b.session, b.config.Lavalink, b.logger)
	if err != nil {
		return fmt.Errorf("failed to create Lavalink client: %w", err)
	}
	b.lavalinkClient = lavalinkClient

	// Connect to Lavalink with retry
	if err := b.lavalinkClient.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to Lavalink: %w", err)
	}

	// Create player manager
	b.playerManager = player.NewManager(b.lavalinkClient.Link, b.session, b.logger)

	// Create command handler
	b.commandHandler = commands.NewHandler(b.session, b.playerManager, b.config, b.logger)

	// Register slash commands
	if err := b.commandHandler.Register(b.config.Discord.GuildID); err != nil {
		return fmt.Errorf("failed to register commands: %w", err)
	}

	// Add interaction handler
	b.session.AddHandler(b.commandHandler.HandleInteraction)

	b.logger.Info("bot is ready!", "guilds", len(b.session.State.Guilds))
	return nil
}

// Stop gracefully shuts down the bot.
func (b *Bot) Stop() error {
	b.logger.Info("shutting down bot...")

	if b.lavalinkClient != nil {
		b.lavalinkClient.Close()
	}

	if b.session != nil {
		if err := b.session.Close(); err != nil {
			return fmt.Errorf("failed to close Discord session: %w", err)
		}
	}

	return nil
}

// handleVoiceStateUpdate forwards voice state updates to disgolink.
func (b *Bot) handleVoiceStateUpdate(s *discordgo.Session, event *discordgo.VoiceStateUpdate) {
	if event.UserID != s.State.User.ID {
		return
	}

	guildID, err := snowflake.Parse(event.GuildID)
	if err != nil {
		return
	}

	var channelID *snowflake.ID
	if event.ChannelID != "" {
		id, err := snowflake.Parse(event.ChannelID)
		if err == nil {
			channelID = &id
		}
	}

	b.lavalinkClient.Link.OnVoiceStateUpdate(ctx(s), guildID, channelID, event.SessionID)
}

// handleVoiceServerUpdate forwards voice server updates to disgolink.
func (b *Bot) handleVoiceServerUpdate(s *discordgo.Session, event *discordgo.VoiceServerUpdate) {
	guildID, err := snowflake.Parse(event.GuildID)
	if err != nil {
		return
	}

	b.lavalinkClient.Link.OnVoiceServerUpdate(ctx(s), guildID, event.Token, event.Endpoint)
}

// ctx creates a context from a discordgo session (helper for the handlers).
func ctx(s *discordgo.Session) context.Context {
	return context.Background()
}

// VoiceStateUpdateHandler satisfies disgolink's requirement for voice state updates.
type VoiceStateUpdateHandler struct {
	Link disgolink.Client
}
