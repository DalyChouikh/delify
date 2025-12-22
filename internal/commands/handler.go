// Package commands handles Discord slash commands registration and execution.
package commands

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/DalyChouikh/delify/internal/components"
	"github.com/DalyChouikh/delify/internal/config"
	"github.com/DalyChouikh/delify/internal/embed"
	"github.com/DalyChouikh/delify/internal/errors"
	"github.com/DalyChouikh/delify/internal/lyrics"
	"github.com/DalyChouikh/delify/internal/player"
	"github.com/DalyChouikh/delify/internal/utils"
	"github.com/disgoorg/snowflake/v2"
)

const (
	queuePageSize = 10
)

// Handler manages slash commands.
type Handler struct {
	session      *discordgo.Session
	player       *player.Manager
	lyrics       *lyrics.Client
	logger       *slog.Logger
	templates    *embed.Templates
	config       *config.Config
	lyricsStates *lyricsStateManager // Track lyrics pagination state
}

// lyricsStateManager manages active lyrics sessions for pagination.
type lyricsStateManager struct {
	mu     sync.RWMutex
	states map[string]*lyricsState // keyed by messageID
}

type lyricsState struct {
	result      *lyrics.LyricsResult
	currentPage int
}

func newLyricsStateManager() *lyricsStateManager {
	return &lyricsStateManager{
		states: make(map[string]*lyricsState),
	}
}

func (m *lyricsStateManager) set(messageID string, result *lyrics.LyricsResult, page int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states[messageID] = &lyricsState{result: result, currentPage: page}
}

func (m *lyricsStateManager) get(messageID string) (*lyricsState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state, exists := m.states[messageID]
	return state, exists
}

func (m *lyricsStateManager) updatePage(messageID string, page int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if state, exists := m.states[messageID]; exists {
		state.currentPage = page
	}
}

func (m *lyricsStateManager) delete(messageID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.states, messageID)
}

// NewHandler creates a new command handler.
func NewHandler(session *discordgo.Session, playerManager *player.Manager, cfg *config.Config, logger *slog.Logger) *Handler {
	// Get bot avatar URL
	botAvatarURL := ""
	if session.State != nil && session.State.User != nil {
		botAvatarURL = session.State.User.AvatarURL("64")
	}

	// Get developer avatar URL
	devAvatarURL := ""
	if cfg.Bot.DeveloperUserID != "" {
		user, err := session.User(cfg.Bot.DeveloperUserID)
		if err == nil && user != nil {
			devAvatarURL = user.AvatarURL("64")
		}
	}

	templates := embed.NewTemplates(embed.TemplateConfig{
		BotAvatarURL:       botAvatarURL,
		DeveloperAvatarURL: devAvatarURL,
	})

	// Create lyrics client
	lyricsClient := lyrics.NewClient(lyrics.Config{
		APIKey:  cfg.Lyrics.RapidAPIKey,
		APIHost: cfg.Lyrics.RapidAPIHost,
		Logger:  logger,
	})

	return &Handler{
		session:      session,
		player:       playerManager,
		lyrics:       lyricsClient,
		logger:       logger,
		templates:    templates,
		config:       cfg,
		lyricsStates: newLyricsStateManager(),
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
		{
			Name:        "pause",
			Description: "Pause the current playback",
		},
		{
			Name:        "resume",
			Description: "Resume paused playback",
		},
		{
			Name:        "nowplaying",
			Description: "Show the currently playing track",
		},
		{
			Name:        "queue",
			Description: "Show the current queue",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "page",
					Description: "Page number to display",
					Required:    false,
				},
			},
		},
		{
			Name:        "clear",
			Description: "Clear the queue (keeps current song)",
		},
		{
			Name:        "seek",
			Description: "Seek to a position in the current track",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "time",
					Description: "Position to seek to (e.g., 1:30, 90, 1:30:00)",
					Required:    true,
				},
			},
		},
		{
			Name:        "lyrics",
			Description: "Show lyrics for the current or specified song",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "query",
					Description: "Song name and artist (optional, uses current track if empty)",
					Required:    false,
				},
			},
		},
	}
}

// Register registers all slash commands with Discord.
// If guildIDs is empty, commands are registered globally (takes up to 1 hour to propagate).
// If guildIDs is provided, commands are registered to each guild instantly.
func (h *Handler) Register(guildIDs []string) error {
	commands := h.Commands()

	// If no guild IDs specified, register globally
	if len(guildIDs) == 0 {
		h.logger.Info("registering commands globally (may take up to 1 hour to propagate)")
		for _, cmd := range commands {
			_, err := h.session.ApplicationCommandCreate(h.session.State.User.ID, "", cmd)
			if err != nil {
				return fmt.Errorf("failed to create global command %s: %w", cmd.Name, err)
			}
			h.logger.Info("registered global command", "name", cmd.Name)
		}
		return nil
	}

	// Register to each specified guild
	for _, guildID := range guildIDs {
		h.logger.Info("registering commands to guild", "guild_id", guildID)
		for _, cmd := range commands {
			_, err := h.session.ApplicationCommandCreate(h.session.State.User.ID, guildID, cmd)
			if err != nil {
				return fmt.Errorf("failed to create command %s for guild %s: %w", cmd.Name, guildID, err)
			}
			h.logger.Info("registered command", "name", cmd.Name, "guild_id", guildID)
		}
	}
	return nil
}

// HandleInteraction routes interaction events to appropriate handlers.
func (h *Handler) HandleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		h.handleCommand(s, i)
	case discordgo.InteractionMessageComponent:
		h.handleButton(s, i)
	}
}

// handleCommand routes slash commands.
func (h *Handler) handleCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()

	switch data.Name {
	case "play":
		h.handlePlay(s, i)
	case "skip":
		h.handleSkip(s, i)
	case "stop":
		h.handleStop(s, i)
	case "pause":
		h.handlePause(s, i)
	case "resume":
		h.handleResume(s, i)
	case "nowplaying":
		h.handleNowPlaying(s, i)
	case "queue":
		h.handleQueue(s, i)
	case "clear":
		h.handleClear(s, i)
	case "seek":
		h.handleSeek(s, i)
	case "lyrics":
		h.handleLyrics(s, i)
	}
}

// handleButton routes button interactions.
func (h *Handler) handleButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID

	// Handle lyrics pagination separately (doesn't require voice channel)
	if strings.HasPrefix(customID, "delify:lyrics:") {
		h.handleLyricsPagination(s, i)
		return
	}

	// For playback controls, verify user is in the same voice channel
	if err := h.verifyVoiceChannel(i); err != nil {
		h.respondEphemeralError(s, i, err)
		return
	}

	guildID, _ := snowflake.Parse(i.GuildID)
	ctx := context.Background()

	switch customID {
	case components.ButtonPause:
		if err := h.player.Pause(ctx, guildID); err != nil {
			h.respondEphemeralError(s, i, errors.New(errors.ErrAlreadyPaused))
			return
		}
		h.updateNowPlayingMessage(s, i, true)

	case components.ButtonResume:
		if err := h.player.Resume(ctx, guildID); err != nil {
			h.respondEphemeralError(s, i, errors.New(errors.ErrNotPaused))
			return
		}
		h.updateNowPlayingMessage(s, i, false)

	case components.ButtonSkip:
		if _, err := h.player.Skip(ctx, guildID); err != nil {
			h.respondEphemeralError(s, i, errors.New(errors.ErrNothingPlaying))
			return
		}
		h.updateNowPlayingMessage(s, i, false)

	case components.ButtonStop:
		if err := h.player.Stop(ctx, guildID); err != nil {
			h.respondEphemeralError(s, i, errors.New(errors.ErrInternal))
			return
		}
		h.respondWithEmbed(s, i, h.templates.Stopped(), nil)

	case components.ButtonClear:
		count := h.player.ClearQueue(guildID)
		h.respondWithEmbed(s, i, h.templates.QueueCleared(count), nil)

	case components.ButtonLyrics:
		h.handleLyricsButton(s, i, guildID)

	case components.ButtonSeekBack30:
		h.handleSeekButton(s, i, guildID, -30000)
	case components.ButtonSeekBack10:
		h.handleSeekButton(s, i, guildID, -10000)
	case components.ButtonSeekBack5:
		h.handleSeekButton(s, i, guildID, -5000)
	case components.ButtonSeekForward5:
		h.handleSeekButton(s, i, guildID, 5000)
	case components.ButtonSeekForward10:
		h.handleSeekButton(s, i, guildID, 10000)
	case components.ButtonSeekForward30:
		h.handleSeekButton(s, i, guildID, 30000)

	default:
		h.respondEphemeralError(s, i, errors.New(errors.ErrInternal))
	}
}

// handleSeekButton handles seek button presses.
func (h *Handler) handleSeekButton(s *discordgo.Session, i *discordgo.InteractionCreate, guildID snowflake.ID, deltaMs int64) {
	ctx := context.Background()
	newPos, err := h.player.SeekRelative(ctx, guildID, deltaMs)
	if err != nil {
		h.respondEphemeralError(s, i, errors.New(errors.ErrNothingPlaying))
		return
	}

	state := h.player.GetPlayerState(guildID)
	if state.CurrentTrack != nil {
		trackInfo := h.buildTrackInfo(state)
		emb := h.templates.Seeked(trackInfo, newPos)
		h.respondEphemeral(s, i, emb)
	} else {
		h.respondEphemeralError(s, i, errors.New(errors.ErrNothingPlaying))
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
		h.editWithError(s, i, errors.New(errors.ErrNotInVoice))
		return
	}

	// Get the query option
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		h.editWithError(s, i, errors.New(errors.ErrInvalidInput).WithDetails("Please provide a song name or URL"))
		return
	}
	query := options[0].StringValue()

	// Parse IDs
	guildID, err := snowflake.Parse(i.GuildID)
	if err != nil {
		h.editWithError(s, i, errors.New(errors.ErrInternal))
		return
	}

	channelID, err := snowflake.Parse(voiceState.ChannelID)
	if err != nil {
		h.editWithError(s, i, errors.New(errors.ErrInternal))
		return
	}

	// Get requester info
	requestedByID := i.Member.User.ID
	requestedBy := i.Member.User.Username

	// Play the track
	ctx := context.Background()
	result, err := h.player.Play(ctx, guildID, channelID, query, requestedByID, requestedBy)
	if err != nil {
		h.logger.Error("failed to play track", "error", err, "query", query)
		h.editWithError(s, i, errors.New(errors.ErrTrackFailed).WithDetails(err.Error()))
		return
	}

	// Build response
	var responseEmbed *discordgo.MessageEmbed
	var comps []discordgo.MessageComponent

	switch result.Type {
	case player.ResultTypeTrack:
		trackInfo := embed.TrackInfo{
			Title:         result.Track.Info.Title,
			Author:        result.Track.Info.Author,
			Duration:      result.Track.Info.Length,
			RequestedByID: requestedByID,
			RequestedBy:   requestedBy,
			QueueLength:   h.player.GetQueue(guildID).Len(),
			Position:      result.QueuePosition,
		}
		if result.Track.Info.URI != nil {
			trackInfo.URL = *result.Track.Info.URI
		}
		if result.Track.Info.ArtworkURL != nil {
			trackInfo.ArtworkURL = *result.Track.Info.ArtworkURL
		}

		// Get next track info
		if next, ok := h.player.GetQueue(guildID).Peek(); ok {
			trackInfo.NextTrackTitle = next.Track.Info.Title
			trackInfo.NextTrackAuthor = next.Track.Info.Author
		}

		if result.StartedPlaying {
			responseEmbed = h.templates.NowPlaying(trackInfo)
			comps = components.NowPlayingComponents(false)
		} else {
			responseEmbed = h.templates.AddedToQueue(trackInfo)
		}

	case player.ResultTypePlaylist:
		responseEmbed = h.templates.PlaylistAdded(result.PlaylistName, result.TrackCount, requestedByID)
	}

	h.editWithEmbed(s, i, responseEmbed, comps)
}

// handleSkip handles the /skip command.
func (h *Handler) handleSkip(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if err := h.verifyVoiceChannel(i); err != nil {
		h.respondEphemeralError(s, i, err)
		return
	}

	guildID, _ := snowflake.Parse(i.GuildID)
	ctx := context.Background()

	skippedTrack, err := h.player.Skip(ctx, guildID)
	if err != nil {
		h.respondEphemeralError(s, i, errors.New(errors.ErrNothingPlaying))
		return
	}

	// Check if there's a new track playing
	state := h.player.GetPlayerState(guildID)
	var nextInfo *embed.TrackInfo
	if state.CurrentTrack != nil {
		info := h.buildTrackInfo(state)
		nextInfo = &info
	}

	responseEmbed := h.templates.Skipped(skippedTrack.Info.Title, nextInfo)
	h.respondWithEmbed(s, i, responseEmbed, nil)
}

// handleStop handles the /stop command.
func (h *Handler) handleStop(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if err := h.verifyVoiceChannel(i); err != nil {
		h.respondEphemeralError(s, i, err)
		return
	}

	guildID, _ := snowflake.Parse(i.GuildID)
	ctx := context.Background()

	if err := h.player.Stop(ctx, guildID); err != nil {
		h.respondEphemeralError(s, i, errors.New(errors.ErrInternal))
		return
	}

	h.respondWithEmbed(s, i, h.templates.Stopped(), nil)
}

// handlePause handles the /pause command.
func (h *Handler) handlePause(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if err := h.verifyVoiceChannel(i); err != nil {
		h.respondEphemeralError(s, i, err)
		return
	}

	guildID, _ := snowflake.Parse(i.GuildID)
	ctx := context.Background()

	if err := h.player.Pause(ctx, guildID); err != nil {
		h.respondEphemeralError(s, i, errors.New(errors.ErrAlreadyPaused))
		return
	}

	state := h.player.GetPlayerState(guildID)
	if state.CurrentTrack != nil {
		trackInfo := h.buildTrackInfo(state)
		emb := h.templates.Paused(trackInfo, state.CurrentTrack.Position)
		h.respondWithEmbed(s, i, emb, components.NowPlayingComponents(true))
	}
}

// handleResume handles the /resume command.
func (h *Handler) handleResume(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if err := h.verifyVoiceChannel(i); err != nil {
		h.respondEphemeralError(s, i, err)
		return
	}

	guildID, _ := snowflake.Parse(i.GuildID)
	ctx := context.Background()

	if err := h.player.Resume(ctx, guildID); err != nil {
		h.respondEphemeralError(s, i, errors.New(errors.ErrNotPaused))
		return
	}

	state := h.player.GetPlayerState(guildID)
	if state.CurrentTrack != nil {
		trackInfo := h.buildTrackInfo(state)
		emb := h.templates.Resumed(trackInfo)
		h.respondWithEmbed(s, i, emb, nil)
	}
}

// handleNowPlaying handles the /nowplaying command.
func (h *Handler) handleNowPlaying(s *discordgo.Session, i *discordgo.InteractionCreate) {
	guildID, _ := snowflake.Parse(i.GuildID)
	state := h.player.GetPlayerState(guildID)

	if !state.IsPlaying || state.CurrentTrack == nil {
		h.respondEphemeralError(s, i, errors.New(errors.ErrNothingPlaying))
		return
	}

	trackInfo := h.buildTrackInfo(state)
	emb := h.templates.NowPlaying(trackInfo)
	h.respondWithEmbed(s, i, emb, components.NowPlayingComponents(state.IsPaused))
}

// handleQueue handles the /queue command.
func (h *Handler) handleQueue(s *discordgo.Session, i *discordgo.InteractionCreate) {
	guildID, _ := snowflake.Parse(i.GuildID)
	queue := h.player.GetQueue(guildID)

	// Get page from options
	page := 1
	options := i.ApplicationCommandData().Options
	for _, opt := range options {
		if opt.Name == "page" {
			page = int(opt.IntValue())
		}
	}
	if page < 1 {
		page = 1
	}

	// Get queue page
	tracks, currentPage, totalPages := queue.GetPage(page-1, queuePageSize)

	// Build queue items
	items := make([]embed.QueueItem, len(tracks))
	for idx, t := range tracks {
		items[idx] = embed.QueueItem{
			Position:    (currentPage-1)*queuePageSize + idx + 1,
			Title:       t.Track.Info.Title,
			Author:      t.Track.Info.Author,
			Duration:    t.Track.Info.Length,
			RequestedBy: t.RequestedBy,
		}
	}

	// Get current track
	var currentTrack *embed.TrackInfo
	state := h.player.GetPlayerState(guildID)
	if state.CurrentTrack != nil {
		info := h.buildTrackInfo(state)
		currentTrack = &info
	}

	totalDuration := queue.TotalDuration()
	queueEmbed := h.templates.QueueDisplay(items, currentTrack, currentPage, totalPages, queue.Len(), totalDuration)

	h.respondWithEmbed(s, i, queueEmbed, components.QueueComponents(currentPage, totalPages))
}

// handleClear handles the /clear command.
func (h *Handler) handleClear(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if err := h.verifyVoiceChannel(i); err != nil {
		h.respondEphemeralError(s, i, err)
		return
	}

	guildID, _ := snowflake.Parse(i.GuildID)
	count := h.player.ClearQueue(guildID)

	if count == 0 {
		h.respondEphemeralError(s, i, errors.New(errors.ErrQueueEmpty))
		return
	}

	h.respondWithEmbed(s, i, h.templates.QueueCleared(count), nil)
}

// handleSeek handles the /seek command.
func (h *Handler) handleSeek(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if err := h.verifyVoiceChannel(i); err != nil {
		h.respondEphemeralError(s, i, err)
		return
	}

	guildID, _ := snowflake.Parse(i.GuildID)

	// Get time option
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		h.respondEphemeralError(s, i, errors.New(errors.ErrInvalidInput))
		return
	}

	timeStr := options[0].StringValue()
	positionMs, err := utils.ParseDuration(timeStr)
	if err != nil {
		h.respondEphemeralError(s, i, errors.New(errors.ErrSeekInvalid).WithDetails("Use format like 1:30, 90, or 1:30:00"))
		return
	}

	ctx := context.Background()
	if err := h.player.Seek(ctx, guildID, positionMs); err != nil {
		h.respondEphemeralError(s, i, errors.New(errors.ErrNothingPlaying))
		return
	}

	state := h.player.GetPlayerState(guildID)
	if state.CurrentTrack != nil {
		trackInfo := h.buildTrackInfo(state)
		emb := h.templates.Seeked(trackInfo, state.CurrentTrack.Position)
		h.respondWithEmbed(s, i, emb, nil)
	}
}

// handleLyrics handles the /lyrics command.
func (h *Handler) handleLyrics(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !h.lyrics.IsEnabled() {
		h.respondEphemeralError(s, i, errors.New(errors.ErrLyricsDisabled))
		return
	}

	// Defer reply to give us time to fetch lyrics
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	}); err != nil {
		h.logger.Error("failed to defer response", "error", err)
		return
	}

	// Get song title and artist
	var title, artist string
	options := i.ApplicationCommandData().Options
	if len(options) > 0 && options[0].StringValue() != "" {
		// User provided a query
		query := options[0].StringValue()
		parts := strings.SplitN(query, " - ", 2)
		if len(parts) == 2 {
			artist = strings.TrimSpace(parts[0])
			title = strings.TrimSpace(parts[1])
		} else {
			title = query
			artist = ""
		}
	} else {
		// Use current track
		guildID, _ := snowflake.Parse(i.GuildID)
		state := h.player.GetPlayerState(guildID)
		if state.CurrentTrack == nil || state.CurrentTrack.Track == nil {
			h.editWithError(s, i, errors.New(errors.ErrNothingPlaying))
			return
		}
		title = state.CurrentTrack.Track.Info.Title
		artist = state.CurrentTrack.Track.Info.Author
	}

	// Fetch lyrics
	ctx := context.Background()
	result, err := h.lyrics.FetchLyrics(ctx, title, artist)
	if err != nil {
		h.logger.Warn("failed to fetch lyrics", "error", err, "title", title, "artist", artist)
		emb := h.templates.LyricsNotFound(title, artist)
		h.editWithEmbed(s, i, emb, nil)
		return
	}

	// Send first page
	emb := h.templates.LyricsDisplay(
		result.Title,
		result.Artist,
		result.Pages[0],
		result.ArtworkURL,
		result.GeniusURL,
		1,
		result.TotalPages,
	)
	comps := components.LyricsComponents(1, result.TotalPages)
	h.editWithEmbed(s, i, emb, comps)

	// Store lyrics state for pagination (only if multiple pages)
	if result.TotalPages > 1 {
		// Get message ID from the response
		msg, err := s.InteractionResponse(i.Interaction)
		if err == nil && msg != nil {
			h.lyricsStates.set(msg.ID, result, 1)
		}
	}
}

// handleLyricsButton handles the lyrics button from now playing.
func (h *Handler) handleLyricsButton(s *discordgo.Session, i *discordgo.InteractionCreate, guildID snowflake.ID) {
	if !h.lyrics.IsEnabled() {
		h.respondEphemeralError(s, i, errors.New(errors.ErrLyricsDisabled))
		return
	}

	// Get current track
	state := h.player.GetPlayerState(guildID)
	if state.CurrentTrack == nil || state.CurrentTrack.Track == nil {
		h.respondEphemeralError(s, i, errors.New(errors.ErrNothingPlaying))
		return
	}

	title := state.CurrentTrack.Track.Info.Title
	artist := state.CurrentTrack.Track.Info.Author

	// Defer reply
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	}); err != nil {
		h.logger.Error("failed to defer response", "error", err)
		return
	}

	// Fetch lyrics
	ctx := context.Background()
	result, err := h.lyrics.FetchLyrics(ctx, title, artist)
	if err != nil {
		h.logger.Warn("failed to fetch lyrics", "error", err, "title", title, "artist", artist)
		emb := h.templates.LyricsNotFound(title, artist)
		h.editWithEmbed(s, i, emb, nil)
		return
	}

	// Send first page
	emb := h.templates.LyricsDisplay(
		result.Title,
		result.Artist,
		result.Pages[0],
		result.ArtworkURL,
		result.GeniusURL,
		1,
		result.TotalPages,
	)
	comps := components.LyricsComponents(1, result.TotalPages)
	h.editWithEmbed(s, i, emb, comps)

	// Store lyrics state for pagination
	if result.TotalPages > 1 {
		msg, err := s.InteractionResponse(i.Interaction)
		if err == nil && msg != nil {
			h.lyricsStates.set(msg.ID, result, 1)
		}
	}
}

// handleLyricsPagination handles lyrics pagination button presses.
func (h *Handler) handleLyricsPagination(s *discordgo.Session, i *discordgo.InteractionCreate) {
	messageID := i.Message.ID
	customID := i.MessageComponentData().CustomID

	// Get stored lyrics state
	state, exists := h.lyricsStates.get(messageID)
	if !exists {
		h.respondEphemeralError(s, i, errors.New(errors.ErrInternal).WithDetails("Lyrics session expired, please use /lyrics again"))
		return
	}

	// Determine new page
	newPage := state.currentPage
	switch customID {
	case components.ButtonLyricsPrev:
		if newPage > 1 {
			newPage--
		}
	case components.ButtonLyricsNext:
		if newPage < state.result.TotalPages {
			newPage++
		}
	}

	// Update state
	h.lyricsStates.updatePage(messageID, newPage)

	// Update message
	emb := h.templates.LyricsDisplay(
		state.result.Title,
		state.result.Artist,
		state.result.Pages[newPage-1],
		state.result.ArtworkURL,
		state.result.GeniusURL,
		newPage,
		state.result.TotalPages,
	)
	comps := components.LyricsComponents(newPage, state.result.TotalPages)

	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{emb},
			Components: comps,
		},
	}); err != nil {
		h.logger.Error("failed to update lyrics message", "error", err)
	}
}

// Helper methods

// buildTrackInfo builds embed.TrackInfo from PlayerState.
func (h *Handler) buildTrackInfo(state *player.PlayerState) embed.TrackInfo {
	info := embed.TrackInfo{
		QueueLength: state.QueueLength,
	}

	if state.CurrentTrack != nil && state.CurrentTrack.Track != nil {
		track := state.CurrentTrack.Track
		info.Title = track.Info.Title
		info.Author = track.Info.Author
		info.Duration = track.Info.Length
		info.RequestedByID = state.CurrentTrack.RequestedByID
		info.RequestedBy = state.CurrentTrack.RequestedBy

		if track.Info.URI != nil {
			info.URL = *track.Info.URI
		}
		if track.Info.ArtworkURL != nil {
			info.ArtworkURL = *track.Info.ArtworkURL
		}
	}

	if state.NextTrack != nil {
		info.NextTrackTitle = state.NextTrack.Title
		info.NextTrackAuthor = state.NextTrack.Author
	}

	return info
}

// verifyVoiceChannel checks if the user is in the same voice channel as the bot.
func (h *Handler) verifyVoiceChannel(i *discordgo.InteractionCreate) *errors.UserError {
	voiceState, err := h.getUserVoiceState(i)
	if err != nil {
		return errors.New(errors.ErrNotInVoice)
	}

	// Check if bot is in a voice channel in this guild
	guild, err := h.session.State.Guild(i.GuildID)
	if err != nil {
		return nil // Can't verify, allow the action
	}

	for _, vs := range guild.VoiceStates {
		if vs.UserID == h.session.State.User.ID {
			if vs.ChannelID != voiceState.ChannelID {
				return errors.New(errors.ErrDifferentChannel)
			}
			break
		}
	}

	return nil
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

// Response helpers

func (h *Handler) respondWithEmbed(s *discordgo.Session, i *discordgo.InteractionCreate, emb *discordgo.MessageEmbed, comps []discordgo.MessageComponent) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{emb},
			Components: comps,
		},
	}); err != nil {
		h.logger.Error("failed to respond with embed", "error", err)
	}
}

func (h *Handler) respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, emb *discordgo.MessageEmbed) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{emb},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	}); err != nil {
		h.logger.Error("failed to respond ephemeral", "error", err)
	}
}

func (h *Handler) respondEphemeralError(s *discordgo.Session, i *discordgo.InteractionCreate, userErr *errors.UserError) {
	if err := s.InteractionRespond(i.Interaction, userErr.EphemeralResponse()); err != nil {
		h.logger.Error("failed to respond with error", "error", err)
	}
}

func (h *Handler) editWithEmbed(s *discordgo.Session, i *discordgo.InteractionCreate, emb *discordgo.MessageEmbed, comps []discordgo.MessageComponent) {
	embeds := []*discordgo.MessageEmbed{emb}
	edit := &discordgo.WebhookEdit{
		Embeds:     &embeds,
		Components: &comps,
	}
	if _, err := s.InteractionResponseEdit(i.Interaction, edit); err != nil {
		h.logger.Error("failed to edit with embed", "error", err)
	}
}

func (h *Handler) editWithError(s *discordgo.Session, i *discordgo.InteractionCreate, userErr *errors.UserError) {
	if _, err := s.InteractionResponseEdit(i.Interaction, userErr.EphemeralWebhookEdit()); err != nil {
		h.logger.Error("failed to edit with error", "error", err)
	}
}

func (h *Handler) updateNowPlayingMessage(s *discordgo.Session, i *discordgo.InteractionCreate, isPaused bool) {
	guildID, _ := snowflake.Parse(i.GuildID)
	state := h.player.GetPlayerState(guildID)

	if state.CurrentTrack != nil {
		trackInfo := h.buildTrackInfo(state)
		emb := h.templates.NowPlaying(trackInfo)
		embeds := []*discordgo.MessageEmbed{emb}
		comps := components.NowPlayingComponents(isPaused)

		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Embeds:     embeds,
				Components: comps,
			},
		}); err != nil {
			h.logger.Error("failed to update message", "error", err)
		}
	}
}
