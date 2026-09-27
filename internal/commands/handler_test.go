package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DalyChouikh/delify/internal/components"
	"github.com/DalyChouikh/delify/internal/embed"
	"github.com/DalyChouikh/delify/internal/lyrics"
	"github.com/DalyChouikh/delify/internal/player"
	"github.com/bwmarrin/discordgo"
	"github.com/disgoorg/disgolink/v3/disgolink"
	"github.com/disgoorg/disgolink/v3/lavalink"
	"github.com/disgoorg/snowflake/v2"
)

type recordedRequest struct {
	method      string
	path        string
	body        []byte
	hasDeadline bool
}

type recordingTransport struct {
	requests  []recordedRequest
	onRequest func(recordedRequest)
}

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	_, hasDeadline := req.Context().Deadline()
	r.requests = append(r.requests, recordedRequest{req.Method, req.URL.Path, body, hasDeadline})
	if r.onRequest != nil {
		r.onRequest(r.requests[len(r.requests)-1])
	}
	response := `{}`
	if req.Method == http.MethodPut {
		response = `[]`
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(response)), Request: req}, nil
}

type commandTestClient struct {
	disgolink.Client
	playback     disgolink.Player
	beforePlayer func()
	node         disgolink.Node
}

func (c *commandTestClient) BestNode() disgolink.Node {
	if c.beforePlayer != nil {
		c.beforePlayer()
	}
	return c.node
}

type commandTestNode struct {
	disgolink.Node
	rest disgolink.RestClient
}

func (n *commandTestNode) Rest() disgolink.RestClient { return n.rest }
func (n *commandTestNode) SessionID() string          { return "test-session" }
func (n *commandTestNode) LoadTracks(context.Context, string) (*lavalink.LoadResult, error) {
	return &lavalink.LoadResult{Data: lavalink.Track{Encoded: "test-track", Info: lavalink.TrackInfo{Title: "Test Song", Length: 120000}}}, nil
}

type commandTestRest struct {
	disgolink.RestClient
	beforeUpdate func()
}

func (r *commandTestRest) UpdatePlayer(context.Context, string, snowflake.ID, lavalink.PlayerUpdate) (*lavalink.Player, error) {
	if r.beforeUpdate != nil {
		r.beforeUpdate()
	}
	return &lavalink.Player{}, nil
}

func startCommandTrack(t *testing.T, h *Handler, paused bool) {
	t.Helper()
	channel := snowflake.ID(3)
	h.player.OnVoiceStateUpdate(1, &channel)
	h.player.OnVoiceServerUpdate(1)
	if _, err := h.player.Play(context.Background(), 1, 3, "song", "2", "listener"); err != nil {
		t.Fatal(err)
	}
	if paused {
		if err := h.player.Pause(context.Background(), 1); err != nil {
			t.Fatal(err)
		}
	}
}

func (c *commandTestClient) Player(snowflake.ID) disgolink.Player {
	if c.beforePlayer != nil {
		c.beforePlayer()
	}
	return c.playback
}

func (c *commandTestClient) ExistingPlayer(snowflake.ID) disgolink.Player { return c.Player(1) }

func commandFixture(t *testing.T) (*Handler, *discordgo.InteractionCreate, *recordingTransport, *commandTestClient) {
	t.Helper()
	s, err := discordgo.New("Bot test")
	if err != nil {
		t.Fatal(err)
	}
	recorder := &recordingTransport{}
	s.Client = &http.Client{Transport: recorder}
	s.State.User = &discordgo.User{ID: "9"}
	if err := s.State.GuildAdd(&discordgo.Guild{ID: "1", VoiceStates: []*discordgo.VoiceState{
		{GuildID: "1", UserID: "2", ChannelID: "3"},
		{GuildID: "1", UserID: "9", ChannelID: "3"},
	}}); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	link := &commandTestClient{Client: disgolink.New(9)}
	link.playback = disgolink.NewPlayer(logger, link, nil, 1)
	link.node = &commandTestNode{rest: &commandTestRest{}}
	m := player.NewManager(link, nil, time.Hour, logger)
	t.Cleanup(m.Close)
	h := &Handler{session: s, player: m, logger: logger, templates: embed.NewTemplates(embed.TemplateConfig{}), lyricsStates: newLyricsStateManager()}
	i := &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{ID: "4", AppID: "9", Token: "test", GuildID: "1", Member: &discordgo.Member{User: &discordgo.User{ID: "2", Username: "listener"}}}}
	return h, i, recorder, link
}

func TestLyricsStateGetReturnsSnapshot(t *testing.T) {
	m := newLyricsStateManager()
	m.set("message", &lyrics.LyricsResult{Pages: []string{"one", "two"}, TotalPages: 2}, 1)
	state, _ := m.get("message")
	m.updatePage("message", 2)
	if state.currentPage != 1 {
		t.Fatalf("previous snapshot changed to page %d", state.currentPage)
	}
}

func TestLyricsStateStorageIsBounded(t *testing.T) {
	m := newLyricsStateManager()
	for n := 0; n < 1001; n++ {
		m.set(fmt.Sprint(n), &lyrics.LyricsResult{Pages: []string{"lyrics"}, TotalPages: 1}, 1)
	}
	if len(m.states) > 1000 {
		t.Fatalf("lyrics pagination retains %d messages without a bound", len(m.states))
	}
	if _, exists := m.get("1000"); !exists {
		t.Fatal("newest lyrics were not retained")
	}
}

func TestInteractionRejectsDirectMessages(t *testing.T) {
	h, i, recorder, link := commandFixture(t)
	i.GuildID = ""
	i.Member = nil
	i.Type = discordgo.InteractionApplicationCommand
	i.Data = discordgo.ApplicationCommandInteractionData{Name: "nowplaying"}
	link.beforePlayer = func() { t.Fatal("direct message reached playback") }
	h.HandleInteraction(h.session, i)
	if len(recorder.requests) != 1 || !strings.Contains(string(recorder.requests[0].body), "server") {
		t.Fatal("missing guild-only response")
	}
}

func TestLyricsPaginationRejectsMissingMessage(t *testing.T) {
	h, i, recorder, _ := commandFixture(t)
	i.Type = discordgo.InteractionMessageComponent
	i.Data = discordgo.MessageComponentInteractionData{CustomID: components.ButtonLyricsNext}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("missing message panicked: %v", r)
		}
	}()
	h.handleLyricsPagination(h.session, i)
	if len(recorder.requests) != 1 {
		t.Fatal("missing-message interaction was not acknowledged")
	}
}

func TestVoiceStateReadsUseDiscordStateLock(t *testing.T) {
	h, i, _, _ := commandFixture(t)
	state := h.session.State
	done := make(chan struct{})
	go func() {
		defer close(done)
		for n := 0; n < 1000; n++ {
			state.Lock()
			state.Guilds[0].VoiceStates[0].ChannelID = fmt.Sprint(n)
			state.Unlock()
		}
	}()
	for n := 0; n < 1000; n++ {
		h.verifyVoiceChannel(i)
	}
	<-done
}

func TestGetUserVoiceStateReturnsSnapshot(t *testing.T) {
	h, i, _, _ := commandFixture(t)
	voice, err := h.getUserVoiceState(i)
	if err != nil {
		t.Fatal(err)
	}
	voice.ChannelID = "changed"
	again, err := h.getUserVoiceState(i)
	if err != nil || again.ChannelID != "3" {
		t.Fatalf("caller changed shared voice state: %+v, %v", again, err)
	}
}

func TestGetUserVoiceStateRejectsMissingMember(t *testing.T) {
	h, i, _, _ := commandFixture(t)
	i.Member = nil
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("missing member panicked: %v", r)
		}
	}()
	if _, err := h.getUserVoiceState(i); err == nil {
		t.Fatal("missing member was accepted")
	}
}

func TestGetUserVoiceStateRejectsDisconnectedUser(t *testing.T) {
	h, i, _, _ := commandFixture(t)
	guild, _ := h.session.State.Guild("1")
	guild.VoiceStates[0].ChannelID = ""
	if _, err := h.getUserVoiceState(i); err == nil {
		t.Fatal("disconnected voice state was accepted")
	}
}

func TestPlayRejectsDifferentVoiceChannelBeforePlayback(t *testing.T) {
	h, i, recorder, link := commandFixture(t)
	guild, _ := h.session.State.Guild("1")
	guild.VoiceStates[1].ChannelID = "other"
	i.Type = discordgo.InteractionApplicationCommand
	i.Data = discordgo.ApplicationCommandInteractionData{Name: "play", Options: []*discordgo.ApplicationCommandInteractionDataOption{{Name: "query", Type: discordgo.ApplicationCommandOptionString, Value: "song"}}}
	link.beforePlayer = func() { t.Fatal("playback accessed from a different voice channel") }
	h.handlePlay(h.session, i)
	if len(recorder.requests) == 0 || !strings.Contains(string(recorder.requests[len(recorder.requests)-1].body), "Different Voice Channel") {
		t.Fatal("missing different-channel error")
	}
}

func TestQueuePaginationAdvancesPastSecondPage(t *testing.T) {
	h, i, recorder, _ := commandFixture(t)
	for n := 0; n < 30; n++ {
		h.player.GetQueue(1).Add(lavalink.Track{Info: lavalink.TrackInfo{Title: fmt.Sprintf("track %d", n+1)}}, "2", "listener")
	}
	i.Type = discordgo.InteractionMessageComponent
	i.Data = discordgo.MessageComponentInteractionData{CustomID: components.ButtonQueueNext}
	i.Message = &discordgo.Message{Embeds: []*discordgo.MessageEmbed{h.templates.QueueDisplay(nil, nil, 2, 3, 30, 0)}}
	h.handleQueuePagination(h.session, i)
	if len(recorder.requests) != 1 || !strings.Contains(string(recorder.requests[0].body), "Page 3/3") {
		t.Fatalf("did not advance to third page: %+v", recorder.requests)
	}
}

func TestSlashControlsAcknowledgeBeforePlayback(t *testing.T) {
	for _, command := range []string{"pause", "resume", "skip", "stop", "seek", "clear"} {
		t.Run(command, func(t *testing.T) {
			h, i, recorder, link := commandFixture(t)
			startCommandTrack(t, h, command == "resume")
			i.Type = discordgo.InteractionApplicationCommand
			i.Data = discordgo.ApplicationCommandInteractionData{Name: command, Options: []*discordgo.ApplicationCommandInteractionDataOption{{Name: "time", Type: discordgo.ApplicationCommandOptionString, Value: "10"}}}
			verifyAck := func() {
				if len(recorder.requests) == 0 {
					t.Fatal("playback called before interaction acknowledgment")
				}
				var response discordgo.InteractionResponse
				if err := json.Unmarshal(recorder.requests[0].body, &response); err != nil {
					t.Fatal(err)
				}
				if response.Type != discordgo.InteractionResponseDeferredChannelMessageWithSource {
					t.Fatalf("wrong acknowledgment type: %d", response.Type)
				}
			}
			updates := 0
			link.node.(*commandTestNode).rest.(*commandTestRest).beforeUpdate = func() { updates++; verifyAck() }
			h.handleCommand(h.session, i)
			verifyAck()
			if command != "clear" && updates == 0 {
				t.Fatal("control did not reach the REST update boundary")
			}
			if len(recorder.requests) < 2 || recorder.requests[len(recorder.requests)-1].method != http.MethodPatch {
				t.Fatal("deferred response was not completed")
			}
		})
	}
}

func TestButtonControlsAcknowledgeBeforePlayback(t *testing.T) {
	for _, button := range []string{components.ButtonPause, components.ButtonResume, components.ButtonSkip, components.ButtonStop, components.ButtonSeekForward10, components.ButtonClear} {
		t.Run(button, func(t *testing.T) {
			h, i, recorder, link := commandFixture(t)
			startCommandTrack(t, h, button == components.ButtonResume)
			i.Type = discordgo.InteractionMessageComponent
			i.Data = discordgo.MessageComponentInteractionData{CustomID: button}
			verifyAck := func() {
				if len(recorder.requests) == 0 {
					t.Fatal("playback called before interaction acknowledgment")
				}
				var response discordgo.InteractionResponse
				if err := json.Unmarshal(recorder.requests[0].body, &response); err != nil {
					t.Fatal(err)
				}
				want := discordgo.InteractionResponseDeferredMessageUpdate
				if button == components.ButtonSeekForward10 || button == components.ButtonClear {
					want = discordgo.InteractionResponseDeferredChannelMessageWithSource
				}
				if response.Type != want {
					t.Fatalf("acknowledgment type = %d, want %d", response.Type, want)
				}
			}
			updates := 0
			link.node.(*commandTestNode).rest.(*commandTestRest).beforeUpdate = func() { updates++; verifyAck() }
			h.handleButton(h.session, i)
			verifyAck()
			if button != components.ButtonClear && updates == 0 {
				t.Fatal("control did not reach the REST update boundary")
			}
			if len(recorder.requests) < 2 || recorder.requests[len(recorder.requests)-1].method != http.MethodPatch {
				t.Fatal("deferred button response was not completed")
			}
		})
	}
}

func TestClearAcknowledgesWhilePlaybackIsBlocked(t *testing.T) {
	for _, button := range []bool{false, true} {
		t.Run(fmt.Sprintf("button=%t", button), func(t *testing.T) {
			h, i, recorder, link := commandFixture(t)
			startCommandTrack(t, h, false)
			entered, release := make(chan struct{}), make(chan struct{})
			var enterOnce sync.Once
			link.beforePlayer = func() { enterOnce.Do(func() { close(entered) }); <-release }
			playDone := make(chan error, 1)
			go func() {
				_, err := h.player.Play(context.Background(), 1, 3, "queued song", "2", "listener")
				playDone <- err
			}()
			<-entered
			ack := make(chan discordgo.InteractionResponseType, 1)
			recorder.onRequest = func(request recordedRequest) {
				if strings.HasSuffix(request.path, "/callback") {
					var response discordgo.InteractionResponse
					if err := json.Unmarshal(request.body, &response); err == nil {
						ack <- response.Type
					}
				}
			}
			clearDone := make(chan struct{})
			go func() {
				defer close(clearDone)
				if button {
					i.Type = discordgo.InteractionMessageComponent
					i.Data = discordgo.MessageComponentInteractionData{CustomID: components.ButtonClear}
					h.handleButton(h.session, i)
				} else {
					i.Type = discordgo.InteractionApplicationCommand
					i.Data = discordgo.ApplicationCommandInteractionData{Name: "clear"}
					h.handleClear(h.session, i)
				}
			}()
			select {
			case got := <-ack:
				if got != discordgo.InteractionResponseDeferredChannelMessageWithSource {
					t.Errorf("clear acknowledgment = %d, want deferred reply", got)
				}
			case <-time.After(time.Second):
				t.Error("clear waited for playback before acknowledging")
			}
			close(release)
			if err := <-playDone; err != nil {
				t.Fatal(err)
			}
			<-clearDone
			if recorder.requests[len(recorder.requests)-1].method != http.MethodPatch {
				t.Fatal("clear did not complete its deferred reply")
			}
			if h.player.GetQueue(1).Len() != 0 {
				t.Fatal("queued song was not cleared")
			}
		})
	}
}

func TestNowPlayingUpdateClearsMessageAfterLastTrack(t *testing.T) {
	h, i, recorder, _ := commandFixture(t)
	h.updateNowPlayingMessage(h.session, i, false)
	if len(recorder.requests) != 1 {
		t.Fatalf("expected completed response with empty playback, got %d requests", len(recorder.requests))
	}
}

func TestCommandsAreGuildOnly(t *testing.T) {
	for _, command := range (&Handler{}).Commands() {
		if command.DMPermission == nil || *command.DMPermission {
			t.Errorf("%s is available in direct messages", command.Name)
		}
	}
}

func TestLyricsAcknowledgmentHasDeadline(t *testing.T) {
	h, i, recorder, _ := commandFixture(t)
	h.lyrics = lyrics.NewClient(lyrics.Config{APIKey: "test"})
	i.Type = discordgo.InteractionApplicationCommand
	i.Data = discordgo.ApplicationCommandInteractionData{Name: "lyrics"}
	h.handleLyrics(h.session, i)
	if len(recorder.requests) == 0 || !recorder.requests[0].hasDeadline {
		t.Fatal("lyrics acknowledgment has no deadline")
	}
}

func TestLyricsStateExpires(t *testing.T) {
	m := newLyricsStateManager()
	m.set("old", &lyrics.LyricsResult{}, 1)
	m.states["old"].expiresAt = time.Now().Add(-time.Second)
	if _, exists := m.get("old"); exists {
		t.Fatal("expired lyrics state remains accessible")
	}
	if len(m.states) != 0 {
		t.Fatal("expired state was not removed")
	}
}

func TestRegisterBulkReplacesCommandsWithDeadline(t *testing.T) {
	h, _, recorder, _ := commandFixture(t)
	if err := h.Register([]string{"1"}); err != nil {
		t.Fatal(err)
	}
	if len(recorder.requests) != 1 || recorder.requests[0].method != http.MethodPut || !recorder.requests[0].hasDeadline {
		t.Fatalf("want one bulk update with deadline, got %+v", recorder.requests)
	}
}
