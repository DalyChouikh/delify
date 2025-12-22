package player

import "github.com/disgoorg/disgolink/v3/lavalink"

// ResultType indicates the type of play result.
type ResultType string

const (
	ResultTypeTrack    ResultType = "track"
	ResultTypePlaylist ResultType = "playlist"
)

// PlayResult contains information about a play operation.
type PlayResult struct {
	Type           ResultType
	Track          *lavalink.Track
	PlaylistName   string
	TrackCount     int
	StartedPlaying bool
}
