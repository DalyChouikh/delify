// Package player manages music playback and queue for Discord guilds.
package player

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/disgoorg/disgolink/v3/disgolink"
	"github.com/disgoorg/disgolink/v3/lavalink"
	"github.com/disgoorg/snowflake/v2"
)

// Manager handles music players across all guilds.
type Manager struct {
	link              disgolink.Client
	session           *discordgo.Session
	queues            map[snowflake.ID]*Queue
	currentTracks     map[snowflake.ID]*QueuedTrack // Track who requested the current song
	inactivityTimers  map[snowflake.ID]*time.Timer  // Inactivity timers per guild
	inactivityTimeout time.Duration
	mu                sync.RWMutex
	logger            *slog.Logger
}

// NewManager creates a new player manager.
func NewManager(link disgolink.Client, session *discordgo.Session, inactivityTimeout time.Duration, logger *slog.Logger) *Manager {
	m := &Manager{
		link:              link,
		session:           session,
		queues:            make(map[snowflake.ID]*Queue),
		currentTracks:     make(map[snowflake.ID]*QueuedTrack),
		inactivityTimers:  make(map[snowflake.ID]*time.Timer),
		inactivityTimeout: inactivityTimeout,
		logger:            logger,
	}

	// Register event handlers for queue management
	link.AddListeners(disgolink.NewListenerFunc(m.onTrackEnd))
	link.AddListeners(disgolink.NewListenerFunc(m.onTrackStart))
	link.AddListeners(disgolink.NewListenerFunc(m.onTrackException))

	return m
}

// Play loads and plays a track or adds it to the queue.
func (m *Manager) Play(ctx context.Context, guildID, channelID snowflake.ID, query, requestedByID, requestedBy string) (*PlayResult, error) {
	// Get or create player
	player := m.link.Player(guildID)
	queue := m.GetQueue(guildID)

	// Ensure bot is connected to voice channel
	if err := m.joinVoiceChannel(guildID, channelID); err != nil {
		return nil, fmt.Errorf("failed to join voice channel: %w", err)
	}

	// Determine search prefix based on query
	searchQuery := m.buildSearchQuery(query)

	// Load tracks
	var result *PlayResult
	node := m.link.BestNode()
	if node == nil {
		return nil, fmt.Errorf("no available Lavalink node")
	}

	loadResult, err := node.LoadTracks(ctx, searchQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to load tracks: %w", err)
	}

	switch data := loadResult.Data.(type) {
	case lavalink.Track:
		// Single track
		pos := queue.Add(data, requestedByID, requestedBy)
		result = &PlayResult{
			Type:          ResultTypeTrack,
			Track:         &data,
			QueuePosition: pos,
		}

	case lavalink.Playlist:
		// Playlist
		for _, track := range data.Tracks {
			queue.Add(track, requestedByID, requestedBy)
		}
		result = &PlayResult{
			Type:         ResultTypePlaylist,
			PlaylistName: data.Info.Name,
			TrackCount:   len(data.Tracks),
		}

	case lavalink.Search:
		// Search results - take the first one
		if len(data) == 0 {
			return nil, fmt.Errorf("no tracks found for query: %s", query)
		}
		pos := queue.Add(data[0], requestedByID, requestedBy)
		result = &PlayResult{
			Type:          ResultTypeTrack,
			Track:         &data[0],
			QueuePosition: pos,
		}

	case lavalink.Empty:
		return nil, fmt.Errorf("no tracks found for query: %s", query)

	case lavalink.Exception:
		return nil, fmt.Errorf("error loading track: %s", data.Message)

	default:
		return nil, fmt.Errorf("unexpected load result type")
	}

	// If nothing is playing, start playback
	if player.Track() == nil {
		if err := m.playNext(ctx, guildID); err != nil {
			return nil, err
		}
		result.StartedPlaying = true
	} else {
		result.StartedPlaying = false
	}

	return result, nil
}

// Skip skips the current track and returns info about it.
func (m *Manager) Skip(ctx context.Context, guildID snowflake.ID) (*lavalink.Track, error) {
	player := m.link.Player(guildID)
	currentTrack := player.Track()
	if currentTrack == nil {
		return nil, fmt.Errorf("nothing is currently playing")
	}

	// Play next track
	if err := m.playNext(ctx, guildID); err != nil {
		return nil, err
	}

	return currentTrack, nil
}

// Stop stops playback and clears the queue.
func (m *Manager) Stop(ctx context.Context, guildID snowflake.ID) error {
	player := m.link.Player(guildID)
	queue := m.GetQueue(guildID)

	// Clear the queue
	queue.Clear()

	// Cancel inactivity timer
	m.cancelInactivityTimer(guildID)

	// Clear current track info
	m.mu.Lock()
	delete(m.currentTracks, guildID)
	m.mu.Unlock()

	// Stop the player
	if err := player.Update(ctx, lavalink.WithNullTrack()); err != nil {
		return fmt.Errorf("failed to stop player: %w", err)
	}

	// Disconnect from voice
	if err := m.leaveVoiceChannel(guildID); err != nil {
		m.logger.Warn("failed to leave voice channel", "error", err)
	}

	return nil
}

// Pause pauses the current playback.
func (m *Manager) Pause(ctx context.Context, guildID snowflake.ID) error {
	player := m.link.Player(guildID)
	if player.Track() == nil {
		return fmt.Errorf("nothing is currently playing")
	}
	if player.Paused() {
		return fmt.Errorf("playback is already paused")
	}

	return player.Update(ctx, lavalink.WithPaused(true))
}

// Resume resumes paused playback.
func (m *Manager) Resume(ctx context.Context, guildID snowflake.ID) error {
	player := m.link.Player(guildID)
	if player.Track() == nil {
		return fmt.Errorf("nothing is currently playing")
	}
	if !player.Paused() {
		return fmt.Errorf("playback is not paused")
	}

	return player.Update(ctx, lavalink.WithPaused(false))
}

// Seek seeks to a position in the current track.
func (m *Manager) Seek(ctx context.Context, guildID snowflake.ID, positionMs int64) error {
	player := m.link.Player(guildID)
	track := player.Track()
	if track == nil {
		return fmt.Errorf("nothing is currently playing")
	}

	// Validate position
	if positionMs < 0 {
		positionMs = 0
	}
	maxPos := int64(track.Info.Length)
	if positionMs > maxPos {
		positionMs = maxPos - 1000 // Stay 1 second before end
	}

	return player.Update(ctx, lavalink.WithPosition(lavalink.Duration(positionMs)))
}

// SeekRelative seeks relative to the current position.
func (m *Manager) SeekRelative(ctx context.Context, guildID snowflake.ID, deltaMs int64) (lavalink.Duration, error) {
	player := m.link.Player(guildID)
	track := player.Track()
	if track == nil {
		return 0, fmt.Errorf("nothing is currently playing")
	}

	currentPos := int64(player.Position())
	newPos := currentPos + deltaMs

	if err := m.Seek(ctx, guildID, newPos); err != nil {
		return 0, err
	}

	// Clamp for return value
	if newPos < 0 {
		newPos = 0
	}
	if newPos > int64(track.Info.Length) {
		newPos = int64(track.Info.Length)
	}

	return lavalink.Duration(newPos), nil
}

// ClearQueue clears the queue but keeps the current track playing.
func (m *Manager) ClearQueue(guildID snowflake.ID) int {
	queue := m.GetQueue(guildID)
	return queue.Clear()
}

// GetPlayerState returns the current state of the player.
func (m *Manager) GetPlayerState(guildID snowflake.ID) *PlayerState {
	player := m.link.Player(guildID)
	queue := m.GetQueue(guildID)

	state := &PlayerState{
		QueueLength: queue.Len(),
	}

	track := player.Track()
	if track != nil {
		state.IsPlaying = true
		state.IsPaused = player.Paused()

		// Get current track requester info
		m.mu.RLock()
		currentQueued := m.currentTracks[guildID]
		m.mu.RUnlock()

		var reqID, reqName string
		if currentQueued != nil {
			reqID = currentQueued.RequestedByID
			reqName = currentQueued.RequestedBy
		}

		state.CurrentTrack = &CurrentTrackInfo{
			Track:         track,
			Position:      player.Position(),
			IsPaused:      player.Paused(),
			RequestedByID: reqID,
			RequestedBy:   reqName,
		}

		// Get next track info
		if nextTrack, ok := queue.Peek(); ok {
			info := nextTrack.ToDisplayInfo(1)
			state.NextTrack = &info
		}
	}

	return state
}

// GetCurrentTrack returns the currently playing track with requester info.
func (m *Manager) GetCurrentTrack(guildID snowflake.ID) *QueuedTrack {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentTracks[guildID]
}

// GetQueue returns the queue for a guild, creating one if it doesn't exist.
func (m *Manager) GetQueue(guildID snowflake.ID) *Queue {
	m.mu.Lock()
	defer m.mu.Unlock()

	if queue, exists := m.queues[guildID]; exists {
		return queue
	}

	queue := NewQueue()
	m.queues[guildID] = queue
	return queue
}

// playNext plays the next track in the queue.
func (m *Manager) playNext(ctx context.Context, guildID snowflake.ID) error {
	queue := m.GetQueue(guildID)
	player := m.link.Player(guildID)

	queuedTrack, ok := queue.Next()
	if !ok {
		// Queue is empty, stop the player and start inactivity timer
		m.mu.Lock()
		delete(m.currentTracks, guildID)
		m.mu.Unlock()

		if err := player.Update(ctx, lavalink.WithNullTrack()); err != nil {
			return fmt.Errorf("failed to stop player: %w", err)
		}

		// Start inactivity timer
		m.startInactivityTimer(guildID)
		return nil
	}

	// Cancel any existing inactivity timer since we're playing
	m.cancelInactivityTimer(guildID)

	// Store current track info
	m.mu.Lock()
	m.currentTracks[guildID] = &queuedTrack
	m.mu.Unlock()

	if err := player.Update(ctx, lavalink.WithTrack(queuedTrack.Track)); err != nil {
		return fmt.Errorf("failed to play track: %w", err)
	}

	return nil
}

// startInactivityTimer starts a timer to leave voice after inactivity.
func (m *Manager) startInactivityTimer(guildID snowflake.ID) {
	m.cancelInactivityTimer(guildID) // Cancel any existing timer

	m.mu.Lock()
	defer m.mu.Unlock()

	m.inactivityTimers[guildID] = time.AfterFunc(m.inactivityTimeout, func() {
		m.handleInactivityTimeout(guildID)
	})

	m.logger.Debug("started inactivity timer",
		"guild", guildID,
		"timeout", m.inactivityTimeout,
	)
}

// cancelInactivityTimer stops the inactivity timer for a guild.
func (m *Manager) cancelInactivityTimer(guildID snowflake.ID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if timer, exists := m.inactivityTimers[guildID]; exists {
		timer.Stop()
		delete(m.inactivityTimers, guildID)
	}
}

// handleInactivityTimeout handles the inactivity timeout by leaving voice.
func (m *Manager) handleInactivityTimeout(guildID snowflake.ID) {
	m.logger.Info("leaving voice channel due to inactivity", "guild", guildID)

	// Clean up
	m.mu.Lock()
	delete(m.inactivityTimers, guildID)
	delete(m.currentTracks, guildID)
	m.mu.Unlock()

	// Leave voice channel
	if err := m.leaveVoiceChannel(guildID); err != nil {
		m.logger.Warn("failed to leave voice channel on inactivity", "error", err)
	}
}

// onTrackStart handles track start events for logging.
func (m *Manager) onTrackStart(player disgolink.Player, event lavalink.TrackStartEvent) {
	m.logger.Info("track started",
		"guild", player.GuildID(),
		"track", event.Track.Info.Title,
	)
}

// onTrackEnd handles track end events to play the next track.
func (m *Manager) onTrackEnd(player disgolink.Player, event lavalink.TrackEndEvent) {
	if !event.Reason.MayStartNext() {
		return
	}

	ctx := context.Background()
	if err := m.playNext(ctx, player.GuildID()); err != nil {
		m.logger.Error("failed to play next track", "error", err, "guild", player.GuildID())
	}
}

// onTrackException handles track exception events.
func (m *Manager) onTrackException(player disgolink.Player, event lavalink.TrackExceptionEvent) {
	m.logger.Error("track exception",
		"guild", player.GuildID(),
		"track", event.Track.Info.Title,
		"error", event.Exception.Message,
	)
}

// buildSearchQuery adds appropriate search prefix based on query type.
func (m *Manager) buildSearchQuery(query string) string {
	// Check if it's already a URL
	if isURL(query) {
		return query
	}

	// Default to YouTube search
	return "ytsearch:" + query
}

// isURL checks if the string is a URL.
func isURL(s string) bool {
	return len(s) > 8 && (s[:7] == "http://" || s[:8] == "https://")
}

// joinVoiceChannel connects the bot to a voice channel.
func (m *Manager) joinVoiceChannel(guildID, channelID snowflake.ID) error {
	err := m.session.ChannelVoiceJoinManual(guildID.String(), channelID.String(), false, true)
	if err != nil {
		return err
	}
	return nil
}

// leaveVoiceChannel disconnects the bot from voice.
func (m *Manager) leaveVoiceChannel(guildID snowflake.ID) error {
	return m.session.ChannelVoiceJoinManual(guildID.String(), "", false, false)
}
