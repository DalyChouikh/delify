package player

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/disgoorg/disgolink/v3/disgolink"
	"github.com/disgoorg/disgolink/v3/lavalink"
	"github.com/disgoorg/snowflake/v2"
)

// Only the external audio service is substituted; tests exercise the real queue
// and manager. Embedding interfaces makes unexpected calls fail immediately.
type testNode struct {
	disgolink.Node
	queries []string
	results []*lavalink.LoadResult
	rest    disgolink.RestClient
	load    func(context.Context, string) (*lavalink.LoadResult, error)
}

func (n *testNode) Rest() disgolink.RestClient { return n.rest }
func (n *testNode) SessionID() string          { return "test-session" }

func (n *testNode) LoadTracks(ctx context.Context, query string) (*lavalink.LoadResult, error) {
	if n.load != nil {
		return n.load(ctx, query)
	}
	n.queries = append(n.queries, query)
	result := n.results[0]
	n.results = n.results[1:]
	return result, nil
}

type testRest struct {
	disgolink.RestClient
	player *testPlayer
}

func (r *testRest) UpdatePlayer(_ context.Context, _ string, _ snowflake.ID, update lavalink.PlayerUpdate) (*lavalink.Player, error) {
	r.player.updates = append(r.player.updates, &update)
	return &lavalink.Player{}, r.player.err
}

type testPlayer struct {
	disgolink.Player
	track   *lavalink.Track
	updates []*lavalink.PlayerUpdate
	err     error
}

func (p *testPlayer) Track() *lavalink.Track      { return p.track }
func (p *testPlayer) Paused() bool                { return false }
func (p *testPlayer) Position() lavalink.Duration { return 0 }
func (p *testPlayer) GuildID() snowflake.ID       { return 1 }
func (p *testPlayer) Update(_ context.Context, opts ...lavalink.PlayerUpdateOpt) error {
	u := lavalink.DefaultPlayerUpdate()
	u.Apply(opts)
	p.updates = append(p.updates, u)
	return p.err
}

type testLink struct {
	disgolink.Client
	node   disgolink.Node
	player *testPlayer
}

func (l *testLink) BestNode() disgolink.Node                { return l.node }
func (l *testLink) Player(snowflake.ID) disgolink.Player    { return l.player }
func (l *testLink) AddListeners(...disgolink.EventListener) {}

func testManager(t *testing.T, n *testNode) (*Manager, *testPlayer) {
	t.Helper()
	p := &testPlayer{track: &lavalink.Track{Encoded: "current"}}
	n.rest = &testRest{player: p}
	l := &testLink{node: n, player: p}
	m := NewManager(l, nil, time.Hour, slog.New(slog.NewTextHandler(io.Discard, nil)))
	state := newVoiceConnState()
	state.setVoiceState()
	state.setVoiceServer()
	m.voiceConnStates[1] = state
	t.Cleanup(func() { m.cancelInactivityTimer(1) })
	return m, p
}

func TestSearchFallsBackOnProviderException(t *testing.T) {
	n := &testNode{results: []*lavalink.LoadResult{
		{Data: lavalink.Exception{Message: "source unavailable"}},
		{Data: lavalink.Search{{Encoded: "found", Info: lavalink.TrackInfo{Title: "Found"}}}},
	}}
	m, _ := testManager(t, n)
	result, err := m.Play(context.Background(), 1, 2, "a song", "user", "User")
	if err != nil {
		t.Fatalf("fallback failed: %v", err)
	}
	if result.Track.Encoded != "found" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !reflect.DeepEqual(n.queries, []string{"ytsearch:a song", "scsearch:a song"}) {
		t.Fatal(n.queries)
	}
}

func TestDisconnectDuringLoadDiscardsTrack(t *testing.T) {
	n := &testNode{}
	m, p := testManager(t, n)
	n.load = func(ctx context.Context, _ string) (*lavalink.LoadResult, error) {
		m.OnVoiceStateUpdate(1, nil)
		return &lavalink.LoadResult{Data: lavalink.Track{Encoded: "late"}}, nil
	}
	if _, err := m.Play(context.Background(), 1, 2, "https://youtu.be/test", "", ""); err == nil {
		t.Fatal("load from disconnected voice session was accepted")
	}
	if m.GetCurrentTrack(1) != nil || m.GetQueue(1).Len() != 0 || len(p.updates) != 0 {
		t.Fatal("disconnected load recreated playback")
	}
}

func TestSearchFallsBackOnEmptySearch(t *testing.T) {
	n := &testNode{results: []*lavalink.LoadResult{
		{Data: lavalink.Search{}},
		{Data: lavalink.Search{{Encoded: "found"}}},
	}}
	m, _ := testManager(t, n)
	if _, err := m.Play(context.Background(), 1, 2, "song", "user", "User"); err != nil {
		t.Fatal(err)
	}
}

func TestExplicitSearchPrefixIsPreserved(t *testing.T) {
	m := &Manager{}
	for _, query := range []string{"ytsearch:hello", "ytmsearch:hello", "scsearch:hello", "spsearch:hello", "https://youtu.be/test"} {
		if got := m.buildSearchQueries("  " + query + "  "); !reflect.DeepEqual(got, []string{query}) {
			t.Errorf("%q: got %v", query, got)
		}
	}
}

func TestPlayNextUnpausesReplacementTrack(t *testing.T) {
	m, p := testManager(t, &testNode{})
	m.GetQueue(1).Add(lavalink.Track{Encoded: "next"}, "user", "User")
	if err := m.playNext(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	u := p.updates[0]
	if u.Paused == nil || *u.Paused {
		t.Fatal("replacement must explicitly unpause playback")
	}
}

func TestFailedPlayDoesNotLeaveCurrentTrack(t *testing.T) {
	m, p := testManager(t, &testNode{})
	p.err = errors.New("node unavailable")
	m.GetQueue(1).Add(lavalink.Track{Encoded: "next"}, "user", "User")
	if err := m.playNext(context.Background(), 1); err == nil {
		t.Fatal("expected failure")
	}
	if current := m.GetCurrentTrack(1); current != nil {
		t.Fatal("failed track retained as current")
	}
}

func TestTrackWithoutURIHasDisplayInfo(t *testing.T) {
	q := QueuedTrack{Track: lavalink.Track{Info: lavalink.TrackInfo{Title: "No URI"}}}
	info := q.ToDisplayInfo(1)
	if info.URL != "" || info.Title != "No URI" {
		t.Fatalf("unexpected display: %+v", info)
	}
}

func TestQueuePageRejectsInvalidSize(t *testing.T) {
	q := NewQueue()
	q.Add(lavalink.Track{}, "", "")
	for _, size := range []int{0, -1} {
		tracks, page, total := q.GetPage(0, size)
		if len(tracks) != 0 || page != 0 || total != 0 {
			t.Fatal("invalid page size must return empty page")
		}
	}
}

func TestSkippedTrackStateIsSynchronous(t *testing.T) {
	m, p := testManager(t, &testNode{})
	m.currentTracks[1] = &QueuedTrack{Track: *p.track}
	m.GetQueue(1).Add(lavalink.Track{Encoded: "next", Info: lavalink.TrackInfo{Title: "Next"}}, "user", "User")
	if _, err := m.Skip(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	state := m.GetPlayerState(1)
	if state.CurrentTrack == nil || state.CurrentTrack.Track.Encoded != "next" {
		t.Fatalf("state still contains old track: %+v", state)
	}
	if _, err := m.Skip(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if m.GetPlayerState(1).CurrentTrack != nil {
		t.Fatal("last skipped track still appears as playing")
	}
}

func TestSecondPlayBeforeTrackStartQueuesInsteadOfReplacing(t *testing.T) {
	n := &testNode{results: []*lavalink.LoadResult{
		{Data: lavalink.Track{Encoded: "one"}},
		{Data: lavalink.Track{Encoded: "two"}},
	}}
	m, p := testManager(t, n)
	p.track = nil
	if _, err := m.Play(context.Background(), 1, 2, "https://example.com/one", "", ""); err != nil {
		t.Fatal(err)
	}
	result, err := m.Play(context.Background(), 1, 2, "https://example.com/two", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if result.StartedPlaying || len(p.updates) != 1 || m.GetQueue(1).Len() != 1 {
		t.Fatal("second request replaced first track before its start event arrived")
	}
}

func TestClosedManagerRejectsPlayback(t *testing.T) {
	n := &testNode{results: []*lavalink.LoadResult{{Data: lavalink.Track{Encoded: "one"}}}}
	m, p := testManager(t, n)
	m.Close()
	if _, err := m.Play(context.Background(), 1, 2, "a song", "", ""); err == nil {
		t.Fatal("closed manager accepted playback")
	}
	if len(n.queries) != 0 || len(p.updates) != 0 {
		t.Fatal("closed manager called audio service")
	}
}

func TestLateEndDoesNotSkipRepeatedTrack(t *testing.T) {
	m, p := testManager(t, &testNode{})
	track := lavalink.Track{Encoded: "repeated"}
	m.GetQueue(1).Add(track, "", "")
	m.GetQueue(1).Add(track, "", "")
	if err := m.playNext(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	first := m.GetCurrentTrack(1).Track
	if _, err := m.Skip(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	m.onTrackEnd(p, lavalink.TrackEndEvent{Track: first, Reason: lavalink.TrackEndReasonFinished})
	if m.GetCurrentTrack(1) == nil {
		t.Fatal("late end event skipped the next copy of the same track")
	}
}

func TestReencodedTrackEndAdvancesQueue(t *testing.T) {
	m, p := testManager(t, &testNode{})
	m.GetQueue(1).Add(lavalink.Track{Encoded: "loaded"}, "", "")
	m.GetQueue(1).Add(lavalink.Track{Encoded: "next"}, "", "")
	if err := m.playNext(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	ended := m.GetCurrentTrack(1).Track
	// Lavalink re-encodes tracks with their current position for events.
	ended.Encoded = "serialized-at-end-position"
	m.onTrackEnd(p, lavalink.TrackEndEvent{Track: ended, Reason: lavalink.TrackEndReasonFinished})
	if current := m.GetCurrentTrack(1); current == nil || current.Track.Encoded != "next" {
		t.Fatal("natural end with changed encoding did not advance queue")
	}
}

func TestOldPositionUpdateDoesNotAffectNewTrack(t *testing.T) {
	m, p := testManager(t, &testNode{})
	old := lavalink.Now()
	m.GetQueue(1).Add(lavalink.Track{Encoded: "new", Info: lavalink.TrackInfo{Length: 180000}}, "", "")
	if err := m.playNext(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	m.onPlayerUpdate(p, lavalink.PlayerUpdateMessage{State: lavalink.PlayerState{Time: old, Position: 100000}})
	if got := m.GetPlayerState(1).CurrentTrack.Position; got >= 100000 {
		t.Fatalf("stale update changed position to %d", got)
	}
}

func TestCloseCancelsAndDrainsActiveLoad(t *testing.T) {
	entered := make(chan struct{})
	n := &testNode{load: func(ctx context.Context, _ string) (*lavalink.LoadResult, error) {
		close(entered)
		<-ctx.Done()
		return &lavalink.LoadResult{Data: lavalink.Track{Encoded: "late"}}, nil
	}}
	m, p := testManager(t, n)
	finished := make(chan error, 1)
	go func() {
		_, err := m.Play(context.Background(), 1, 2, "https://example.com/song", "", "")
		finished <- err
	}()
	<-entered
	m.Close()
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("load after shutdown succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel/drain active load")
	}
	if len(p.updates) != 0 || m.GetCurrentTrack(1) != nil {
		t.Fatal("late load started playback after Close")
	}
}
