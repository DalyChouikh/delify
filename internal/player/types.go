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
	QueuePosition  int // Position in queue if not playing immediately
}

// QueuedTrack wraps a lavalink.Track with additional metadata.
type QueuedTrack struct {
	Track         lavalink.Track
	RequestedByID string
	RequestedBy   string // Display name
}

// TrackDisplayInfo provides display-ready track information.
type TrackDisplayInfo struct {
	Title          string
	Author         string
	URL            string
	Duration       lavalink.Duration
	ArtworkURL     string
	RequestedByID  string
	RequestedBy    string
	Position       int
	IsLive         bool
}

// ToDisplayInfo converts a QueuedTrack to TrackDisplayInfo.
func (q *QueuedTrack) ToDisplayInfo(position int) TrackDisplayInfo {
	return TrackDisplayInfo{
		Title:         q.Track.Info.Title,
		Author:        q.Track.Info.Author,
		URL:           *q.Track.Info.URI,
		Duration:      q.Track.Info.Length,
		ArtworkURL:    getArtworkURL(q.Track),
		RequestedByID: q.RequestedByID,
		RequestedBy:   q.RequestedBy,
		Position:      position,
		IsLive:        q.Track.Info.IsStream,
	}
}

// getArtworkURL extracts artwork URL from track, preferring Spotify artwork.
func getArtworkURL(track lavalink.Track) string {
	if track.Info.ArtworkURL != nil {
		return *track.Info.ArtworkURL
	}
	return ""
}

// CurrentTrackInfo holds information about the currently playing track.
type CurrentTrackInfo struct {
	Track         *lavalink.Track
	Position      lavalink.Duration
	IsPaused      bool
	RequestedByID string
	RequestedBy   string
}

// PlayerState represents the current state of the player.
type PlayerState struct {
	IsPlaying     bool
	IsPaused      bool
	CurrentTrack  *CurrentTrackInfo
	QueueLength   int
	NextTrack     *TrackDisplayInfo
}
