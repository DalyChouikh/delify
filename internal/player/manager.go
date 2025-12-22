// Package player manages music playback and queue for Discord guilds.
package player

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/disgoorg/disgolink/v3/disgolink"
	"github.com/disgoorg/disgolink/v3/lavalink"
	"github.com/disgoorg/snowflake/v2"
)

// Manager handles music players across all guilds.
type Manager struct {
	link    disgolink.Client
	session *discordgo.Session
	queues  map[snowflake.ID]*Queue
	mu      sync.RWMutex
	logger  *slog.Logger
}

// NewManager creates a new player manager.
func NewManager(link disgolink.Client, session *discordgo.Session, logger *slog.Logger) *Manager {
	m := &Manager{
		link:    link,
		session: session,
		queues:  make(map[snowflake.ID]*Queue),
		logger:  logger,
	}

	// Register event handlers for queue management
	link.AddListeners(disgolink.NewListenerFunc(m.onTrackEnd))

	return m
}

// Play loads and plays a track or adds it to the queue.
func (m *Manager) Play(ctx context.Context, guildID, channelID snowflake.ID, query string) (*PlayResult, error) {
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
		queue.Add(data)
		result = &PlayResult{
			Type:  ResultTypeTrack,
			Track: &data,
		}

	case lavalink.Playlist:
		// Playlist
		for _, track := range data.Tracks {
			queue.Add(track)
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
		queue.Add(data[0])
		result = &PlayResult{
			Type:  ResultTypeTrack,
			Track: &data[0],
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

// Skip skips the current track.
func (m *Manager) Skip(ctx context.Context, guildID snowflake.ID) error {
	player := m.link.Player(guildID)
	if player.Track() == nil {
		return fmt.Errorf("nothing is currently playing")
	}

	return m.playNext(ctx, guildID)
}

// Stop stops playback and clears the queue.
func (m *Manager) Stop(ctx context.Context, guildID snowflake.ID) error {
	player := m.link.Player(guildID)
	queue := m.GetQueue(guildID)

	// Clear the queue
	queue.Clear()

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

	track, ok := queue.Next()
	if !ok {
		// Queue is empty, stop the player
		if err := player.Update(ctx, lavalink.WithNullTrack()); err != nil {
			return fmt.Errorf("failed to stop player: %w", err)
		}
		return nil
	}

	if err := player.Update(ctx, lavalink.WithTrack(track)); err != nil {
		return fmt.Errorf("failed to play track: %w", err)
	}

	return nil
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
