// Package bot provides Discord bot initialization and lifecycle management.
package bot

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/DalyChouikh/delify/internal/commands"
	"github.com/DalyChouikh/delify/internal/config"
	"github.com/DalyChouikh/delify/internal/lavalink"
	"github.com/DalyChouikh/delify/internal/player"
	"github.com/bwmarrin/discordgo"
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
	voiceMu        sync.Mutex
	stopping       bool // protected by voiceMu
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
	b.playerManager = player.NewManager(b.lavalinkClient.Link, b.session, b.config.Bot.InactivityTimeout, b.logger)

	// Publish voice handlers only after their dependencies are initialized.
	b.session.AddHandler(b.handleVoiceStateUpdate)
	b.session.AddHandler(b.handleVoiceServerUpdate)

	// Create command handler
	b.commandHandler = commands.NewHandler(b.session, b.playerManager, b.config, b.logger)

	// Register slash commands
	if err := b.commandHandler.RegisterContext(ctx, b.config.Discord.GuildIDs); err != nil {
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
	b.voiceMu.Lock()
	b.stopping = true
	b.voiceMu.Unlock()
	if b.playerManager != nil {
		b.playerManager.Close()
	}

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
	b.voiceMu.Lock()
	defer b.voiceMu.Unlock()
	if b.stopping || b.lavalinkClient == nil || event == nil || event.VoiceState == nil || s.State == nil || s.State.User == nil || event.UserID != s.State.User.ID {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	b.lavalinkClient.Link.OnVoiceStateUpdate(ctx, guildID, channelID, event.SessionID)

	// Only release a waiting /play after Lavalink has received the update.
	if b.playerManager != nil {
		b.playerManager.OnVoiceStateUpdate(guildID, channelID)
	}

}

// handleVoiceServerUpdate forwards voice server updates to disgolink.
func (b *Bot) handleVoiceServerUpdate(s *discordgo.Session, event *discordgo.VoiceServerUpdate) {
	b.voiceMu.Lock()
	defer b.voiceMu.Unlock()
	if b.stopping || b.lavalinkClient == nil || event == nil || event.Endpoint == "" {
		return
	}
	guildID, err := snowflake.Parse(event.GuildID)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	b.lavalinkClient.Link.OnVoiceServerUpdate(ctx, guildID, event.Token, event.Endpoint)

	// Only release a waiting /play after Lavalink has received the update.
	if b.playerManager != nil {
		b.playerManager.OnVoiceServerUpdate(guildID)
	}

}
