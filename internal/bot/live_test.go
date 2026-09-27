package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/DalyChouikh/delify/internal/config"
	audio "github.com/DalyChouikh/delify/internal/lavalink"
	"github.com/DalyChouikh/delify/internal/logging"
	"github.com/DalyChouikh/delify/internal/player"
	"github.com/disgoorg/disgolink/v3/disgolink"
	"github.com/disgoorg/disgolink/v3/lavalink"
	"github.com/disgoorg/snowflake/v2"
)

// TestLivePlayback is opt-in: it joins the explicitly named Discord voice
// channel and plays the supplied track for 10 seconds. Stop the regular bot
// before running it, and have a listener confirm audible audio independently.
func TestLivePlayback(t *testing.T) {
	guildName, channelName, query := os.Getenv("DELIFY_TEST_GUILD"), os.Getenv("DELIFY_TEST_CHANNEL"), os.Getenv("DELIFY_TEST_QUERY")
	if guildName == "" || channelName == "" || query == "" {
		t.Skip("set DELIFY_TEST_GUILD, DELIFY_TEST_CHANNEL and DELIFY_TEST_QUERY for a live voice test")
	}
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{ReplaceAttr: logging.Redact(cfg.Discord.Token, cfg.Lavalink.Password, cfg.Lyrics.RapidAPIKey)}))
	b, err := New(cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := b.session.Open(); err != nil {
		t.Fatal("Discord connection failed")
	}
	t.Cleanup(func() { _ = b.Stop() })
	b.lavalinkClient, err = audio.NewClient(b.session, cfg.Lavalink, logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.lavalinkClient.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	b.playerManager = player.NewManager(b.lavalinkClient.Link, b.session, cfg.Bot.InactivityTimeout, logger)
	b.session.AddHandler(b.handleVoiceStateUpdate)
	b.session.AddHandler(b.handleVoiceServerUpdate)
	var guildID, channelID snowflake.ID
	guilds, err := b.session.UserGuilds(100, "", "", false, discordgo.WithContext(ctx))
	if err != nil {
		t.Fatal("could not list bot guilds")
	}
	for _, guild := range guilds {
		if strings.EqualFold(guild.Name, guildName) {
			guildID, _ = snowflake.Parse(guild.ID)
			channels, err := b.session.GuildChannels(guild.ID, discordgo.WithContext(ctx))
			if err != nil {
				t.Fatal("could not list target guild channels")
			}
			channelID, err = selectVoiceChannel(channels, channelName)
			if err != nil {
				for _, channel := range channels {
					if channel.Type == discordgo.ChannelTypeGuildVoice && strings.Contains(strings.ToLower(channel.Name), strings.ToLower(channelName)) {
						t.Logf("possible voice channel: %q (ID %s); rerun with its exact name or ID", channel.Name, channel.ID)
					}
				}
				t.Fatal(err)
			}
			if channelID != 0 {
				break
			}
		}
	}
	if guildID == 0 || channelID == 0 {
		t.Fatal("specified guild/voice channel not found")
	}
	failures := make(chan string, 5)
	b.lavalinkClient.Link.AddListeners(disgolink.NewListenerFunc(func(p disgolink.Player, e lavalink.TrackExceptionEvent) {
		if p.GuildID() == guildID {
			select {
			case failures <- e.Exception.Message:
			default:
			}
		}
	}))
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()
		_ = b.playerManager.Stop(stopCtx, guildID)
	})
	result, err := b.playerManager.Play(ctx, guildID, channelID, query, "", "Playback verification")
	if err != nil {
		t.Fatal(err)
	}
	if result.Track != nil {
		t.Logf("loaded %q, source=%s", result.Track.Info.Title, result.Track.Info.SourceName)
	}
	node := b.lavalinkClient.Link.BestNode()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case message := <-failures:
			t.Fatalf("audio source failed: %s", message)
		case <-ctx.Done():
			t.Fatal("timed out waiting for connected voice and track progress")
		case <-ticker.C:
			state, err := node.Rest().Player(ctx, node.SessionID(), guildID)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("voice connected=%t, position=%d ms, ping=%d ms", state.State.Connected, state.State.Position, state.State.Ping)
			if state.State.Connected && state.State.Position >= 10000 {
				t.Log("voice connection and 10 seconds of playback verified; audible confirmation still requires a listener")
				if os.Getenv("DELIFY_TEST_CONTROLS") == "true" {
					verifyLiveControls(t, ctx, b.playerManager, node, guildID, channelID, result.Track)
				}
				return
			}
		}
	}
}

func verifyLiveControls(t *testing.T, ctx context.Context, manager *player.Manager, node disgolink.Node, guildID, channelID snowflake.ID, track *lavalink.Track) {
	t.Helper()
	readPlayer := func() *lavalink.Player {
		t.Helper()
		state, err := node.Rest().Player(ctx, node.SessionID(), guildID)
		if err != nil {
			t.Fatal(err)
		}
		return state
	}
	if err := manager.Pause(ctx, guildID); err != nil {
		t.Fatal(err)
	}
	if !readPlayer().Paused {
		t.Fatal("Lavalink did not pause")
	}
	if err := manager.Seek(ctx, guildID, 30000); err != nil {
		t.Fatal(err)
	}
	if err := manager.Resume(ctx, guildID); err != nil {
		t.Fatal(err)
	}
	if readPlayer().Paused {
		t.Fatal("Lavalink did not resume")
	}
	// Seeking is asynchronous: buffered frames may be served until it completes.
	seekTimer := time.NewTimer(15 * time.Second)
	defer seekTimer.Stop()
	seekTicker := time.NewTicker(250 * time.Millisecond)
	defer seekTicker.Stop()
	seeked := false
	for !seeked {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-seekTimer.C:
			t.Fatalf("seek did not complete: position=%d ms", readPlayer().State.Position)
		case <-seekTicker.C:
			position := readPlayer().State.Position
			seeked = position >= 30000 && position < 45000
		}
	}
	if track == nil || track.Info.URI == nil {
		t.Fatal("control test requires a single track with a URL")
	}
	queued, err := manager.Play(ctx, guildID, channelID, *track.Info.URI, "", "Playback verification")
	if err != nil {
		t.Fatal(err)
	}
	if queued.StartedPlaying || queued.Track == nil || queued.Track.Info.Identifier != track.Info.Identifier || manager.GetQueue(guildID).Len() != 1 {
		t.Fatal("URL did not enqueue behind current playback")
	}
	if err := manager.Pause(ctx, guildID); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Skip(ctx, guildID); err != nil {
		t.Fatal(err)
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			t.Fatal("timed out verifying replacement playback")
		case <-ticker.C:
			state := readPlayer()
			current := manager.GetCurrentTrack(guildID)
			matchesQueued := state.Track != nil && current != nil && state.Track.Info.Identifier == queued.Track.Info.Identifier && livePlayID(state.Track) != "" && livePlayID(state.Track) == livePlayID(&current.Track)
			t.Logf("replacement connected=%t, paused=%t, position=%d ms, matches queued URL=%t", state.State.Connected, state.Paused, state.State.Position, matchesQueued)
			if state.State.Connected && !state.Paused && state.State.Position >= 3000 && matchesQueued {
				verifyLiveNaturalEnd(t, ctx, manager, node, guildID, channelID, queued.Track)
				if err := manager.Stop(ctx, guildID); err != nil {
					t.Fatal(err)
				}
				if readPlayer().Track != nil || manager.GetQueue(guildID).Len() != 0 || manager.GetCurrentTrack(guildID) != nil {
					t.Fatal("stop did not clear playback")
				}
				t.Log("pause, seek, resume, URL enqueue, skip from paused state, natural queue advancement, and stop verified against Lavalink")
				return
			}
		}
	}
}

func livePlayID(track *lavalink.Track) string {
	var data struct {
		ID string `json:"delify_play_id"`
	}
	_ = json.Unmarshal(track.UserData, &data)
	return data.ID
}

func verifyLiveNaturalEnd(t *testing.T, ctx context.Context, manager *player.Manager, node disgolink.Node, guildID, channelID snowflake.ID, track *lavalink.Track) {
	t.Helper()
	current := manager.GetCurrentTrack(guildID)
	oldID := livePlayID(&current.Track)
	if _, err := manager.Play(ctx, guildID, channelID, *track.Info.URI, "", "Playback verification"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Seek(ctx, guildID, int64(track.Info.Length)-2000); err != nil {
		t.Fatal(err)
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			t.Fatal("timed out waiting for natural queue advancement")
		case <-ticker.C:
			state, err := node.Rest().Player(ctx, node.SessionID(), guildID)
			if err != nil {
				t.Fatal(err)
			}
			if state.Track != nil && livePlayID(state.Track) != "" && livePlayID(state.Track) != oldID && state.State.Connected && state.State.Position >= 2000 {
				t.Log("natural track end advanced to queued repeat and playback progressed")
				return
			}
		}
	}
}

func selectVoiceChannel(channels []*discordgo.Channel, nameOrID string) (snowflake.ID, error) {
	var selected snowflake.ID
	for _, channel := range channels {
		if channel.Type != discordgo.ChannelTypeGuildVoice || (!strings.EqualFold(channel.Name, nameOrID) && channel.ID != nameOrID) {
			continue
		}
		if selected != 0 {
			return 0, fmt.Errorf("voice channel name is ambiguous; specify its channel ID")
		}
		id, err := snowflake.Parse(channel.ID)
		if err != nil {
			return 0, err
		}
		selected = id
	}
	if selected == 0 {
		return 0, fmt.Errorf("no exact voice channel match for %q; specify its exact name or channel ID", nameOrID)
	}
	return selected, nil
}

func TestLiveChannelSelectionIsExact(t *testing.T) {
	channels := []*discordgo.Channel{
		{ID: "1", Name: "music", Type: discordgo.ChannelTypeGuildText},
		{ID: "2", Name: "music", Type: discordgo.ChannelTypeGuildVoice},
		{ID: "3", Name: "private-music", Type: discordgo.ChannelTypeGuildVoice},
	}
	if id, err := selectVoiceChannel(channels, "music"); err != nil || id != 2 {
		t.Fatalf("selected %v: %v", id, err)
	}
	channels = append(channels, &discordgo.Channel{ID: "4", Name: "music", Type: discordgo.ChannelTypeGuildVoice})
	if _, err := selectVoiceChannel(channels, "music"); err == nil {
		t.Fatal("ambiguous voice name accepted")
	}
	if id, err := selectVoiceChannel(channels, "2"); err != nil || id != 2 {
		t.Fatalf("explicit ID selected %v: %v", id, err)
	}
}
