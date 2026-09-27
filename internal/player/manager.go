// Package player manages music playback and queue for Discord guilds.
package player

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/disgoorg/disgolink/v3/disgolink"
	"github.com/disgoorg/disgolink/v3/lavalink"
	"github.com/disgoorg/snowflake/v2"
)

// voiceConnState tracks the voice connection state for a guild.
type voiceConnState struct {
	ctx            context.Context
	cancel         context.CancelFunc
	ready          chan struct{} // Closed when voice connection is ready
	hasVoiceState  bool          // VoiceStateUpdate received
	hasVoiceServer bool          // VoiceServerUpdate received
	mu             sync.Mutex
}

// newVoiceConnState creates a new voice connection state tracker.
func newVoiceConnState() *voiceConnState {
	ctx, cancel := context.WithCancel(context.Background())
	return &voiceConnState{
		ctx: ctx, cancel: cancel,
		ready: make(chan struct{}),
	}
}

// setVoiceState marks voice state as received and signals ready if both are received.
func (v *voiceConnState) setVoiceState() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.hasVoiceState = true
	v.checkReady()
}

// setVoiceServer marks voice server as received and signals ready if both are received.
func (v *voiceConnState) setVoiceServer() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.hasVoiceServer = true
	v.checkReady()
}

// checkReady closes the ready channel if both voice state and server are received.
func (v *voiceConnState) checkReady() {
	if v.hasVoiceState && v.hasVoiceServer {
		select {
		case <-v.ready:
			// Already closed
		default:
			close(v.ready)
		}
	}
}

// isReady returns true if the voice connection is fully established.
func (v *voiceConnState) isReady() bool {
	select {
	case <-v.ready:
		return true
	default:
		return false
	}
}

// Manager handles music players across all guilds.
type Manager struct {
	link              disgolink.Client
	session           VoiceGateway
	queues            map[snowflake.ID]*Queue
	currentTracks     map[snowflake.ID]*QueuedTrack    // Track who requested the current song
	inactivityTimers  map[snowflake.ID]*time.Timer     // Inactivity timers per guild
	voiceConnStates   map[snowflake.ID]*voiceConnState // Voice connection states per guild
	inactivityTimeout time.Duration
	mu                sync.RWMutex
	logger            *slog.Logger
	operations        sync.Map // per-guild command serialization
	playback          map[snowflake.ID]playbackSnapshot
	closed            bool
	lifetime          context.Context
	cancel            context.CancelFunc
	inflight          sync.WaitGroup
	sequence          uint64
}

// VoiceGateway is the Discord gateway operation needed by the audio manager.
type VoiceGateway interface {
	ChannelVoiceJoinManual(guildID, channelID string, mute, deaf bool) error
}

type playbackSnapshot struct {
	paused   bool
	position lavalink.Duration
	updated  time.Time
	changed  time.Time
}

// Control audio through the REST boundary. disgolink.Player.Update mutates
// unsynchronized voice fields; this manager owns its playback snapshots.
func (m *Manager) updatePlayer(ctx context.Context, guildID snowflake.ID, opts ...lavalink.PlayerUpdateOpt) error {
	node := m.link.BestNode()
	if node == nil {
		return fmt.Errorf("no available Lavalink node")
	}
	update := lavalink.DefaultPlayerUpdate()
	update.Apply(opts)
	_, err := node.Rest().UpdatePlayer(ctx, node.SessionID(), guildID, *update)
	return err
}

func (m *Manager) lockGuild(guildID snowflake.ID) func() {
	value, _ := m.operations.LoadOrStore(guildID, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func (m *Manager) begin(ctx context.Context, guildID snowflake.ID) (context.Context, func(), error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, nil, fmt.Errorf("player manager is shutting down")
	}
	m.inflight.Add(1)
	m.mu.Unlock()
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(m.lifetime, cancel)
	unlock := m.lockGuild(guildID)
	done := func() { unlock(); stop(); cancel(); m.inflight.Done() }
	if err := ctx.Err(); err != nil {
		done()
		return nil, nil, err
	}
	return ctx, done, nil
}

// NewManager creates a new player manager.
func NewManager(link disgolink.Client, session VoiceGateway, inactivityTimeout time.Duration, logger *slog.Logger) *Manager {
	lifetime, cancel := context.WithCancel(context.Background())
	m := &Manager{
		link:              link,
		session:           session,
		queues:            make(map[snowflake.ID]*Queue),
		currentTracks:     make(map[snowflake.ID]*QueuedTrack),
		inactivityTimers:  make(map[snowflake.ID]*time.Timer),
		voiceConnStates:   make(map[snowflake.ID]*voiceConnState),
		inactivityTimeout: inactivityTimeout,
		logger:            logger,
		playback:          make(map[snowflake.ID]playbackSnapshot),
		lifetime:          lifetime,
		cancel:            cancel,
	}

	// Register event handlers for queue management
	link.AddListeners(disgolink.NewListenerFunc(m.onTrackEnd))
	link.AddListeners(disgolink.NewListenerFunc(m.onTrackStart))
	link.AddListeners(disgolink.NewListenerFunc(m.onTrackException))
	link.AddListeners(disgolink.NewListenerFunc(m.onPlayerUpdate))

	return m
}

// Play loads and plays a track or adds it to the queue.
func (m *Manager) Play(ctx context.Context, guildID, channelID snowflake.ID, query, requestedByID, requestedBy string) (*PlayResult, error) {
	ctx, done, err := m.begin(ctx, guildID)
	if err != nil {
		return nil, err
	}
	defer done()
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("please provide a song name or URL")
	}
	// Get or create player
	m.link.Player(guildID)
	queue := m.GetQueue(guildID)

	// Check if we need to join voice channel (not already connected or connected to different channel)
	needsVoiceJoin := !m.isVoiceConnected(guildID)
	node := m.link.BestNode()
	if node == nil {
		return nil, fmt.Errorf("no available Lavalink node")
	}

	// Ensure bot is connected to voice channel
	if needsVoiceJoin {
		if err := m.joinVoiceChannel(ctx, guildID, channelID); err != nil {
			return nil, fmt.Errorf("failed to join voice channel: %w", err)
		}
		defer func() {
			if m.GetCurrentTrack(guildID) == nil {
				m.startInactivityTimer(guildID)
			}
		}()
	}

	// Build ordered search queries. We try YouTube first, then SoundCloud fallback for plain text.
	m.mu.RLock()
	voice := m.voiceConnStates[guildID]
	m.mu.RUnlock()
	if voice == nil {
		return nil, fmt.Errorf("voice session disconnected")
	}
	// A disconnect synchronously cancels this session's loads and updates.
	requestCtx := ctx
	ctx, cancelVoice := context.WithCancel(voice.ctx)
	stopCancel := context.AfterFunc(requestCtx, cancelVoice)
	defer func() { stopCancel(); cancelVoice() }()
	if requestCtx.Err() != nil {
		cancelVoice()
	}
	searchQueries := m.buildSearchQueries(query)

	// Load tracks
	var result *PlayResult

	var (
		loadResult *lavalink.LoadResult
		loadErr    error
		loaded     bool
	)

	for i, searchQuery := range searchQueries {
		loadResult, loadErr = node.LoadTracks(ctx, searchQuery)
		if loadErr != nil {
			if i == len(searchQueries)-1 {
				return nil, fmt.Errorf("failed to load tracks: %w", loadErr)
			}
			m.logger.Warn("search attempt failed, trying fallback",
				"search_query", searchQuery,
				"error", loadErr,
			)
			continue
		}

		if loadResult == nil {
			return nil, fmt.Errorf("empty response from Lavalink")
		}
		var retry bool
		switch data := loadResult.Data.(type) {
		case lavalink.Empty, lavalink.Exception:
			retry = true
		case lavalink.Search:
			retry = len(data) == 0
		}
		if retry && i < len(searchQueries)-1 {
			m.logger.Info("source returned no usable tracks, trying fallback",
				"search_query", searchQuery,
			)
			continue
		}

		loaded = true
		break
	}

	if !loaded {
		return nil, fmt.Errorf("no tracks found for query: %s", query)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
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
		if len(data.Tracks) == 0 {
			return nil, fmt.Errorf("playlist contains no playable tracks")
		}
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
		if strings.Contains(query, "open.spotify.com/") || strings.HasPrefix(query, "spsearch:") {
			return nil, fmt.Errorf("Spotify could not load this item; check the developer app credentials and the app owner's Premium subscription: %s", data.Message)
		}
		return nil, fmt.Errorf("error loading track: %s", data.Message)

	default:
		return nil, fmt.Errorf("unexpected load result type")
	}

	// If nothing is playing, start playback
	if m.GetCurrentTrack(guildID) == nil {
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
	ctx, done, err := m.begin(ctx, guildID)
	if err != nil {
		return nil, err
	}
	defer done()
	current := m.GetCurrentTrack(guildID)
	if current == nil {
		return nil, fmt.Errorf("nothing is currently playing")
	}

	// Play next track
	if err := m.playNext(ctx, guildID); err != nil {
		return nil, err
	}

	return &current.Track, nil
}

// Stop stops playback and clears the queue.
func (m *Manager) Stop(ctx context.Context, guildID snowflake.ID) error {
	ctx, done, err := m.begin(ctx, guildID)
	if err != nil {
		return err
	}
	defer done()
	queue := m.GetQueue(guildID)

	// Clear the queue
	queue.Clear()

	// Cancel inactivity timer
	m.cancelInactivityTimer(guildID)

	// Clear current track info
	m.mu.Lock()
	delete(m.currentTracks, guildID)
	delete(m.playback, guildID)
	m.mu.Unlock()

	// Stop the player
	stopErr := m.updatePlayer(ctx, guildID, lavalink.WithNullTrack())

	// Disconnect from voice
	if err := m.leaveVoiceChannel(guildID); err != nil {
		m.logger.Warn("failed to leave voice channel", "error", err)
	}

	if stopErr != nil {
		return fmt.Errorf("failed to stop player: %w", stopErr)
	}
	return nil
}

// Pause pauses the current playback.
func (m *Manager) Pause(ctx context.Context, guildID snowflake.ID) error {
	ctx, done, err := m.begin(ctx, guildID)
	if err != nil {
		return err
	}
	defer done()
	state := m.GetPlayerState(guildID)
	if state.CurrentTrack == nil {
		return fmt.Errorf("nothing is currently playing")
	}
	if state.IsPaused {
		return fmt.Errorf("playback is already paused")
	}

	if err := m.updatePlayer(ctx, guildID, lavalink.WithPaused(true)); err != nil {
		return err
	}
	m.mu.Lock()
	m.playback[guildID] = playbackSnapshot{paused: true, position: state.CurrentTrack.Position, updated: time.Now(), changed: time.Now()}
	m.mu.Unlock()
	return nil
}

// Resume resumes paused playback.
func (m *Manager) Resume(ctx context.Context, guildID snowflake.ID) error {
	ctx, done, err := m.begin(ctx, guildID)
	if err != nil {
		return err
	}
	defer done()
	state := m.GetPlayerState(guildID)
	if state.CurrentTrack == nil {
		return fmt.Errorf("nothing is currently playing")
	}
	if !state.IsPaused {
		return fmt.Errorf("playback is not paused")
	}

	if err := m.updatePlayer(ctx, guildID, lavalink.WithPaused(false)); err != nil {
		return err
	}
	m.mu.Lock()
	m.playback[guildID] = playbackSnapshot{position: state.CurrentTrack.Position, updated: time.Now(), changed: time.Now()}
	m.mu.Unlock()
	return nil
}

// Seek seeks to a position in the current track.
func (m *Manager) Seek(ctx context.Context, guildID snowflake.ID, positionMs int64) error {
	ctx, done, err := m.begin(ctx, guildID)
	if err != nil {
		return err
	}
	defer done()
	_, err = m.seek(ctx, guildID, positionMs)
	return err
}

func (m *Manager) seek(ctx context.Context, guildID snowflake.ID, positionMs int64) (lavalink.Duration, error) {
	current := m.GetCurrentTrack(guildID)
	if current == nil {
		return 0, fmt.Errorf("nothing is currently playing")
	}
	track := &current.Track
	if track.Info.IsStream || track.Info.Length <= 0 {
		return 0, fmt.Errorf("this track cannot be seeked")
	}

	// Validate position
	if positionMs < 0 {
		positionMs = 0
	}
	maxPos := max(int64(track.Info.Length)-1, 0)
	if positionMs > maxPos {
		positionMs = maxPos
	}
	position := lavalink.Duration(positionMs)
	if err := m.updatePlayer(ctx, guildID, lavalink.WithPosition(position)); err != nil {
		return 0, err
	}
	m.mu.Lock()
	state := m.playback[guildID]
	state.position, state.updated = position, time.Now()
	state.changed = state.updated
	m.playback[guildID] = state
	m.mu.Unlock()
	return position, nil
}

// SeekRelative seeks relative to the current position.
func (m *Manager) SeekRelative(ctx context.Context, guildID snowflake.ID, deltaMs int64) (lavalink.Duration, error) {
	ctx, done, err := m.begin(ctx, guildID)
	if err != nil {
		return 0, err
	}
	defer done()
	state := m.GetPlayerState(guildID)
	if state.CurrentTrack == nil {
		return 0, fmt.Errorf("nothing is currently playing")
	}

	return m.seek(ctx, guildID, int64(state.CurrentTrack.Position)+deltaMs)
}

// ClearQueue clears the queue but keeps the current track playing.
func (m *Manager) ClearQueue(guildID snowflake.ID) int {
	_, done, err := m.begin(context.Background(), guildID)
	if err != nil {
		return 0
	}
	defer done()
	queue := m.GetQueue(guildID)
	return queue.Clear()
}

// GetPlayerState returns the current state of the player.
func (m *Manager) GetPlayerState(guildID snowflake.ID) *PlayerState {
	queue := m.GetQueue(guildID)

	state := &PlayerState{
		QueueLength: queue.Len(),
	}

	m.mu.RLock()
	var currentQueued *QueuedTrack
	if current := m.currentTracks[guildID]; current != nil {
		copy := *current
		currentQueued = &copy
	}
	playback := m.playback[guildID]
	m.mu.RUnlock()
	if currentQueued != nil {
		track := &currentQueued.Track
		state.IsPlaying = true
		state.IsPaused = playback.paused
		position := playback.position
		if !playback.paused && !playback.updated.IsZero() {
			position += lavalink.Duration(time.Since(playback.updated).Milliseconds())
		}
		if !track.Info.IsStream {
			position = min(position, track.Info.Length)
		}
		position = max(position, 0)

		state.CurrentTrack = &CurrentTrackInfo{
			Track:         track,
			Position:      position,
			IsPaused:      playback.paused,
			RequestedByID: currentQueued.RequestedByID,
			RequestedBy:   currentQueued.RequestedBy,
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
	if current := m.currentTracks[guildID]; current != nil {
		copy := *current
		return &copy
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

	queuedTrack, ok := queue.Next()
	if !ok {
		// Queue is empty, stop the player and start inactivity timer
		m.mu.Lock()
		delete(m.currentTracks, guildID)
		delete(m.playback, guildID)
		m.mu.Unlock()

		if err := m.updatePlayer(ctx, guildID, lavalink.WithNullTrack()); err != nil {
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
	m.sequence++
	sequence := m.sequence
	m.mu.Unlock()
	// Encoded audio is identical for repeated songs. Attach a distinct play ID
	// so a delayed end event cannot advance past a newer copy of the same song.
	metadata := map[string]any{}
	if len(queuedTrack.Track.UserData) > 0 {
		_ = json.Unmarshal(queuedTrack.Track.UserData, &metadata)
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["delify_play_id"] = fmt.Sprint(sequence)
	track, err := queuedTrack.Track.WithUserData(metadata)
	if err != nil {
		return fmt.Errorf("failed to attach playback identity: %w", err)
	}
	queuedTrack.Track = track
	m.mu.Lock()
	if err := ctx.Err(); err != nil {
		queue.Clear()
		m.mu.Unlock()
		return err
	}
	m.currentTracks[guildID] = &queuedTrack
	m.playback[guildID] = playbackSnapshot{updated: time.Now(), changed: time.Now()}
	m.mu.Unlock()

	if err := m.updatePlayer(ctx, guildID, lavalink.WithTrack(queuedTrack.Track), lavalink.WithPaused(false)); err != nil {
		m.mu.Lock()
		delete(m.currentTracks, guildID)
		delete(m.playback, guildID)
		m.mu.Unlock()
		m.startInactivityTimer(guildID)
		return fmt.Errorf("failed to play track: %w", err)
	}

	return nil
}

// startInactivityTimer starts a timer to leave voice after inactivity.
func (m *Manager) startInactivityTimer(guildID snowflake.ID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	if timer := m.inactivityTimers[guildID]; timer != nil {
		timer.Stop()
	}

	var timer *time.Timer
	timer = time.AfterFunc(m.inactivityTimeout, func() {
		defer m.lockGuild(guildID)()
		m.mu.Lock()
		if m.closed || m.inactivityTimers[guildID] != timer || m.currentTracks[guildID] != nil {
			m.mu.Unlock()
			return
		}
		delete(m.inactivityTimers, guildID)
		m.mu.Unlock()
		m.handleInactivityTimeout(guildID)
	})
	m.inactivityTimers[guildID] = timer

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
	delete(m.playback, guildID)
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

	ctx, done, err := m.begin(context.Background(), player.GuildID())
	if err != nil {
		return
	}
	defer done()
	current := m.GetCurrentTrack(player.GuildID())
	// Lavalink re-encodes audio with its current position. Only our immutable
	// per-play identity distinguishes both re-encoded events and repeated songs.
	if current == nil || playbackID(current.Track) == "" || playbackID(current.Track) != playbackID(event.Track) {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := m.playNext(ctx, player.GuildID()); err != nil {
		m.logger.Error("failed to play next track", "error", err, "guild", player.GuildID())
	}
}

func playbackID(track lavalink.Track) string {
	var data struct {
		ID string `json:"delify_play_id"`
	}
	_ = json.Unmarshal(track.UserData, &data)
	return data.ID
}

func (m *Manager) onPlayerUpdate(player disgolink.Player, event lavalink.PlayerUpdateMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.currentTracks[player.GuildID()] == nil {
		return
	}
	state := m.playback[player.GuildID()]
	if event.State.Time.Before(state.changed) {
		return
	}
	state.position, state.updated = event.State.Position, time.Now()
	m.playback[player.GuildID()] = state
}

// onTrackException handles track exception events.
func (m *Manager) onTrackException(player disgolink.Player, event lavalink.TrackExceptionEvent) {
	m.logger.Error("track exception",
		"guild", player.GuildID(),
		"track", event.Track.Info.Title,
		"error", event.Exception.Message,
	)
}

// buildSearchQueries returns search queries ordered by preference.
func (m *Manager) buildSearchQueries(query string) []string {
	query = strings.TrimSpace(query)
	// URLs should be loaded directly.
	if isURL(query) {
		return []string{query}
	}
	for _, prefix := range []string{"ytsearch:", "ytmsearch:", "scsearch:", "spsearch:"} {
		if strings.HasPrefix(query, prefix) {
			return []string{query}
		}
	}

	// Plain text searches: prefer YouTube and fallback to SoundCloud if empty.
	return []string{
		"ytsearch:" + query,
		"scsearch:" + query,
	}
}

// isURL checks if the string is a URL.
func isURL(s string) bool {
	return len(s) > 8 && (s[:7] == "http://" || s[:8] == "https://")
}

// isVoiceConnected checks if the bot is currently connected to a voice channel in the guild.
func (m *Manager) isVoiceConnected(guildID snowflake.ID) bool {
	m.mu.RLock()
	state, exists := m.voiceConnStates[guildID]
	m.mu.RUnlock()

	return exists && state.isReady()
}

// getOrCreateVoiceConnState gets or creates a voice connection state for a guild.
func (m *Manager) getOrCreateVoiceConnState(guildID snowflake.ID) *voiceConnState {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}

	if state, exists := m.voiceConnStates[guildID]; exists {
		return state
	}

	state := newVoiceConnState()
	m.voiceConnStates[guildID] = state
	return state
}

// clearVoiceConnState removes the voice connection state for a guild.
func (m *Manager) clearVoiceConnState(guildID snowflake.ID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if state := m.voiceConnStates[guildID]; state != nil {
		state.cancel()
	}
	delete(m.voiceConnStates, guildID)
}

// OnVoiceStateUpdate should be called when a voice state update is received for the bot.
func (m *Manager) OnVoiceStateUpdate(guildID snowflake.ID, channelID *snowflake.ID) {
	// If channelID is nil or empty, the bot left the voice channel
	if channelID == nil {
		m.cancelInactivityTimer(guildID)
		m.mu.Lock()
		if state := m.voiceConnStates[guildID]; state != nil {
			state.cancel()
		}
		delete(m.voiceConnStates, guildID)
		delete(m.currentTracks, guildID)
		delete(m.playback, guildID)
		if queue := m.queues[guildID]; queue != nil {
			queue.Clear()
		}
		m.mu.Unlock()
		return
	}

	state := m.getOrCreateVoiceConnState(guildID)
	if state == nil {
		return
	}
	state.setVoiceState()
	m.logger.Debug("voice state update received", "guild", guildID)
}

// OnVoiceServerUpdate should be called when a voice server update is received.
func (m *Manager) OnVoiceServerUpdate(guildID snowflake.ID) {
	state := m.getOrCreateVoiceConnState(guildID)
	if state == nil {
		return
	}
	state.setVoiceServer()
	m.logger.Debug("voice server update received", "guild", guildID)
}

// joinVoiceChannel connects the bot to a voice channel and waits for the connection to be ready.
func (m *Manager) joinVoiceChannel(ctx context.Context, guildID, channelID snowflake.ID) error {
	// Create a fresh voice connection state for this join attempt
	m.mu.Lock()
	if previous := m.voiceConnStates[guildID]; previous != nil {
		previous.cancel()
	}
	state := newVoiceConnState()
	m.voiceConnStates[guildID] = state
	m.mu.Unlock()

	// Request to join the voice channel
	err := m.session.ChannelVoiceJoinManual(guildID.String(), channelID.String(), false, true)
	if err != nil {
		m.clearVoiceConnState(guildID)
		return fmt.Errorf("failed to send voice join request: %w", err)
	}

	// Wait for voice connection to be fully established
	voiceTimeout := 10 * time.Second
	select {
	case <-state.ready:
		m.logger.Debug("voice connection ready", "guild", guildID)
		return nil
	case <-state.ctx.Done():
		return fmt.Errorf("voice session disconnected while joining")
	case <-time.After(voiceTimeout):
		m.clearVoiceConnState(guildID)
		// Try to leave since we timed out
		_ = m.session.ChannelVoiceJoinManual(guildID.String(), "", false, false)
		return fmt.Errorf("timed out waiting for voice connection")
	case <-ctx.Done():
		m.clearVoiceConnState(guildID)
		_ = m.session.ChannelVoiceJoinManual(guildID.String(), "", false, false)
		return ctx.Err()
	}
}

// leaveVoiceChannel disconnects the bot from voice.
func (m *Manager) leaveVoiceChannel(guildID snowflake.ID) error {
	m.clearVoiceConnState(guildID)
	if m.session == nil {
		return nil
	}
	return m.session.ChannelVoiceJoinManual(guildID.String(), "", false, false)
}

// Close cancels timers and disconnects all managed voice sessions on shutdown.
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	m.cancel()
	for _, timer := range m.inactivityTimers {
		timer.Stop()
	}
	clear(m.inactivityTimers)
	m.mu.Unlock()
	m.inflight.Wait()
	m.mu.Lock()
	guilds := make([]snowflake.ID, 0, len(m.voiceConnStates))
	for guildID := range m.voiceConnStates {
		guilds = append(guilds, guildID)
	}
	m.mu.Unlock()
	for _, guildID := range guilds {
		if m.session != nil {
			_ = m.leaveVoiceChannel(guildID)
		}
	}
}
