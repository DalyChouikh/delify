// Package commands handles Discord slash commands registration and execution.
package commands

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/bwmarrin/discordgo"
	"github.com/DalyChouikh/delify/internal/player"
	"github.com/disgoorg/snowflake/v2"
)

// Handler manages slash commands.
type Handler struct {
	session *discordgo.Session
	player  *player.Manager
	logger  *slog.Logger
}

// NewHandler creates a new command handler.
func NewHandler(session *discordgo.Session, playerManager *player.Manager, logger *slog.Logger) *Handler {
	return &Handler{
		session: session,
		player:  playerManager,
		logger:  logger,
	}
}

// Commands returns all slash commands to register.
func (h *Handler) Commands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:        "play",
			Description: "Play a song or add it to the queue",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "query",
					Description: "Song name, YouTube URL, or Spotify link",
					Required:    true,
				},
			},
		},
		{
			Name:        "skip",
			Description: "Skip the current song",
		},
		{
			Name:        "stop",
			Description: "Stop playback and clear the queue",
		},
	}
}

// Register registers all slash commands with Discord.
func (h *Handler) Register(guildID string) error {
	for _, cmd := range h.Commands() {
		_, err := h.session.ApplicationCommandCreate(h.session.State.User.ID, guildID, cmd)
		if err != nil {
			return fmt.Errorf("failed to create command %s: %w", cmd.Name, err)
		}
		h.logger.Info("registered command", "name", cmd.Name)
	}
	return nil
}

// HandleInteraction routes interaction events to appropriate handlers.
func (h *Handler) HandleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	data := i.ApplicationCommandData()

	switch data.Name {
	case "play":
		h.handlePlay(s, i)
	case "skip":
		h.handleSkip(s, i)
	case "stop":
		h.handleStop(s, i)
	}
}

// handlePlay handles the /play command.
func (h *Handler) handlePlay(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Defer reply to give us time to process
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	}); err != nil {
		h.logger.Error("failed to defer response", "error", err)
		return
	}

	// Get the voice state of the user
	voiceState, err := h.getUserVoiceState(i)
	if err != nil {
		h.editResponse(s, i, "❌ You need to be in a voice channel to use this command!")
		return
	}

	// Get the query option
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		h.editResponse(s, i, "❌ Please provide a song name or URL!")
		return
	}
	query := options[0].StringValue()

	// Parse IDs
	guildID, err := snowflake.Parse(i.GuildID)
	if err != nil {
		h.editResponse(s, i, "❌ Invalid guild ID")
		return
	}

	channelID, err := snowflake.Parse(voiceState.ChannelID)
	if err != nil {
		h.editResponse(s, i, "❌ Invalid channel ID")
		return
	}

	// Play the track
	ctx := context.Background()
	result, err := h.player.Play(ctx, guildID, channelID, query)
	if err != nil {
		h.logger.Error("failed to play track", "error", err, "query", query)
		h.editResponse(s, i, fmt.Sprintf("❌ Failed to play: %s", err.Error()))
		return
	}

	// Build response message
	var message string
	switch result.Type {
	case player.ResultTypeTrack:
		if result.StartedPlaying {
			message = fmt.Sprintf("🎵 Now playing: **%s**", result.Track.Info.Title)
		} else {
			message = fmt.Sprintf("📝 Added to queue: **%s**", result.Track.Info.Title)
		}
	case player.ResultTypePlaylist:
		message = fmt.Sprintf("📝 Added **%d** tracks from playlist: **%s**", result.TrackCount, result.PlaylistName)
	}

	h.editResponse(s, i, message)
}

// handleSkip handles the /skip command.
func (h *Handler) handleSkip(s *discordgo.Session, i *discordgo.InteractionCreate) {
	guildID, err := snowflake.Parse(i.GuildID)
	if err != nil {
		h.respondWithError(s, i, "Invalid guild ID")
		return
	}

	ctx := context.Background()
	if err := h.player.Skip(ctx, guildID); err != nil {
		h.respondWithError(s, i, err.Error())
		return
	}

	h.respond(s, i, "⏭️ Skipped to the next track!")
}

// handleStop handles the /stop command.
func (h *Handler) handleStop(s *discordgo.Session, i *discordgo.InteractionCreate) {
	guildID, err := snowflake.Parse(i.GuildID)
	if err != nil {
		h.respondWithError(s, i, "Invalid guild ID")
		return
	}

	ctx := context.Background()
	if err := h.player.Stop(ctx, guildID); err != nil {
		h.respondWithError(s, i, err.Error())
		return
	}

	h.respond(s, i, "⏹️ Stopped playback and cleared the queue!")
}

// getUserVoiceState gets the voice state of the interaction user.
func (h *Handler) getUserVoiceState(i *discordgo.InteractionCreate) (*discordgo.VoiceState, error) {
	guild, err := h.session.State.Guild(i.GuildID)
	if err != nil {
		return nil, err
	}

	for _, vs := range guild.VoiceStates {
		if vs.UserID == i.Member.User.ID {
			return vs, nil
		}
	}

	return nil, fmt.Errorf("user not in voice channel")
}

// respond sends an immediate response to an interaction.
func (h *Handler) respond(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
		},
	}); err != nil {
		h.logger.Error("failed to respond to interaction", "error", err)
	}
}

// respondWithError sends an error response.
func (h *Handler) respondWithError(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	h.respond(s, i, fmt.Sprintf("❌ %s", message))
}

// editResponse edits a deferred response.
func (h *Handler) editResponse(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	if _, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &content,
	}); err != nil {
		h.logger.Error("failed to edit interaction response", "error", err)
	}
}
